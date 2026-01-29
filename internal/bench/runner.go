// Package bench implements A/B benchmark orchestration for evaluating
// Claude's ability to fix build failures with and without skill context.
package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	claude "github.com/MateoSegura/claudesdk-go"
	"github.com/MateoSegura/claudesdk-go/internal/corpus"
	"github.com/MateoSegura/claudesdk-go/internal/docker"
)

// Variant identifies whether a run includes skill context.
type Variant string

const (
	WithSkill    Variant = "with-skill"
	WithoutSkill Variant = "without-skill"
)

// RunConfig holds all parameters for a benchmark run.
type RunConfig struct {
	Corpus        *corpus.Corpus
	SkillName     string
	SkillPath     string // absolute path to skill directory on host
	Entries       []corpus.Entry
	MaxTurns      int
	Model         string
	OutputDir     string
	DryRun        bool
	GraderModel   string // model for prompt gen + grading (default: "sonnet")
	SkipGrading   bool   // disable automated grading
	SkipPromptGen bool   // use hardcoded prompts instead of generated ones
}

// HostPaths holds auto-detected paths for bind-mounting into containers.
type HostPaths struct {
	NodeDir  string // Node.js installation root (contains bin/claude)
	ClaudeDB string // ~/.claude directory with credentials
}

// EntryResult captures the outcome of one entry+variant combination.
type EntryResult struct {
	EntryID       string                 `json:"entry_id"`
	Variant       Variant                `json:"variant"`
	BuildPass     bool                   `json:"build_pass"`
	EvalExitCode  int                    `json:"eval_exit_code"`
	Metrics       RichMetrics            `json:"metrics"`
	WallClock     time.Duration          `json:"wall_clock_ns"`
	ClaudeOutput  string                 `json:"claude_output,omitempty"`
	Error         string                 `json:"error,omitempty"`
	ContainerName string                 `json:"container_name"`
	Prompt        string                 `json:"prompt_used,omitempty"`
	Transcript    []claude.StreamMessage `json:"-"`
	RawStream     string                 `json:"-"`
	Diff          string                 `json:"-"`
}

// BenchmarkResult is the complete output of a benchmark run.
type BenchmarkResult struct {
	RunID       string            `json:"run_id"`
	Timestamp   time.Time         `json:"timestamp"`
	Config      ConfigSummary     `json:"config"`
	Results     []EntryResult     `json:"results"`
	Comparisons []EntryComparison `json:"comparisons,omitempty"`
}

// ConfigSummary is a serializable snapshot of the run configuration.
type ConfigSummary struct {
	CorpusName string `json:"corpus_name"`
	SkillName  string `json:"skill_name"`
	MaxTurns   int    `json:"max_turns"`
	Model      string `json:"model"`
	EntryCount int    `json:"entry_count"`
}

// Runner executes A/B benchmarks.
type Runner struct {
	docker    *docker.Manager
	config    RunConfig
	host      HostPaths
	log       *log.Logger
	output    *OutputWriter
	promptGen *PromptGenerator
	grader    *Grader
}

const containerNodePath = "/usr/local/host-node"

// DetectHostPaths finds the Node.js installation and Claude credentials
// on the host so they can be bind-mounted into containers.
func DetectHostPaths() (HostPaths, error) {
	var hp HostPaths

	nodeBin, err := exec.LookPath("node")
	if err != nil {
		return hp, fmt.Errorf("node not found in PATH: %w", err)
	}
	nodeBin, err = filepath.EvalSymlinks(nodeBin)
	if err != nil {
		return hp, fmt.Errorf("resolving node symlink: %w", err)
	}
	// node is at <prefix>/bin/node, we need <prefix>
	hp.NodeDir = filepath.Dir(filepath.Dir(nodeBin))

	claudeBin := filepath.Join(hp.NodeDir, "bin", "claude")
	if _, err := os.Stat(claudeBin); err != nil {
		return hp, fmt.Errorf("claude CLI not found at %s: %w", claudeBin, err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return hp, fmt.Errorf("getting home dir: %w", err)
	}
	hp.ClaudeDB = filepath.Join(home, ".claude")
	if _, err := os.Stat(hp.ClaudeDB); err != nil {
		return hp, fmt.Errorf("~/.claude not found: %w", err)
	}

	return hp, nil
}

// NewRunner creates a benchmark runner with auto-detected host paths.
func NewRunner(cfg RunConfig, host HostPaths) *Runner {
	return &Runner{
		docker: docker.NewManager(),
		config: cfg,
		host:   host,
		log:    log.New(os.Stderr, "[bench] ", log.LstdFlags),
	}
}

// Run executes the full A/B benchmark. For each entry, it runs variant A
// (with-skill) then variant B (without-skill), collecting results as it goes.
// Handles SIGINT gracefully: cleans up the current container and writes
// partial results.
func (r *Runner) Run(ctx context.Context) (*BenchmarkResult, error) {
	runID := fmt.Sprintf("%s-%d", r.config.Corpus.Name, time.Now().Unix())

	result := &BenchmarkResult{
		RunID:     runID,
		Timestamp: time.Now(),
		Config: ConfigSummary{
			CorpusName: r.config.Corpus.Name,
			SkillName:  r.config.SkillName,
			MaxTurns:   r.config.MaxTurns,
			Model:      r.config.Model,
			EntryCount: len(r.config.Entries),
		},
	}

	if r.config.DryRun {
		r.printDryRun()
		return result, nil
	}

	// Initialize output writer
	ow, err := NewOutputWriter(r.config.OutputDir, runID)
	if err != nil {
		return nil, fmt.Errorf("creating output writer: %w", err)
	}
	r.output = ow

	// Initialize prompt generator
	graderModel := r.config.GraderModel
	if graderModel == "" {
		graderModel = "sonnet"
	}

	if !r.config.SkipPromptGen {
		r.promptGen = NewPromptGenerator(graderModel, r.config.SkillName, r.config.SkillPath)
	}

	// Initialize grader
	if !r.config.SkipGrading && r.config.SkillName != "" {
		r.grader = NewGrader(graderModel)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	go func() {
		select {
		case <-sigCh:
			r.log.Println("interrupt received, finishing current entry...")
			cancel()
		case <-ctx.Done():
		}
	}()

	for i, entry := range r.config.Entries {
		if ctx.Err() != nil {
			r.log.Printf("interrupted, writing partial results (%d/%d entries)", i, len(r.config.Entries))
			break
		}

		r.log.Printf("entry %d/%d: %s (%s)", i+1, len(r.config.Entries), entry.ID, entry.Difficulty)

		var withSkillResult, withoutSkillResult *EntryResult

		// Variant A: with skill
		if r.config.SkillName != "" {
			r.log.Printf("  variant: %s", WithSkill)
			er := r.runEntry(ctx, entry, WithSkill)
			result.Results = append(result.Results, er)
			withSkillResult = &result.Results[len(result.Results)-1]
		}

		if ctx.Err() != nil {
			break
		}

		// Variant B: without skill
		r.log.Printf("  variant: %s", WithoutSkill)
		er := r.runEntry(ctx, entry, WithoutSkill)
		result.Results = append(result.Results, er)
		withoutSkillResult = &result.Results[len(result.Results)-1]

		// Grade both variants
		if r.grader != nil && withSkillResult != nil && withoutSkillResult != nil && ctx.Err() == nil {
			r.log.Printf("  grading %s...", entry.ID)
			comparison := EntryComparison{
				EntryID:      entry.ID,
				WithSkill:    withSkillResult,
				WithoutSkill: withoutSkillResult,
			}

			grade, err := r.grader.Grade(ctx, entry, withSkillResult, withoutSkillResult)
			if err != nil {
				r.log.Printf("  warning: grading failed for %s: %v", entry.ID, err)
			} else {
				comparison.Grade = grade
				if err := r.output.WriteGrade(entry.ID, grade); err != nil {
					r.log.Printf("  warning: failed to write grade for %s: %v", entry.ID, err)
				}
				r.log.Printf("  grade: %s (skill=%d/50, no-skill=%d/50)",
					grade.Verdict, grade.WithSkillGrade.Total(), grade.WithoutSkillGrade.Total())
			}

			result.Comparisons = append(result.Comparisons, comparison)
		}
	}

	return result, nil
}

func (r *Runner) runEntry(ctx context.Context, entry corpus.Entry, variant Variant) EntryResult {
	start := time.Now()
	containerName := fmt.Sprintf("bench-%s-%s-%d", entry.ID, variant, time.Now().UnixNano())

	er := EntryResult{
		EntryID:       entry.ID,
		Variant:       variant,
		ContainerName: containerName,
	}

	// 1. Create container with host Node.js + Claude credentials bind-mounted
	containerID, err := r.docker.CreateContainer(docker.ContainerOpts{
		Image:   r.config.Corpus.Image,
		Name:    containerName,
		Volumes: r.config.Corpus.Volumes,
		Binds: map[string]string{
			r.host.NodeDir:  containerNodePath,
			r.host.ClaudeDB: "/root/.claude",
		},
		Entrypoint: "/bin/bash",
		Cmd:        []string{"-c", "sleep infinity"},
	})
	if err != nil {
		er.Error = fmt.Sprintf("create container: %v", err)
		er.WallClock = time.Since(start)
		return er
	}

	defer func() {
		if rmErr := r.docker.RemoveContainer(containerID, true); rmErr != nil {
			r.log.Printf("  warning: failed to remove container %s: %v", containerName, rmErr)
		}
	}()

	// 2. Start container
	if err := r.docker.StartContainer(containerID); err != nil {
		er.Error = fmt.Sprintf("start container: %v", err)
		er.WallClock = time.Since(start)
		return er
	}

	// 3. Run setup commands
	if len(entry.SetupCommands) > 0 {
		setupCmd := strings.Join(entry.SetupCommands, " && ")
		r.log.Printf("  setup: %s", truncate(setupCmd, 80))

		output, exitCode, err := r.docker.ExecCommand(containerID, []string{"bash", "-c", setupCmd})
		if err != nil {
			er.Error = fmt.Sprintf("setup exec: %v", err)
			er.WallClock = time.Since(start)
			return er
		}
		if exitCode != 0 {
			er.Error = fmt.Sprintf("setup failed (exit %d): %s", exitCode, truncate(output, 500))
			er.WallClock = time.Since(start)
			return er
		}
	}

	// 4. Copy skill into container (variant A only)
	if variant == WithSkill && r.config.SkillPath != "" {
		skillDst := "/root/zephyrproject/zephyr/.claude/skills/"
		r.docker.ExecCommand(containerID, []string{"mkdir", "-p", skillDst})
		if err := r.docker.CopyToContainer(containerID, r.config.SkillPath, skillDst); err != nil {
			er.Error = fmt.Sprintf("copy skill: %v", err)
			er.WallClock = time.Since(start)
			return er
		}
	}

	// 5. Snapshot broken state for diff capture
	snapshotCmd := "cd /root/zephyrproject/zephyr && git config user.name bench && git config user.email bench@local && git add -A && git commit --allow-empty -m broken --no-verify"
	r.docker.ExecCommand(containerID, []string{"bash", "-c", snapshotCmd})

	// 6. Generate or build prompt
	var prompt string
	if r.promptGen != nil {
		prompt, _ = r.promptGen.Generate(ctx, entry, variant)
	} else {
		prompt = fallbackPrompt(entry, variant, r.config.SkillName)
	}
	er.Prompt = prompt

	// Write prompt to disk
	if r.output != nil {
		if err := r.output.WritePrompt(entry.ID, variant, prompt); err != nil {
			r.log.Printf("  warning: failed to write prompt: %v", err)
		}
	}

	// 7. Run Claude (--print mode skips workspace trust, no permission bypass needed)
	claudeCmd := r.buildClaudeCommand(prompt)
	r.log.Printf("  running claude (max %d turns)...", r.config.MaxTurns)

	claudeOutput, _, err := r.docker.ExecCommand(containerID, []string{"bash", "-c", claudeCmd})
	if err != nil {
		er.Error = fmt.Sprintf("claude exec: %v", err)
		er.WallClock = time.Since(start)
		return er
	}
	er.ClaudeOutput = claudeOutput
	er.RawStream = claudeOutput

	// 8. Parse full transcript from stream-json output
	er.Transcript = parseTranscript(claudeOutput)

	// 9. Extract rich metrics from result message
	er.Metrics = parseRichMetrics(claudeOutput)

	// 10. Capture git diff of Claude's changes
	diffCmd := "cd /root/zephyrproject/zephyr && git diff HEAD"
	diffOutput, _, _ := r.docker.ExecCommand(containerID, []string{"bash", "-c", diffCmd})
	er.Diff = diffOutput

	// 11. Run evaluation (from the zephyr working directory)
	evalCmd := fmt.Sprintf("cd /root/zephyrproject/zephyr && . /root/zephyrproject/zephyr/zephyr-env.sh && %s", entry.Evaluation.Command)
	r.log.Printf("  evaluating: %s", truncate(entry.Evaluation.Command, 80))

	_, evalExit, err := r.docker.ExecCommand(containerID, []string{"bash", "-c", evalCmd})
	if err != nil {
		er.Error = fmt.Sprintf("eval exec: %v", err)
		er.WallClock = time.Since(start)
		er.Metrics.WallClock = er.WallClock
		r.writeVariantArtifacts(&er)
		return er
	}

	er.EvalExitCode = evalExit
	er.BuildPass = evalExit == entry.Evaluation.SuccessExitCode
	er.WallClock = time.Since(start)
	er.Metrics.WallClock = er.WallClock

	status := "FAIL"
	if er.BuildPass {
		status = "PASS"
	}
	r.log.Printf("  result: %s (exit=%d, cost=$%.4f, turns=%d)", status, evalExit, er.Metrics.TotalCostUSD, er.Metrics.Turns)

	// 12. Write all variant artifacts to disk (after eval so WallClock is set)
	r.writeVariantArtifacts(&er)

	return er
}

func (r *Runner) writeVariantArtifacts(er *EntryResult) {
	if r.output == nil {
		return
	}
	if err := r.output.WriteVariantOutput(er); err != nil {
		r.log.Printf("  warning: failed to write variant output: %v", err)
	}
}

func (r *Runner) buildClaudeCommand(prompt string) string {
	var parts []string

	// Add host Node.js to PATH so claude CLI is available
	parts = append(parts, fmt.Sprintf("export PATH=%s/bin:$PATH", containerNodePath))
	parts = append(parts, "cd /root/zephyrproject/zephyr")
	parts = append(parts, ". /root/zephyrproject/zephyr/zephyr-env.sh")

	claudeArgs := []string{
		"claude",
		"--print",
		"--verbose",
		"--output-format", "stream-json",
		"--max-turns", fmt.Sprintf("%d", r.config.MaxTurns),
		"--allowed-tools", "Bash", "Edit", "Read", "Write", "Glob", "Grep",
	}

	if r.config.Model != "" {
		claudeArgs = append(claudeArgs, "--model", r.config.Model)
	}

	// Pipe prompt via stdin (--allowed-tools consumes positional args)
	escaped := strings.ReplaceAll(prompt, "'", "'\\''")
	cmd := fmt.Sprintf("echo '%s' | %s", escaped, strings.Join(claudeArgs, " "))

	parts = append(parts, cmd)
	return strings.Join(parts, " && ")
}

func (r *Runner) printDryRun() {
	fmt.Println("=== DRY RUN ===")
	fmt.Printf("Corpus:    %s (%d entries)\n", r.config.Corpus.Name, len(r.config.Entries))
	fmt.Printf("Image:     %s\n", r.config.Corpus.Image)
	fmt.Printf("Skill:     %s\n", r.config.SkillName)
	if r.config.SkillPath != "" {
		fmt.Printf("Skill Dir: %s\n", r.config.SkillPath)
	}
	fmt.Printf("Node Dir:  %s\n", r.host.NodeDir)
	fmt.Printf("Max Turns: %d\n", r.config.MaxTurns)
	fmt.Printf("Model:     %s\n", r.config.Model)
	fmt.Printf("Output:    %s\n", r.config.OutputDir)
	if !r.config.SkipPromptGen {
		graderModel := r.config.GraderModel
		if graderModel == "" {
			graderModel = "sonnet"
		}
		fmt.Printf("Prompt Gen: enabled (model: %s)\n", graderModel)
	} else {
		fmt.Printf("Prompt Gen: disabled (using hardcoded prompts)\n")
	}
	if !r.config.SkipGrading {
		graderModel := r.config.GraderModel
		if graderModel == "" {
			graderModel = "sonnet"
		}
		fmt.Printf("Grading:    enabled (model: %s)\n", graderModel)
	} else {
		fmt.Printf("Grading:    disabled\n")
	}
	fmt.Println()

	variants := 1
	if r.config.SkillName != "" {
		variants = 2
	}
	fmt.Printf("Plan: %d entries x %d variants = %d total runs\n",
		len(r.config.Entries), variants, len(r.config.Entries)*variants)
	fmt.Println()

	for i, e := range r.config.Entries {
		fmt.Printf("  %d. %-30s [%s] %s\n", i+1, e.ID, e.Difficulty, e.Board)
		fmt.Printf("     Build: %s\n", e.Evaluation.Command)
		fmt.Printf("     Setup: %s\n", truncate(strings.Join(e.SetupCommands, " && "), 70))
	}
	fmt.Println()
	fmt.Println("Validation: OK")
}

// parseTranscript parses raw JSONL output into a slice of StreamMessage.
func parseTranscript(output string) []claude.StreamMessage {
	var messages []claude.StreamMessage

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "{") {
			continue
		}

		var msg claude.StreamMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		messages = append(messages, msg)
	}

	return messages
}

// parseRichMetrics extracts RichMetrics from the stream-json output.
// Scans backward for the result message (metrics), and forward for the init
// message (model, since it's not included in the result line).
func parseRichMetrics(output string) RichMetrics {
	var metrics RichMetrics

	lines := strings.Split(output, "\n")

	// Backward scan: find result message for metrics
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || !strings.Contains(line, `"type"`) {
			continue
		}

		var msg claude.StreamMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Type == "result" {
			metrics.CostUSD = msg.CostUSD
			metrics.TotalCostUSD = msg.TotalCost
			metrics.Turns = msg.NumTurns
			metrics.DurationMS = msg.DurationMS
			metrics.DurationAPIMS = msg.DurationAPIMS
			metrics.SessionID = msg.SessionID
			metrics.IsError = msg.IsErrorResult
			metrics.ResultSubtype = msg.Subtype

			if msg.Usage != nil {
				metrics.InputTokens = msg.Usage.InputTokens
				metrics.OutputTokens = msg.Usage.OutputTokens
				metrics.CacheCreationInputTokens = msg.Usage.CacheCreationInputTokens
				metrics.CacheReadInputTokens = msg.Usage.CacheReadInputTokens
			}
			break
		}
	}

	// Forward scan: find init message for model (not present in result line)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var msg claude.StreamMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Type == "system" && msg.Subtype == "init" {
			if msg.Model != "" {
				metrics.Model = msg.Model
			}
			if metrics.SessionID == "" && msg.SessionID != "" {
				metrics.SessionID = msg.SessionID
			}
			break
		}
	}

	return metrics
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

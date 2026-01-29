// Command configbench runs A/B benchmarks to evaluate whether Claude Code
// skills improve its ability to fix Zephyr build failures.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MateoSegura/claudesdk-go/internal/bench"
	"github.com/MateoSegura/claudesdk-go/internal/corpus"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "run" {
		fmt.Fprintf(os.Stderr, "Usage: configbench run [flags]\n")
		os.Exit(1)
	}

	fs := flag.NewFlagSet("run", flag.ExitOnError)
	corpusPath := fs.String("corpus", "", "Path to corpus YAML file (required)")
	skillName := fs.String("skill", "", "Skill name for A/B test")
	outputDir := fs.String("output", "results", "Output directory for results")
	entries := fs.String("entries", "", "Comma-separated entry IDs to filter")
	maxTurns := fs.Int("max-turns", 15, "Claude max turns per entry")
	model := fs.String("model", "", "Claude model to use")
	dryRun := fs.Bool("dry-run", false, "Validate and print plan, don't run containers")
	graderModel := fs.String("grader-model", "sonnet", "Model for prompt generation and grading")
	skipGrading := fs.Bool("skip-grading", false, "Disable automated grading")
	skipPromptGen := fs.Bool("skip-prompt-gen", false, "Use hardcoded prompts instead of generated ones")

	fs.Parse(os.Args[2:])

	if *corpusPath == "" {
		fmt.Fprintf(os.Stderr, "error: --corpus is required\n")
		os.Exit(1)
	}

	// Load and validate corpus
	c, err := corpus.Load(*corpusPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading corpus: %v\n", err)
		os.Exit(1)
	}
	if err := c.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "error validating corpus: %v\n", err)
		os.Exit(1)
	}

	// Determine entries to run
	runEntries := c.Entries
	if *entries != "" {
		ids := strings.Split(*entries, ",")
		for i := range ids {
			ids[i] = strings.TrimSpace(ids[i])
		}
		runEntries, err = c.FilterEntries(ids)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error filtering entries: %v\n", err)
			os.Exit(1)
		}
	}

	// Resolve skill path
	var skillPath string
	if *skillName != "" {
		skillPath, err = findSkillDir(*skillName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	// Detect host Node.js + Claude credentials for bind-mounting
	host, err := bench.DetectHostPaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error detecting host paths: %v\n", err)
		os.Exit(1)
	}

	cfg := bench.RunConfig{
		Corpus:        c,
		SkillName:     *skillName,
		SkillPath:     skillPath,
		Entries:       runEntries,
		MaxTurns:      *maxTurns,
		Model:         *model,
		OutputDir:     *outputDir,
		DryRun:        *dryRun,
		GraderModel:   *graderModel,
		SkipGrading:   *skipGrading,
		SkipPromptGen: *skipPromptGen,
	}

	runner := bench.NewRunner(cfg, host)
	result, err := runner.Run(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *dryRun {
		return
	}

	if err := bench.WriteResults(result, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "error writing results: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nResults written to %s/%s/\n", *outputDir, result.RunID)
}

// findSkillDir walks up the directory tree looking for .claude/skills/<name>.
// This finds skills defined in parent project .claude directories (e.g.,
// .claude-test/.claude/skills/) without requiring a copy inside this repo.
func findSkillDir(name string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}

	var searched []string
	for {
		candidate := filepath.Join(dir, ".claude", "skills", name)
		searched = append(searched, candidate)
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("skill %q not found, searched:\n  %s", name, strings.Join(searched, "\n  "))
}

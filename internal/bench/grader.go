package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	claude "github.com/MateoSegura/claudesdk-go"
	"github.com/MateoSegura/claudesdk-go/internal/corpus"
)

// Grader uses the Claude SDK to evaluate and compare both variants of a
// benchmark entry, producing structured grades across multiple dimensions.
type Grader struct {
	model string
	log   *log.Logger
}

// NewGrader creates a grader that uses Claude to evaluate benchmark results.
func NewGrader(model string) *Grader {
	return &Grader{
		model: model,
		log:   log.New(os.Stderr, "[grader] ", log.LstdFlags),
	}
}

// Grade evaluates both variants of an entry and returns structured grades.
func (g *Grader) Grade(ctx context.Context, entry corpus.Entry, withSkill, withoutSkill *EntryResult) (*GradeResult, error) {
	prompt := g.buildGradingPrompt(entry, withSkill, withoutSkill)

	session, err := claude.NewSession(claude.SessionConfig{
		LaunchOptions: claude.LaunchOptions{
			Model:           g.model,
			MaxTurns:        3,
			SkipPermissions: true,
			JSONSchema:      gradeJSONSchema(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating grader session: %w", err)
	}

	result, err := session.RunAndCollect(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("running grader: %w", err)
	}

	if result.StructuredOutput == nil {
		return nil, fmt.Errorf("grader returned no structured output")
	}

	// Marshal then unmarshal to convert from any to GradeResult
	data, err := json.Marshal(result.StructuredOutput)
	if err != nil {
		return nil, fmt.Errorf("marshaling structured output: %w", err)
	}

	var grade GradeResult
	if err := json.Unmarshal(data, &grade); err != nil {
		return nil, fmt.Errorf("unmarshaling grade result: %w", err)
	}

	return &grade, nil
}

func (g *Grader) buildGradingPrompt(entry corpus.Entry, withSkill, withoutSkill *EntryResult) string {
	var sb strings.Builder

	sb.WriteString("You are an expert code reviewer evaluating two attempts to fix a Zephyr RTOS build failure.\n\n")

	sb.WriteString("## Bug Details\n\n")
	sb.WriteString(fmt.Sprintf("- **Entry ID**: %s\n", entry.ID))
	sb.WriteString(fmt.Sprintf("- **Difficulty**: %s\n", entry.Difficulty))
	sb.WriteString(fmt.Sprintf("- **Board**: %s\n", entry.Board))
	sb.WriteString(fmt.Sprintf("- **Build Command**: %s\n", entry.Evaluation.Command))
	sb.WriteString(fmt.Sprintf("- **Error Description**: %s\n\n", strings.TrimSpace(entry.Description)))

	sb.WriteString("## Variant A: With Skill\n\n")
	writeVariantSummary(&sb, withSkill)

	sb.WriteString("## Variant B: Without Skill\n\n")
	writeVariantSummary(&sb, withoutSkill)

	sb.WriteString("## Grading Instructions\n\n")
	sb.WriteString("Grade each variant on a scale of 1-10 for each dimension:\n\n")
	sb.WriteString("1. **Correctness** (1-10): Did it fix the bug? Does the build pass with the correct fix?\n")
	sb.WriteString("2. **Code Quality** (1-10): Is the fix idiomatic, clean, and following Zephyr conventions?\n")
	sb.WriteString("3. **Diagnosis** (1-10): Did it correctly identify the root cause in its reasoning?\n")
	sb.WriteString("4. **Minimality** (1-10): Only necessary changes? No over-engineering or unrelated modifications?\n")
	sb.WriteString("5. **Efficiency** (1-10): Reasonable number of turns, cost, and time usage?\n\n")

	sb.WriteString("Then provide a verdict: 'skill_better', 'no_skill_better', 'tie', or 'inconclusive'.\n")
	sb.WriteString("Include detailed reasoning explaining your verdict.\n")

	return sb.String()
}

func writeVariantSummary(sb *strings.Builder, result *EntryResult) {
	if result == nil {
		sb.WriteString("*No result available*\n\n")
		return
	}

	status := "FAIL"
	if result.BuildPass {
		status = "PASS"
	}
	if result.Error != "" {
		status = "ERROR: " + truncate(result.Error, 200)
	}

	sb.WriteString(fmt.Sprintf("- **Build Result**: %s\n", status))
	sb.WriteString(fmt.Sprintf("- **Turns**: %d\n", result.Metrics.Turns))
	sb.WriteString(fmt.Sprintf("- **Cost**: $%.4f\n", result.Metrics.TotalCostUSD))
	sb.WriteString(fmt.Sprintf("- **Tokens**: %d in / %d out\n", result.Metrics.InputTokens, result.Metrics.OutputTokens))
	sb.WriteString(fmt.Sprintf("- **Wall Clock**: %s\n\n", formatDuration(result.WallClock)))

	if result.Diff != "" {
		sb.WriteString("### Changes Made\n\n")
		sb.WriteString("```diff\n")
		diff := result.Diff
		if len(diff) > 5000 {
			diff = diff[:5000] + "\n...(truncated)"
		}
		sb.WriteString(diff)
		sb.WriteString("\n```\n\n")
	}

	summary := summarizeForGrading(result)
	if summary != "" {
		sb.WriteString("### Conversation Summary\n\n")
		sb.WriteString(summary)
		sb.WriteString("\n\n")
	}
}

// summarizeForGrading condenses a variant's transcript for the grader.
// Includes thinking blocks (truncated), text responses, tool calls (name + key args),
// and tool results (truncated). Capped at 50K chars total.
func summarizeForGrading(result *EntryResult) string {
	if len(result.Transcript) == 0 {
		return ""
	}

	var sb strings.Builder
	const maxTotal = 50000
	const maxThinking = 2000
	const maxToolResult = 500

	for _, msg := range result.Transcript {
		if sb.Len() >= maxTotal {
			sb.WriteString("\n...(transcript truncated)")
			break
		}

		if msg.Message == nil {
			continue
		}

		for _, block := range msg.Message.Content {
			if sb.Len() >= maxTotal {
				break
			}

			switch {
			case block.IsThinking():
				thinking := block.Thinking
				if len(thinking) > maxThinking {
					thinking = thinking[:maxThinking] + "...(truncated)"
				}
				sb.WriteString(fmt.Sprintf("[Thinking]: %s\n", thinking))

			case block.IsText():
				sb.WriteString(fmt.Sprintf("[Text]: %s\n", block.Text))

			case block.IsToolUse():
				args := summarizeToolArgs(block.Name, block.Input)
				sb.WriteString(fmt.Sprintf("[Tool: %s] %s\n", block.Name, args))

			case block.IsToolResult():
				content := block.Content
				if len(content) > maxToolResult {
					content = content[:maxToolResult] + "...(truncated)"
				}
				sb.WriteString(fmt.Sprintf("[Result]: %s\n", content))
			}
		}
	}

	return sb.String()
}

// summarizeToolArgs extracts key arguments based on tool name.
func summarizeToolArgs(name string, input map[string]any) string {
	switch name {
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			if len(cmd) > 200 {
				cmd = cmd[:200] + "..."
			}
			return fmt.Sprintf("command=%q", cmd)
		}
	case "Read":
		if fp, ok := input["file_path"].(string); ok {
			return fmt.Sprintf("file_path=%q", fp)
		}
	case "Edit":
		if fp, ok := input["file_path"].(string); ok {
			return fmt.Sprintf("file_path=%q", fp)
		}
	case "Write":
		if fp, ok := input["file_path"].(string); ok {
			return fmt.Sprintf("file_path=%q", fp)
		}
	case "Glob":
		if p, ok := input["pattern"].(string); ok {
			return fmt.Sprintf("pattern=%q", p)
		}
	case "Grep":
		if p, ok := input["pattern"].(string); ok {
			return fmt.Sprintf("pattern=%q", p)
		}
	}

	// Fallback: show all keys
	var keys []string
	for k := range input {
		keys = append(keys, k)
	}
	if len(keys) > 0 {
		return strings.Join(keys, ", ")
	}
	return ""
}

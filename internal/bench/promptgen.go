package bench

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	claude "github.com/MateoSegura/claudesdk-go"
	"github.com/MateoSegura/claudesdk-go/internal/corpus"
)

// PromptGenerator uses the Claude SDK to generate optimal prompts for each
// benchmark entry and variant.
type PromptGenerator struct {
	model     string
	skillName string
	skillPath string
	log       *log.Logger

	mu    sync.Mutex
	cache map[string]string // "entryID:variant" -> prompt
}

// NewPromptGenerator creates a prompt generator that uses Claude to craft
// optimal prompts for benchmark entries.
func NewPromptGenerator(model, skillName, skillPath string) *PromptGenerator {
	return &PromptGenerator{
		model:     model,
		skillName: skillName,
		skillPath: skillPath,
		log:       log.New(os.Stderr, "[promptgen] ", log.LstdFlags),
		cache:     make(map[string]string),
	}
}

// Generate creates an optimal prompt for the given entry and variant using
// Claude as a prompt engineer. Results are cached per entry+variant.
// On failure, returns a fallback prompt matching the previous hardcoded template.
func (pg *PromptGenerator) Generate(ctx context.Context, entry corpus.Entry, variant Variant) (string, error) {
	key := fmt.Sprintf("%s:%s", entry.ID, variant)

	pg.mu.Lock()
	if cached, ok := pg.cache[key]; ok {
		pg.mu.Unlock()
		return cached, nil
	}
	pg.mu.Unlock()

	prompt, err := pg.generate(ctx, entry, variant)
	if err != nil {
		pg.log.Printf("warning: prompt generation failed for %s, using fallback: %v", key, err)
		prompt = fallbackPrompt(entry, variant, pg.skillName)
	}

	pg.mu.Lock()
	pg.cache[key] = prompt
	pg.mu.Unlock()

	return prompt, nil
}

func (pg *PromptGenerator) generate(ctx context.Context, entry corpus.Entry, variant Variant) (string, error) {
	metaPrompt := pg.buildMetaPrompt(entry, variant)

	session, err := claude.NewSession(claude.SessionConfig{
		LaunchOptions: claude.LaunchOptions{
			Model:           pg.model,
			MaxTurns:        1,
			SkipPermissions: true,
		},
	})
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}

	result, err := session.CollectAll(ctx, metaPrompt)
	if err != nil {
		return "", fmt.Errorf("running prompt generation: %w", err)
	}

	result = strings.TrimSpace(result)
	if result == "" {
		return "", fmt.Errorf("empty prompt generated")
	}

	return result, nil
}

func (pg *PromptGenerator) buildMetaPrompt(entry corpus.Entry, variant Variant) string {
	var sb strings.Builder

	sb.WriteString("You are a prompt engineer. Your task is to write an optimal prompt for a Claude Code instance that will fix a Zephyr RTOS build failure.\n\n")
	sb.WriteString("The Claude instance will run inside a Docker container with the Zephyr project already checked out at the broken commit. ")
	sb.WriteString("It has access to: Bash, Edit, Read, Write, Glob, and Grep tools.\n\n")

	sb.WriteString("## Bug Details\n\n")
	sb.WriteString(fmt.Sprintf("- **Entry ID**: %s\n", entry.ID))
	sb.WriteString(fmt.Sprintf("- **Board**: %s\n", entry.Board))
	sb.WriteString(fmt.Sprintf("- **App Path**: %s\n", entry.AppPath))
	sb.WriteString(fmt.Sprintf("- **Difficulty**: %s\n", entry.Difficulty))
	sb.WriteString(fmt.Sprintf("- **Build Command**: %s\n", entry.Evaluation.Command))
	sb.WriteString(fmt.Sprintf("- **Error Description**: %s\n\n", strings.TrimSpace(entry.Description)))

	if variant == WithSkill && pg.skillPath != "" {
		sb.WriteString("## Skill Context\n\n")
		sb.WriteString(fmt.Sprintf("The Claude instance has a coding skill named '%s' already installed in the workspace (.claude/skills/). ", pg.skillName))
		sb.WriteString("Claude will automatically have access to this skill's guidance — do NOT instruct Claude to explicitly load or invoke the skill. Instead, incorporate the skill's key insights directly into your prompt.\n\n")

		// Try to include skill file content
		skillContent := pg.loadSkillContent()
		if skillContent != "" {
			sb.WriteString("Here is the skill content for reference:\n\n")
			sb.WriteString("```\n")
			sb.WriteString(skillContent)
			sb.WriteString("\n```\n\n")
			sb.WriteString("Incorporate the key insights from this skill into your prompt to guide Claude's approach.\n\n")
		}
	}

	sb.WriteString("## Requirements for the Generated Prompt\n\n")
	sb.WriteString("1. The prompt should instruct Claude to run the build command first to see the actual error\n")
	sb.WriteString("2. It should guide Claude to diagnose the root cause before attempting fixes\n")
	sb.WriteString("3. It should emphasize fixing source code only — not build configuration or board settings\n")
	sb.WriteString("4. It should encourage minimal, targeted changes\n")
	sb.WriteString("5. It should instruct Claude to verify the fix by re-running the build command\n\n")

	sb.WriteString("Write ONLY the prompt text. Do not include any preamble, explanation, or markdown formatting around the prompt. ")
	sb.WriteString("The output will be used directly as the prompt for the Claude instance.")

	return sb.String()
}

func (pg *PromptGenerator) loadSkillContent() string {
	if pg.skillPath == "" {
		return ""
	}

	// Look for common skill file names
	candidates := []string{"SKILL.md", "skill.md", "README.md"}
	for _, name := range candidates {
		data, err := os.ReadFile(filepath.Join(pg.skillPath, name))
		if err == nil {
			content := string(data)
			// Cap at 10K chars to keep meta-prompt reasonable
			if len(content) > 10000 {
				content = content[:10000] + "\n...(truncated)"
			}
			return content
		}
	}
	return ""
}

// fallbackPrompt returns the original hardcoded prompt template, used when
// prompt generation fails or is disabled.
func fallbackPrompt(entry corpus.Entry, variant Variant, skillName string) string {
	var sb strings.Builder

	if variant == WithSkill && skillName != "" {
		sb.WriteString(fmt.Sprintf("Before starting, run /%s to load the coding standard.\n\n", skillName))
	}

	sb.WriteString("The following Zephyr project has a build failure.\n\n")
	sb.WriteString(fmt.Sprintf("Board: %s\n", entry.Board))
	sb.WriteString(fmt.Sprintf("Build command: %s\n", entry.Evaluation.Command))
	sb.WriteString(fmt.Sprintf("Error description: %s\n\n", strings.TrimSpace(entry.Description)))
	sb.WriteString("Run the build command to see the error, diagnose the root cause, and fix the source code so the build succeeds. Do not modify the build command or board configuration -- fix the source code only.")

	return sb.String()
}

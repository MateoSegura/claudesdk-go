package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WriteResults writes both results.json and report.md to the output directory.
func WriteResults(result *BenchmarkResult, outputDir string) error {
	dir := filepath.Join(outputDir, result.RunID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	if err := writeJSON(result, dir); err != nil {
		return err
	}
	return writeMarkdown(result, dir)
}

func writeJSON(result *BenchmarkResult, dir string) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling results: %w", err)
	}
	path := filepath.Join(dir, "results.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing results.json: %w", err)
	}
	return nil
}

func writeMarkdown(result *BenchmarkResult, dir string) error {
	var sb strings.Builder

	sb.WriteString("# Configbench Report\n\n")
	sb.WriteString(fmt.Sprintf("- **Run ID**: %s\n", result.RunID))
	sb.WriteString(fmt.Sprintf("- **Date**: %s\n", result.Timestamp.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("- **Corpus**: %s\n", result.Config.CorpusName))
	sb.WriteString(fmt.Sprintf("- **Skill**: %s\n", result.Config.SkillName))
	sb.WriteString(fmt.Sprintf("- **Max Turns**: %d\n", result.Config.MaxTurns))
	sb.WriteString(fmt.Sprintf("- **Model**: %s\n", result.Config.Model))
	sb.WriteString(fmt.Sprintf("- **Entries**: %d\n\n", result.Config.EntryCount))

	// Partition results by variant
	withSkill := filterByVariant(result.Results, WithSkill)
	withoutSkill := filterByVariant(result.Results, WithoutSkill)

	hasAB := len(withSkill) > 0 && len(withoutSkill) > 0

	if hasAB {
		writeSummary(&sb, withSkill, withoutSkill)
		writeComparisonTable(&sb, withSkill, withoutSkill)
	}

	// Grading sections
	if len(result.Comparisons) > 0 {
		writeGradingSummary(&sb, result.Comparisons)
		writeDetailedGrades(&sb, result.Comparisons)
	}

	writeDetailedMetrics(&sb, result.Results)
	writePerEntryDetails(&sb, result.Results, withSkill, withoutSkill)

	path := filepath.Join(dir, "report.md")
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("writing report.md: %w", err)
	}
	return nil
}

func writeSummary(sb *strings.Builder, withSkill, withoutSkill []EntryResult) {
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Metric | With Skill | Without Skill |\n")
	sb.WriteString("|--------|-----------|---------------|\n")

	wPass, wTotal := countPass(withSkill)
	woPass, woTotal := countPass(withoutSkill)
	sb.WriteString(fmt.Sprintf("| Pass Rate | %d/%d (%s) | %d/%d (%s) |\n",
		wPass, wTotal, pct(wPass, wTotal),
		woPass, woTotal, pct(woPass, woTotal)))

	sb.WriteString(fmt.Sprintf("| Total Cost | $%.2f | $%.2f |\n",
		sumTotalCost(withSkill), sumTotalCost(withoutSkill)))

	sb.WriteString(fmt.Sprintf("| Avg Turns | %.1f | %.1f |\n",
		avgTurns(withSkill), avgTurns(withoutSkill)))

	sb.WriteString(fmt.Sprintf("| Avg Duration | %s | %s |\n\n",
		avgDuration(withSkill), avgDuration(withoutSkill)))
}

func writeComparisonTable(sb *strings.Builder, withSkill, withoutSkill []EntryResult) {
	sb.WriteString("## Results by Entry\n\n")
	sb.WriteString("| Entry | Difficulty | With Skill | Without Skill | Skill Helped? |\n")
	sb.WriteString("|-------|-----------|-----------|---------------|---------------|\n")

	woIndex := make(map[string]EntryResult)
	for _, r := range withoutSkill {
		woIndex[r.EntryID] = r
	}

	for _, ws := range withSkill {
		wo, ok := woIndex[ws.EntryID]
		helped := "--"
		if ok {
			helped = skillHelped(ws, wo)
		}

		wStatus := passStatus(ws)
		woStatus := "--"
		if ok {
			woStatus = passStatus(wo)
		}

		// We don't have difficulty in EntryResult, so leave blank
		sb.WriteString(fmt.Sprintf("| %s | | %s | %s | %s |\n",
			ws.EntryID, wStatus, woStatus, helped))
	}
	sb.WriteString("\n")
}

func writeGradingSummary(sb *strings.Builder, comparisons []EntryComparison) {
	sb.WriteString("## Grading Summary\n\n")
	sb.WriteString("| Entry | Skill Score (/50) | No-Skill Score (/50) | Verdict |\n")
	sb.WriteString("|-------|-------------------|----------------------|---------|\n")

	for _, c := range comparisons {
		if c.Grade == nil {
			sb.WriteString(fmt.Sprintf("| %s | -- | -- | *not graded* |\n", c.EntryID))
			continue
		}

		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %s |\n",
			c.EntryID,
			c.Grade.WithSkillGrade.Total(),
			c.Grade.WithoutSkillGrade.Total(),
			verdictLabel(c.Grade.Verdict)))
	}
	sb.WriteString("\n")
}

func writeDetailedGrades(sb *strings.Builder, comparisons []EntryComparison) {
	sb.WriteString("## Detailed Grades\n\n")

	for _, c := range comparisons {
		if c.Grade == nil {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s\n\n", c.EntryID))

		sb.WriteString("| Dimension | With Skill | Without Skill |\n")
		sb.WriteString("|-----------|-----------|---------------|\n")
		sb.WriteString(fmt.Sprintf("| Correctness | %d | %d |\n",
			c.Grade.WithSkillGrade.Correctness, c.Grade.WithoutSkillGrade.Correctness))
		sb.WriteString(fmt.Sprintf("| Code Quality | %d | %d |\n",
			c.Grade.WithSkillGrade.CodeQuality, c.Grade.WithoutSkillGrade.CodeQuality))
		sb.WriteString(fmt.Sprintf("| Diagnosis | %d | %d |\n",
			c.Grade.WithSkillGrade.Diagnosis, c.Grade.WithoutSkillGrade.Diagnosis))
		sb.WriteString(fmt.Sprintf("| Minimality | %d | %d |\n",
			c.Grade.WithSkillGrade.Minimality, c.Grade.WithoutSkillGrade.Minimality))
		sb.WriteString(fmt.Sprintf("| Efficiency | %d | %d |\n",
			c.Grade.WithSkillGrade.Efficiency, c.Grade.WithoutSkillGrade.Efficiency))
		sb.WriteString(fmt.Sprintf("| **Total** | **%d** | **%d** |\n\n",
			c.Grade.WithSkillGrade.Total(), c.Grade.WithoutSkillGrade.Total()))

		sb.WriteString(fmt.Sprintf("**Verdict**: %s\n\n", verdictLabel(c.Grade.Verdict)))
		sb.WriteString(fmt.Sprintf("**Reasoning**: %s\n\n", c.Grade.Reasoning))
	}
}

func writeDetailedMetrics(sb *strings.Builder, results []EntryResult) {
	sb.WriteString("## Detailed Metrics\n\n")
	sb.WriteString("| Entry | Variant | Pass | Turns | Cost | Tokens (In/Out) | Cache (Create/Read) | Duration (Total/API) | Wall Clock |\n")
	sb.WriteString("|-------|---------|------|-------|------|-----------------|---------------------|----------------------|------------|\n")

	for _, r := range results {
		tokens := fmt.Sprintf("%d/%d", r.Metrics.InputTokens, r.Metrics.OutputTokens)
		cache := fmt.Sprintf("%d/%d", r.Metrics.CacheCreationInputTokens, r.Metrics.CacheReadInputTokens)
		duration := fmt.Sprintf("%s/%s",
			formatDurationMS(r.Metrics.DurationMS),
			formatDurationMS(r.Metrics.DurationAPIMS))
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %d | $%.4f | %s | %s | %s | %s |\n",
			r.EntryID, r.Variant, passStatus(r), r.Metrics.Turns,
			r.Metrics.TotalCostUSD, tokens, cache, duration, formatDuration(r.WallClock)))
	}
	sb.WriteString("\n")
}

func writePerEntryDetails(sb *strings.Builder, results []EntryResult, withSkill, withoutSkill []EntryResult) {
	sb.WriteString("## Per-Entry Details\n\n")

	// Group by entry ID
	entries := make(map[string][]EntryResult)
	var order []string
	for _, r := range results {
		if _, seen := entries[r.EntryID]; !seen {
			order = append(order, r.EntryID)
		}
		entries[r.EntryID] = append(entries[r.EntryID], r)
	}

	for _, id := range order {
		rs := entries[id]
		sb.WriteString(fmt.Sprintf("### %s\n\n", id))

		for _, r := range rs {
			sb.WriteString(fmt.Sprintf("**%s**: %s\n", r.Variant, passStatus(r)))
			if r.Error != "" {
				sb.WriteString(fmt.Sprintf("- Error: %s\n", r.Error))
			}
			sb.WriteString(fmt.Sprintf("- Cost: $%.4f | Turns: %d | Tokens: %d in / %d out | Wall: %s\n",
				r.Metrics.TotalCostUSD, r.Metrics.Turns,
				r.Metrics.InputTokens, r.Metrics.OutputTokens,
				formatDuration(r.WallClock)))
			if r.Metrics.CacheCreationInputTokens > 0 || r.Metrics.CacheReadInputTokens > 0 {
				sb.WriteString(fmt.Sprintf("- Cache: %d created / %d read\n",
					r.Metrics.CacheCreationInputTokens, r.Metrics.CacheReadInputTokens))
			}
			if r.Metrics.DurationAPIMS > 0 {
				sb.WriteString(fmt.Sprintf("- API Duration: %s\n", formatDurationMS(r.Metrics.DurationAPIMS)))
			}
			sb.WriteString("\n")
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func filterByVariant(results []EntryResult, v Variant) []EntryResult {
	var out []EntryResult
	for _, r := range results {
		if r.Variant == v {
			out = append(out, r)
		}
	}
	return out
}

func countPass(results []EntryResult) (pass, total int) {
	for _, r := range results {
		total++
		if r.BuildPass {
			pass++
		}
	}
	return
}

func pct(n, d int) string {
	if d == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.0f%%", float64(n)/float64(d)*100)
}

func sumTotalCost(results []EntryResult) float64 {
	var total float64
	for _, r := range results {
		total += r.Metrics.TotalCostUSD
	}
	return total
}

func avgTurns(results []EntryResult) float64 {
	if len(results) == 0 {
		return 0
	}
	var total int
	for _, r := range results {
		total += r.Metrics.Turns
	}
	return float64(total) / float64(len(results))
}

func avgDuration(results []EntryResult) string {
	if len(results) == 0 {
		return "0s"
	}
	var total time.Duration
	for _, r := range results {
		total += r.WallClock
	}
	return formatDuration(total / time.Duration(len(results)))
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", m, s)
}

func formatDurationMS(ms int64) string {
	if ms <= 0 {
		return "--"
	}
	return formatDuration(time.Duration(ms) * time.Millisecond)
}

func passStatus(r EntryResult) string {
	if r.Error != "" {
		return "ERROR"
	}
	if r.BuildPass {
		return "PASS"
	}
	return "FAIL"
}

func skillHelped(with, without EntryResult) string {
	if with.Error != "" || without.Error != "" {
		return "--"
	}
	switch {
	case with.BuildPass && !without.BuildPass:
		return "Yes"
	case !with.BuildPass && without.BuildPass:
		return "Hurt"
	case with.BuildPass && without.BuildPass:
		return "No (both pass)"
	default:
		return "No"
	}
}

func verdictLabel(verdict string) string {
	switch verdict {
	case "skill_better":
		return "Skill Better"
	case "no_skill_better":
		return "No-Skill Better"
	case "tie":
		return "Tie"
	case "inconclusive":
		return "Inconclusive"
	default:
		return verdict
	}
}

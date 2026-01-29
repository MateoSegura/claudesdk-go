package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RichMetrics holds full metric data extracted from Claude's stream-json output.
type RichMetrics struct {
	TotalCostUSD             float64       `json:"total_cost_usd"`
	CostUSD                  float64       `json:"cost_usd"`
	Turns                    int           `json:"num_turns"`
	InputTokens              int           `json:"input_tokens"`
	OutputTokens             int           `json:"output_tokens"`
	CacheCreationInputTokens int           `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int           `json:"cache_read_input_tokens"`
	DurationMS               int64         `json:"duration_ms"`
	DurationAPIMS            int64         `json:"duration_api_ms"`
	WallClock                time.Duration `json:"wall_clock_ns"`
	Model                    string        `json:"model,omitempty"`
	SessionID                string        `json:"session_id,omitempty"`
	IsError                  bool          `json:"is_error,omitempty"`
	ResultSubtype            string        `json:"result_subtype,omitempty"`
}

// VariantGrade holds per-variant grading scores.
type VariantGrade struct {
	Correctness int `json:"correctness"`
	CodeQuality int `json:"code_quality"`
	Diagnosis   int `json:"diagnosis"`
	Minimality  int `json:"minimality"`
	Efficiency  int `json:"efficiency"`
}

// Total returns the sum of all dimension scores.
func (vg *VariantGrade) Total() int {
	return vg.Correctness + vg.CodeQuality + vg.Diagnosis + vg.Minimality + vg.Efficiency
}

// GradeResult holds structured grading output from the Claude grader.
type GradeResult struct {
	WithSkillGrade    VariantGrade `json:"with_skill_grade"`
	WithoutSkillGrade VariantGrade `json:"without_skill_grade"`
	Verdict           string       `json:"verdict"`
	Reasoning         string       `json:"reasoning"`
}

// EntryComparison groups both variant results and their grade for one entry.
// WithSkill/WithoutSkill are runtime-only references (not serialized to avoid
// duplicating the full EntryResult in results.json). Use the EntryID to
// cross-reference with BenchmarkResult.Results.
type EntryComparison struct {
	EntryID      string       `json:"entry_id"`
	WithSkill    *EntryResult `json:"-"`
	WithoutSkill *EntryResult `json:"-"`
	Grade        *GradeResult `json:"grade,omitempty"`
}

// OutputWriter writes per-entry/variant artifacts to disk.
type OutputWriter struct {
	baseDir string
}

// NewOutputWriter creates an OutputWriter rooted at outputDir/runID.
func NewOutputWriter(outputDir, runID string) (*OutputWriter, error) {
	base := filepath.Join(outputDir, runID)
	if err := os.MkdirAll(base, 0755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}
	return &OutputWriter{baseDir: base}, nil
}

// WriteVariantOutput writes all per-variant artifacts: transcript.json,
// stream.jsonl, metrics.json, and claude_changes.diff.
func (w *OutputWriter) WriteVariantOutput(result *EntryResult) error {
	dir := filepath.Join(w.baseDir, result.EntryID, string(result.Variant))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating variant dir: %w", err)
	}

	// transcript.json — parsed []StreamMessage
	if len(result.Transcript) > 0 {
		data, err := json.MarshalIndent(result.Transcript, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling transcript: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "transcript.json"), data, 0644); err != nil {
			return fmt.Errorf("writing transcript.json: %w", err)
		}
	}

	// stream.jsonl — raw docker exec stdout
	if result.RawStream != "" {
		if err := os.WriteFile(filepath.Join(dir, "stream.jsonl"), []byte(result.RawStream), 0644); err != nil {
			return fmt.Errorf("writing stream.jsonl: %w", err)
		}
	}

	// metrics.json — RichMetrics
	metricsData, err := json.MarshalIndent(result.Metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metrics: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metrics.json"), metricsData, 0644); err != nil {
		return fmt.Errorf("writing metrics.json: %w", err)
	}

	// claude_changes.diff — git diff of what Claude changed
	if result.Diff != "" {
		if err := os.WriteFile(filepath.Join(dir, "claude_changes.diff"), []byte(result.Diff), 0644); err != nil {
			return fmt.Errorf("writing claude_changes.diff: %w", err)
		}
	}

	return nil
}

// WriteGrade writes the grading result for an entry.
func (w *OutputWriter) WriteGrade(entryID string, grade *GradeResult) error {
	dir := filepath.Join(w.baseDir, entryID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating entry dir: %w", err)
	}

	data, err := json.MarshalIndent(grade, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling grade: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "grade.json"), data, 0644)
}

// WritePrompt writes the prompt used for a variant.
func (w *OutputWriter) WritePrompt(entryID string, variant Variant, prompt string) error {
	dir := filepath.Join(w.baseDir, entryID, string(variant))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating variant dir: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "prompt.txt"), []byte(prompt), 0644)
}

// gradeJSONSchema returns the JSON schema for structured grading output.
func gradeJSONSchema() map[string]any {
	variantGradeSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"correctness":  map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
			"code_quality": map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
			"diagnosis":    map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
			"minimality":   map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
			"efficiency":   map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
		},
		"required":             []string{"correctness", "code_quality", "diagnosis", "minimality", "efficiency"},
		"additionalProperties": false,
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"with_skill_grade":    variantGradeSchema,
			"without_skill_grade": variantGradeSchema,
			"verdict": map[string]any{
				"type": "string",
				"enum": []string{"skill_better", "no_skill_better", "tie", "inconclusive"},
			},
			"reasoning": map[string]any{"type": "string"},
		},
		"required":             []string{"with_skill_grade", "without_skill_grade", "verdict", "reasoning"},
		"additionalProperties": false,
	}
}

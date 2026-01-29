package bench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MateoSegura/claudesdk-go/internal/corpus"
)

// testStreamJSONL is a realistic stream-json fixture extracted from an actual
// benchmark run against zephyr-native-sim. It contains: init, assistant (text),
// assistant (tool_use: Skill), user (tool_result), assistant (text + tool_use:
// Read + tool_use: Bash), user (tool_results), assistant (text + tool_use: Edit),
// user (tool_result), assistant (text + tool_use: Bash), user (tool_result),
// assistant (text), result.
const testStreamJSONL = `{"type":"system","subtype":"init","cwd":"/root/zephyrproject/zephyr","session_id":"040873fd-e7a9-4bc7-ae19-359f6fa74be2","tools":["Bash","Edit","Read","Write","Glob","Grep"],"model":"claude-opus-4-5-20251101","permissionMode":"default"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I'll start by examining the source file."}]},"session_id":"040873fd-e7a9-4bc7-ae19-359f6fa74be2"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01Read","name":"Read","input":{"file_path":"/root/zephyrproject/zephyr/samples/synchronization/src/main.c"}}]},"session_id":"040873fd-e7a9-4bc7-ae19-359f6fa74be2"}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01Read","content":"#include <zephyr/sys/printk.h>\nvoid main(void) { }"}]},"session_id":"040873fd-e7a9-4bc7-ae19-359f6fa74be2"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"The kernel header is missing. I need to add #include <zephyr/kernel.h>."},{"type":"text","text":"The root cause is clear: the kernel header is missing."},{"type":"tool_use","id":"toolu_01Edit","name":"Edit","input":{"file_path":"/root/zephyrproject/zephyr/samples/synchronization/src/main.c","old_string":"#include <zephyr/sys/printk.h>","new_string":"#include <zephyr/kernel.h>\n#include <zephyr/sys/printk.h>"}}]},"session_id":"040873fd-e7a9-4bc7-ae19-359f6fa74be2"}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01Edit","content":"File updated successfully."}]},"session_id":"040873fd-e7a9-4bc7-ae19-359f6fa74be2"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01Bash","name":"Bash","input":{"command":"west build -b native_sim samples/synchronization -p"}}]},"session_id":"040873fd-e7a9-4bc7-ae19-359f6fa74be2"}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01Bash","content":"[95/95] Running utility command for native_runner_executable"}]},"session_id":"040873fd-e7a9-4bc7-ae19-359f6fa74be2"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Build succeeds. The fix was to add the missing kernel header."}]},"session_id":"040873fd-e7a9-4bc7-ae19-359f6fa74be2"}
{"type":"result","subtype":"success","is_error":false,"duration_ms":33551,"duration_api_ms":24484,"num_turns":7,"result":"Build succeeds.","session_id":"040873fd-e7a9-4bc7-ae19-359f6fa74be2","total_cost_usd":0.14095825,"usage":{"input_tokens":7,"cache_creation_input_tokens":9277,"cache_read_input_tokens":115944,"output_tokens":850}}
`

// ---------------------------------------------------------------------------
// parseTranscript
// ---------------------------------------------------------------------------

func TestParseTranscript(t *testing.T) {
	messages := parseTranscript(testStreamJSONL)

	if len(messages) != 10 {
		t.Fatalf("expected 10 messages, got %d", len(messages))
	}

	// Verify message types in order
	expectedTypes := []string{
		"system", "assistant", "assistant", "user",
		"assistant", "user", "assistant", "user",
		"assistant", "result",
	}
	for i, msg := range messages {
		if msg.Type != expectedTypes[i] {
			t.Errorf("message[%d]: expected type %q, got %q", i, expectedTypes[i], msg.Type)
		}
	}

	// Verify init message
	if messages[0].Subtype != "init" {
		t.Errorf("first message subtype: expected 'init', got %q", messages[0].Subtype)
	}
	if messages[0].Model != "claude-opus-4-5-20251101" {
		t.Errorf("init model: expected 'claude-opus-4-5-20251101', got %q", messages[0].Model)
	}

	// Verify result message
	last := messages[len(messages)-1]
	if last.Subtype != "success" {
		t.Errorf("result subtype: expected 'success', got %q", last.Subtype)
	}
	if last.NumTurns != 7 {
		t.Errorf("result num_turns: expected 7, got %d", last.NumTurns)
	}
}

func TestParseTranscriptEmpty(t *testing.T) {
	messages := parseTranscript("")
	if len(messages) != 0 {
		t.Errorf("expected 0 messages for empty input, got %d", len(messages))
	}
}

func TestParseTranscriptGarbage(t *testing.T) {
	input := "not json\n{invalid\n\n"
	messages := parseTranscript(input)
	if len(messages) != 0 {
		t.Errorf("expected 0 messages for garbage input, got %d", len(messages))
	}
}

func TestParseTranscriptSkipsNonJSON(t *testing.T) {
	input := "some log output\n" + `{"type":"result","subtype":"success","num_turns":1}` + "\nmore log\n"
	messages := parseTranscript(input)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Type != "result" {
		t.Errorf("expected type 'result', got %q", messages[0].Type)
	}
}

// ---------------------------------------------------------------------------
// parseRichMetrics
// ---------------------------------------------------------------------------

func TestParseRichMetrics(t *testing.T) {
	metrics := parseRichMetrics(testStreamJSONL)

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"TotalCostUSD", metrics.TotalCostUSD, 0.14095825},
		{"Turns", metrics.Turns, 7},
		{"InputTokens", metrics.InputTokens, 7},
		{"OutputTokens", metrics.OutputTokens, 850},
		{"CacheCreationInputTokens", metrics.CacheCreationInputTokens, 9277},
		{"CacheReadInputTokens", metrics.CacheReadInputTokens, 115944},
		{"DurationMS", metrics.DurationMS, int64(33551)},
		{"DurationAPIMS", metrics.DurationAPIMS, int64(24484)},
		{"SessionID", metrics.SessionID, "040873fd-e7a9-4bc7-ae19-359f6fa74be2"},
		{"ResultSubtype", metrics.ResultSubtype, "success"},
		{"IsError", metrics.IsError, false},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestParseRichMetricsModelFromInit(t *testing.T) {
	// Model is NOT in the result line — it comes from the init message.
	metrics := parseRichMetrics(testStreamJSONL)

	if metrics.Model != "claude-opus-4-5-20251101" {
		t.Errorf("Model: got %q, want %q", metrics.Model, "claude-opus-4-5-20251101")
	}
}

func TestParseRichMetricsNoInit(t *testing.T) {
	// Result only, no init message — model should be empty
	input := `{"type":"result","subtype":"success","num_turns":3,"total_cost_usd":0.05,"duration_ms":1000,"duration_api_ms":500,"usage":{"input_tokens":100,"output_tokens":50}}`
	metrics := parseRichMetrics(input)

	if metrics.Model != "" {
		t.Errorf("expected empty model without init, got %q", metrics.Model)
	}
	if metrics.Turns != 3 {
		t.Errorf("Turns: got %d, want 3", metrics.Turns)
	}
	if metrics.TotalCostUSD != 0.05 {
		t.Errorf("TotalCostUSD: got %f, want 0.05", metrics.TotalCostUSD)
	}
}

func TestParseRichMetricsEmpty(t *testing.T) {
	metrics := parseRichMetrics("")
	if metrics.Turns != 0 || metrics.TotalCostUSD != 0 || metrics.Model != "" {
		t.Errorf("expected zero metrics for empty input, got: %+v", metrics)
	}
}

func TestParseRichMetricsErrorResult(t *testing.T) {
	input := `{"type":"system","subtype":"init","model":"claude-sonnet-4-20250514","session_id":"abc123"}
{"type":"result","subtype":"error_max_turns","is_error":true,"num_turns":10,"total_cost_usd":0.50,"duration_ms":60000,"session_id":"abc123","usage":{"input_tokens":5000,"output_tokens":2000}}`
	metrics := parseRichMetrics(input)

	if !metrics.IsError {
		t.Error("expected IsError=true for error result")
	}
	if metrics.ResultSubtype != "error_max_turns" {
		t.Errorf("ResultSubtype: got %q, want 'error_max_turns'", metrics.ResultSubtype)
	}
	if metrics.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model: got %q, want 'claude-sonnet-4-20250514'", metrics.Model)
	}
}

// ---------------------------------------------------------------------------
// summarizeForGrading
// ---------------------------------------------------------------------------

func TestSummarizeForGrading(t *testing.T) {
	transcript := parseTranscript(testStreamJSONL)
	result := &EntryResult{Transcript: transcript}

	summary := summarizeForGrading(result)

	if summary == "" {
		t.Fatal("expected non-empty summary")
	}

	// Should contain thinking blocks
	if !strings.Contains(summary, "[Thinking]") {
		t.Error("summary should contain thinking blocks")
	}

	// Should contain text blocks
	if !strings.Contains(summary, "[Text]") {
		t.Error("summary should contain text blocks")
	}

	// Should contain tool calls with key args
	if !strings.Contains(summary, "[Tool: Read]") {
		t.Error("summary should contain Read tool call")
	}
	if !strings.Contains(summary, "file_path=") {
		t.Error("summary should contain file_path arg for Read tool")
	}
	if !strings.Contains(summary, "[Tool: Edit]") {
		t.Error("summary should contain Edit tool call")
	}
	if !strings.Contains(summary, "[Tool: Bash]") {
		t.Error("summary should contain Bash tool call")
	}
	if !strings.Contains(summary, "command=") {
		t.Error("summary should contain command arg for Bash tool")
	}

	// Should contain tool results
	if !strings.Contains(summary, "[Result]") {
		t.Error("summary should contain tool results")
	}
}

func TestSummarizeForGradingEmpty(t *testing.T) {
	result := &EntryResult{}
	summary := summarizeForGrading(result)
	if summary != "" {
		t.Errorf("expected empty summary for no transcript, got %q", summary)
	}
}

func TestSummarizeForGradingTruncation(t *testing.T) {
	// Build a transcript with a very long thinking block
	longThinking := strings.Repeat("x", 5000)
	input := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"` + longThinking + `"}]}}`
	transcript := parseTranscript(input)
	result := &EntryResult{Transcript: transcript}

	summary := summarizeForGrading(result)

	// Thinking should be truncated to ~2000 chars
	if !strings.Contains(summary, "...(truncated)") {
		t.Error("long thinking block should be truncated")
	}
}

// ---------------------------------------------------------------------------
// fallbackPrompt
// ---------------------------------------------------------------------------

func TestFallbackPromptWithSkill(t *testing.T) {
	entry := makeTestEntry()
	prompt := fallbackPrompt(entry, WithSkill, "my-skill")

	if !strings.Contains(prompt, "/my-skill") {
		t.Error("with-skill prompt should reference the skill")
	}
	if !strings.Contains(prompt, entry.Board) {
		t.Errorf("prompt should contain board %q", entry.Board)
	}
	if !strings.Contains(prompt, entry.Evaluation.Command) {
		t.Error("prompt should contain build command")
	}
	if !strings.Contains(prompt, "fix the source code only") {
		t.Error("prompt should instruct to fix source only")
	}
}

func TestFallbackPromptWithoutSkill(t *testing.T) {
	entry := makeTestEntry()
	prompt := fallbackPrompt(entry, WithoutSkill, "my-skill")

	if strings.Contains(prompt, "/my-skill") {
		t.Error("without-skill prompt should NOT reference the skill")
	}
	if !strings.Contains(prompt, entry.Board) {
		t.Errorf("prompt should contain board %q", entry.Board)
	}
}

func TestFallbackPromptNoSkillName(t *testing.T) {
	entry := makeTestEntry()
	prompt := fallbackPrompt(entry, WithSkill, "")

	if strings.Contains(prompt, "Before starting") {
		t.Error("prompt should not have skill instruction when skillName is empty")
	}
}

// ---------------------------------------------------------------------------
// VariantGrade
// ---------------------------------------------------------------------------

func TestVariantGradeTotal(t *testing.T) {
	vg := VariantGrade{
		Correctness: 8,
		CodeQuality: 7,
		Diagnosis:   9,
		Minimality:  6,
		Efficiency:  5,
	}
	if vg.Total() != 35 {
		t.Errorf("Total: got %d, want 35", vg.Total())
	}
}

func TestVariantGradeTotalZero(t *testing.T) {
	vg := VariantGrade{}
	if vg.Total() != 0 {
		t.Errorf("Total: got %d, want 0", vg.Total())
	}
}

// ---------------------------------------------------------------------------
// gradeJSONSchema
// ---------------------------------------------------------------------------

func TestGradeJSONSchema(t *testing.T) {
	schema := gradeJSONSchema()

	// Should be a valid JSON-serializable structure
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("schema should be JSON-serializable: %v", err)
	}

	// Verify required fields
	schemaStr := string(data)
	for _, field := range []string{
		"with_skill_grade", "without_skill_grade", "verdict", "reasoning",
		"correctness", "code_quality", "diagnosis", "minimality", "efficiency",
	} {
		if !strings.Contains(schemaStr, field) {
			t.Errorf("schema should contain field %q", field)
		}
	}

	// Verify verdict enum
	if !strings.Contains(schemaStr, "skill_better") || !strings.Contains(schemaStr, "no_skill_better") {
		t.Error("schema should contain verdict enum values")
	}
}

// ---------------------------------------------------------------------------
// OutputWriter
// ---------------------------------------------------------------------------

func TestOutputWriter(t *testing.T) {
	tmpDir := t.TempDir()

	ow, err := NewOutputWriter(tmpDir, "test-run-123")
	if err != nil {
		t.Fatalf("NewOutputWriter: %v", err)
	}

	transcript := parseTranscript(testStreamJSONL)
	metrics := parseRichMetrics(testStreamJSONL)

	result := &EntryResult{
		EntryID:    "sync-missing-kernel-header",
		Variant:    WithSkill,
		BuildPass:  true,
		Metrics:    metrics,
		Transcript: transcript,
		RawStream:  testStreamJSONL,
		Diff:       "diff --git a/main.c b/main.c\n+#include <zephyr/kernel.h>",
	}

	if err := ow.WriteVariantOutput(result); err != nil {
		t.Fatalf("WriteVariantOutput: %v", err)
	}

	variantDir := filepath.Join(tmpDir, "test-run-123", "sync-missing-kernel-header", "with-skill")

	// Verify transcript.json
	data, err := os.ReadFile(filepath.Join(variantDir, "transcript.json"))
	if err != nil {
		t.Fatalf("reading transcript.json: %v", err)
	}
	if !strings.Contains(string(data), "claude-opus-4-5-20251101") {
		t.Error("transcript.json should contain model name")
	}

	// Verify stream.jsonl
	data, err = os.ReadFile(filepath.Join(variantDir, "stream.jsonl"))
	if err != nil {
		t.Fatalf("reading stream.jsonl: %v", err)
	}
	if !strings.Contains(string(data), `"type":"result"`) {
		t.Error("stream.jsonl should contain result line")
	}

	// Verify metrics.json
	data, err = os.ReadFile(filepath.Join(variantDir, "metrics.json"))
	if err != nil {
		t.Fatalf("reading metrics.json: %v", err)
	}
	var readMetrics RichMetrics
	if err := json.Unmarshal(data, &readMetrics); err != nil {
		t.Fatalf("parsing metrics.json: %v", err)
	}
	if readMetrics.Turns != 7 {
		t.Errorf("metrics.json Turns: got %d, want 7", readMetrics.Turns)
	}
	if readMetrics.CacheCreationInputTokens != 9277 {
		t.Errorf("metrics.json CacheCreation: got %d, want 9277", readMetrics.CacheCreationInputTokens)
	}

	// Verify claude_changes.diff
	data, err = os.ReadFile(filepath.Join(variantDir, "claude_changes.diff"))
	if err != nil {
		t.Fatalf("reading claude_changes.diff: %v", err)
	}
	if !strings.Contains(string(data), "+#include") {
		t.Error("claude_changes.diff should contain the diff")
	}
}

func TestOutputWriterPrompt(t *testing.T) {
	tmpDir := t.TempDir()

	ow, err := NewOutputWriter(tmpDir, "run-1")
	if err != nil {
		t.Fatalf("NewOutputWriter: %v", err)
	}

	if err := ow.WritePrompt("entry-1", WithSkill, "Fix the build."); err != nil {
		t.Fatalf("WritePrompt: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "run-1", "entry-1", "with-skill", "prompt.txt"))
	if err != nil {
		t.Fatalf("reading prompt.txt: %v", err)
	}
	if string(data) != "Fix the build." {
		t.Errorf("prompt.txt: got %q, want 'Fix the build.'", string(data))
	}
}

func TestOutputWriterGrade(t *testing.T) {
	tmpDir := t.TempDir()

	ow, err := NewOutputWriter(tmpDir, "run-1")
	if err != nil {
		t.Fatalf("NewOutputWriter: %v", err)
	}

	grade := &GradeResult{
		WithSkillGrade:    VariantGrade{10, 9, 8, 7, 6},
		WithoutSkillGrade: VariantGrade{5, 4, 3, 2, 1},
		Verdict:           "skill_better",
		Reasoning:         "Skill variant was superior.",
	}

	if err := ow.WriteGrade("entry-1", grade); err != nil {
		t.Fatalf("WriteGrade: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "run-1", "entry-1", "grade.json"))
	if err != nil {
		t.Fatalf("reading grade.json: %v", err)
	}

	var readGrade GradeResult
	if err := json.Unmarshal(data, &readGrade); err != nil {
		t.Fatalf("parsing grade.json: %v", err)
	}
	if readGrade.Verdict != "skill_better" {
		t.Errorf("verdict: got %q, want 'skill_better'", readGrade.Verdict)
	}
	if readGrade.WithSkillGrade.Total() != 40 {
		t.Errorf("with_skill total: got %d, want 40", readGrade.WithSkillGrade.Total())
	}
	if readGrade.WithoutSkillGrade.Total() != 15 {
		t.Errorf("without_skill total: got %d, want 15", readGrade.WithoutSkillGrade.Total())
	}
}

func TestOutputWriterEmptyTranscript(t *testing.T) {
	tmpDir := t.TempDir()

	ow, err := NewOutputWriter(tmpDir, "run-1")
	if err != nil {
		t.Fatalf("NewOutputWriter: %v", err)
	}

	result := &EntryResult{
		EntryID: "err-entry",
		Variant: WithoutSkill,
		Error:   "setup failed",
	}

	if err := ow.WriteVariantOutput(result); err != nil {
		t.Fatalf("WriteVariantOutput with empty data: %v", err)
	}

	// metrics.json should still be written (zero-valued)
	variantDir := filepath.Join(tmpDir, "run-1", "err-entry", "without-skill")
	if _, err := os.Stat(filepath.Join(variantDir, "metrics.json")); err != nil {
		t.Error("metrics.json should be written even for error results")
	}

	// transcript.json should NOT be written (no transcript)
	if _, err := os.Stat(filepath.Join(variantDir, "transcript.json")); err == nil {
		t.Error("transcript.json should not be written when transcript is empty")
	}

	// stream.jsonl should NOT be written (empty)
	if _, err := os.Stat(filepath.Join(variantDir, "stream.jsonl")); err == nil {
		t.Error("stream.jsonl should not be written when raw stream is empty")
	}
}

// ---------------------------------------------------------------------------
// formatDurationMS
// ---------------------------------------------------------------------------

func TestFormatDurationMS(t *testing.T) {
	tests := []struct {
		ms   int64
		want string
	}{
		{0, "--"},
		{-1, "--"},
		{500, "0.5s"},
		{33551, "33.6s"},
		{120000, "2m0s"},
		{90500, "1m30s"},
	}
	for _, tt := range tests {
		got := formatDurationMS(tt.ms)
		if got != tt.want {
			t.Errorf("formatDurationMS(%d): got %q, want %q", tt.ms, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// verdictLabel
// ---------------------------------------------------------------------------

func TestVerdictLabel(t *testing.T) {
	tests := []struct {
		verdict string
		want    string
	}{
		{"skill_better", "Skill Better"},
		{"no_skill_better", "No-Skill Better"},
		{"tie", "Tie"},
		{"inconclusive", "Inconclusive"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := verdictLabel(tt.verdict)
		if got != tt.want {
			t.Errorf("verdictLabel(%q): got %q, want %q", tt.verdict, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Report generation with grades
// ---------------------------------------------------------------------------

func TestWriteMarkdownWithGrades(t *testing.T) {
	tmpDir := t.TempDir()

	result := &BenchmarkResult{
		RunID:     "test-graded-run",
		Timestamp: time.Now(),
		Config: ConfigSummary{
			CorpusName: "test-corpus",
			SkillName:  "test-skill",
			MaxTurns:   10,
			Model:      "sonnet",
			EntryCount: 1,
		},
		Results: []EntryResult{
			{
				EntryID:   "entry-1",
				Variant:   WithSkill,
				BuildPass: true,
				Metrics:   RichMetrics{TotalCostUSD: 0.10, Turns: 5, InputTokens: 1000, OutputTokens: 500, CacheCreationInputTokens: 200, CacheReadInputTokens: 800, DurationMS: 30000, DurationAPIMS: 20000},
				WallClock: 35 * time.Second,
			},
			{
				EntryID:   "entry-1",
				Variant:   WithoutSkill,
				BuildPass: false,
				Metrics:   RichMetrics{TotalCostUSD: 0.15, Turns: 8, InputTokens: 2000, OutputTokens: 1000, DurationMS: 50000, DurationAPIMS: 35000},
				WallClock: 55 * time.Second,
			},
		},
		Comparisons: []EntryComparison{
			{
				EntryID: "entry-1",
				Grade: &GradeResult{
					WithSkillGrade:    VariantGrade{9, 8, 9, 8, 7},
					WithoutSkillGrade: VariantGrade{3, 5, 4, 6, 5},
					Verdict:           "skill_better",
					Reasoning:         "The skill variant correctly fixed the bug while the no-skill variant failed.",
				},
			},
		},
	}

	if err := WriteResults(result, tmpDir); err != nil {
		t.Fatalf("WriteResults: %v", err)
	}

	// Read the report
	reportPath := filepath.Join(tmpDir, "test-graded-run", "report.md")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("reading report.md: %v", err)
	}
	report := string(data)

	// Verify grading summary section exists
	if !strings.Contains(report, "## Grading Summary") {
		t.Error("report should contain Grading Summary section")
	}
	if !strings.Contains(report, "Skill Score (/50)") {
		t.Error("report should contain score column headers")
	}
	if !strings.Contains(report, "41") { // 9+8+9+8+7
		t.Error("report should contain with-skill total score 41")
	}
	if !strings.Contains(report, "23") { // 3+5+4+6+5
		t.Error("report should contain without-skill total score 23")
	}
	if !strings.Contains(report, "Skill Better") {
		t.Error("report should contain verdict label")
	}

	// Verify detailed grades section
	if !strings.Contains(report, "## Detailed Grades") {
		t.Error("report should contain Detailed Grades section")
	}
	if !strings.Contains(report, "Correctness") {
		t.Error("detailed grades should list dimension names")
	}
	if !strings.Contains(report, "skill variant correctly fixed") {
		t.Error("detailed grades should include reasoning")
	}

	// Verify detailed metrics include cache tokens
	if !strings.Contains(report, "Cache (Create/Read)") {
		t.Error("detailed metrics should have cache column")
	}
	if !strings.Contains(report, "Duration (Total/API)") {
		t.Error("detailed metrics should have duration column")
	}
	if !strings.Contains(report, "200/800") { // cache create/read for with-skill
		t.Error("detailed metrics should show cache values")
	}

	// Verify results.json
	resultsPath := filepath.Join(tmpDir, "test-graded-run", "results.json")
	jsonData, err := os.ReadFile(resultsPath)
	if err != nil {
		t.Fatalf("reading results.json: %v", err)
	}

	// Verify comparisons don't duplicate full EntryResult
	var parsed map[string]any
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("parsing results.json: %v", err)
	}
	comparisons, ok := parsed["comparisons"].([]any)
	if !ok || len(comparisons) != 1 {
		t.Fatal("results.json should have 1 comparison")
	}
	comp := comparisons[0].(map[string]any)
	if _, hasWithSkill := comp["with_skill"]; hasWithSkill {
		t.Error("comparison should NOT serialize with_skill (json:\"-\")")
	}
	if _, hasGrade := comp["grade"]; !hasGrade {
		t.Error("comparison should serialize grade")
	}
}

func TestWriteMarkdownWithoutGrades(t *testing.T) {
	tmpDir := t.TempDir()

	result := &BenchmarkResult{
		RunID:     "no-grade-run",
		Timestamp: time.Now(),
		Config:    ConfigSummary{CorpusName: "test", EntryCount: 1},
		Results: []EntryResult{
			{EntryID: "e1", Variant: WithoutSkill, BuildPass: true, Metrics: RichMetrics{Turns: 3}},
		},
	}

	if err := WriteResults(result, tmpDir); err != nil {
		t.Fatalf("WriteResults: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "no-grade-run", "report.md"))
	if err != nil {
		t.Fatalf("reading report: %v", err)
	}
	report := string(data)

	// Should NOT have grading sections when no comparisons
	if strings.Contains(report, "Grading Summary") {
		t.Error("report without comparisons should not have Grading Summary")
	}
	if strings.Contains(report, "Detailed Grades") {
		t.Error("report without comparisons should not have Detailed Grades")
	}
}

// ---------------------------------------------------------------------------
// EntryComparison JSON serialization
// ---------------------------------------------------------------------------

func TestEntryComparisonJSON(t *testing.T) {
	comp := EntryComparison{
		EntryID:      "test-entry",
		WithSkill:    &EntryResult{EntryID: "test-entry", Variant: WithSkill, ClaudeOutput: "lots of data"},
		WithoutSkill: &EntryResult{EntryID: "test-entry", Variant: WithoutSkill, ClaudeOutput: "more data"},
		Grade: &GradeResult{
			Verdict:   "tie",
			Reasoning: "Both performed equally.",
		},
	}

	data, err := json.Marshal(comp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// WithSkill and WithoutSkill should NOT be serialized
	if strings.Contains(string(data), "lots of data") {
		t.Error("WithSkill should not be serialized (json:\"-\")")
	}
	if strings.Contains(string(data), "more data") {
		t.Error("WithoutSkill should not be serialized (json:\"-\")")
	}

	// EntryID and Grade should be serialized
	if !strings.Contains(string(data), "test-entry") {
		t.Error("EntryID should be serialized")
	}
	if !strings.Contains(string(data), "tie") {
		t.Error("Grade.Verdict should be serialized")
	}
}

// ---------------------------------------------------------------------------
// summarizeToolArgs
// ---------------------------------------------------------------------------

func TestSummarizeToolArgs(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		input map[string]any
		want  string
	}{
		{"Bash", "Bash", map[string]any{"command": "west build"}, `command="west build"`},
		{"Read", "Read", map[string]any{"file_path": "/root/main.c"}, `file_path="/root/main.c"`},
		{"Edit", "Edit", map[string]any{"file_path": "/root/main.c", "old_string": "x"}, `file_path="/root/main.c"`},
		{"Write", "Write", map[string]any{"file_path": "/root/new.c"}, `file_path="/root/new.c"`},
		{"Glob", "Glob", map[string]any{"pattern": "**/*.c"}, `pattern="**/*.c"`},
		{"Grep", "Grep", map[string]any{"pattern": "kernel"}, `pattern="kernel"`},
		{"Unknown with keys", "TodoWrite", map[string]any{"content": "x"}, "content"},
		{"Unknown empty", "TodoWrite", map[string]any{}, ""},
	}
	for _, tt := range tests {
		got := summarizeToolArgs(tt.tool, tt.input)
		if got != tt.want {
			t.Errorf("summarizeToolArgs(%q, ...): got %q, want %q", tt.tool, got, tt.want)
		}
	}
}

func TestSummarizeToolArgsBashLongCommand(t *testing.T) {
	longCmd := strings.Repeat("x", 300)
	got := summarizeToolArgs("Bash", map[string]any{"command": longCmd})

	// Should truncate to ~200 chars + "..."
	if !strings.Contains(got, "...") {
		t.Error("long bash command should be truncated")
	}
	if len(got) > 250 {
		t.Errorf("truncated command too long: %d chars", len(got))
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func makeTestEntry() corpus.Entry {
	return corpus.Entry{
		ID:          "sync-missing-kernel-header",
		Board:       "native_sim/native/64",
		Difficulty:  "easy",
		Description: "Missing #include <zephyr/kernel.h> in synchronization sample",
		Evaluation: corpus.Evaluation{
			Command:         "west build -b native_sim/native/64 samples/synchronization -p",
			SuccessExitCode: 0,
		},
	}
}

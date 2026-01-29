package corpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot walks up from the test's working directory until it finds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod found)")
		}
		dir = parent
	}
}

func corpusPath(t *testing.T) string {
	t.Helper()
	p := filepath.Join(repoRoot(t), "corpus", "zephyr-esp32.yaml")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("corpus file not found: %s", p)
	}
	return p
}

func TestLoadCorpus(t *testing.T) {
	c, err := Load(corpusPath(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if c.Name != "zephyr-esp32" {
		t.Errorf("Name = %q, want %q", c.Name, "zephyr-esp32")
	}
	if c.Image == "" {
		t.Error("Image is empty")
	}
	if len(c.Entries) != 8 {
		t.Errorf("len(Entries) = %d, want 8", len(c.Entries))
	}
}

func TestLoadCorpusEntryFields(t *testing.T) {
	c, err := Load(corpusPath(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	tests := []struct {
		id         string
		difficulty string
		board      string
	}{
		{"adc-missing-header", "easy", "esp32_devkitc_wroom"},
		{"pwm-variable-rename", "easy", "esp32_devkitc_wroom"},
		{"log-const-soc-undefined", "easy", "esp32_devkitc_wroom"},
		{"pinctrl-required-dts", "easy", "esp32_devkitc_wroom"},
		{"heap-sentry-linker", "medium", "esp32_devkitc_wroom"},
		{"hal-syslimits-break", "medium", "esp32_devkitc_wroom"},
		{"wifi-kconfig-clock-gate", "medium", "esp32_devkitc_wroom"},
		{"boot-code-wrong-section", "hard", "esp32_devkitc_wroom"},
	}

	for i, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if i >= len(c.Entries) {
				t.Fatalf("entry index %d out of range", i)
			}
			e := c.Entries[i]
			if e.ID != tt.id {
				t.Errorf("ID = %q, want %q", e.ID, tt.id)
			}
			if e.Difficulty != tt.difficulty {
				t.Errorf("Difficulty = %q, want %q", e.Difficulty, tt.difficulty)
			}
			if e.Board != tt.board {
				t.Errorf("Board = %q, want %q", e.Board, tt.board)
			}
			if e.BrokenSHA == "" {
				t.Error("BrokenSHA is empty")
			}
			if e.AppPath == "" {
				t.Error("AppPath is empty")
			}
			if e.Evaluation.Command == "" {
				t.Error("Evaluation.Command is empty")
			}
			if len(e.SetupCommands) == 0 {
				t.Error("SetupCommands is empty")
			}
		})
	}
}

func TestLoadCorpusVolumes(t *testing.T) {
	c, err := Load(corpusPath(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(c.Volumes) == 0 {
		t.Fatal("Volumes map is empty")
	}
	if path, ok := c.Volumes["zephyr-sdk"]; !ok {
		t.Error("expected zephyr-sdk volume")
	} else if path != "/usr/local/zephyr-sdk" {
		t.Errorf("zephyr-sdk path = %q, want /usr/local/zephyr-sdk", path)
	}
}

func TestValidateValid(t *testing.T) {
	c, err := Load(corpusPath(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateMissingName(t *testing.T) {
	c := &Corpus{
		Image:   "img",
		Entries: []Entry{{ID: "a", Board: "b", AppPath: "p", BrokenSHA: "abc", Difficulty: "easy", Evaluation: Evaluation{Command: "cmd"}}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for missing name")
	}
}

func TestValidateMissingImage(t *testing.T) {
	c := &Corpus{
		Name:    "test",
		Entries: []Entry{{ID: "a", Board: "b", AppPath: "p", BrokenSHA: "abc", Difficulty: "easy", Evaluation: Evaluation{Command: "cmd"}}},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for missing image")
	}
}

func TestValidateNoEntries(t *testing.T) {
	c := &Corpus{Name: "test", Image: "img"}
	if err := c.Validate(); err == nil {
		t.Error("expected error for empty entries")
	}
}

func TestValidateDuplicateID(t *testing.T) {
	c := &Corpus{
		Name:  "test",
		Image: "img",
		Entries: []Entry{
			{ID: "dup", Board: "b", AppPath: "p", BrokenSHA: "abc", Difficulty: "easy", Evaluation: Evaluation{Command: "cmd"}},
			{ID: "dup", Board: "b", AppPath: "p", BrokenSHA: "abc", Difficulty: "easy", Evaluation: Evaluation{Command: "cmd"}},
		},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for duplicate id")
	}
}

func TestValidateInvalidDifficulty(t *testing.T) {
	c := &Corpus{
		Name:  "test",
		Image: "img",
		Entries: []Entry{
			{ID: "a", Board: "b", AppPath: "p", BrokenSHA: "abc", Difficulty: "extreme", Evaluation: Evaluation{Command: "cmd"}},
		},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for invalid difficulty")
	}
}

func TestValidateMissingEntryFields(t *testing.T) {
	c := &Corpus{
		Name:  "test",
		Image: "img",
		Entries: []Entry{
			{ID: "a"},
		},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	s := err.Error()
	for _, want := range []string{"board", "app_path", "broken_sha", "evaluation.command", "difficulty"} {
		if !strings.Contains(s, want) {
			t.Errorf("error should mention %q, got: %s", want, s)
		}
	}
}

func TestFilterEntries(t *testing.T) {
	c, err := Load(corpusPath(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	filtered, err := c.FilterEntries([]string{"adc-missing-header", "boot-code-wrong-section"})
	if err != nil {
		t.Fatalf("FilterEntries: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	if filtered[0].ID != "adc-missing-header" {
		t.Errorf("filtered[0].ID = %q, want adc-missing-header", filtered[0].ID)
	}
	if filtered[1].ID != "boot-code-wrong-section" {
		t.Errorf("filtered[1].ID = %q, want boot-code-wrong-section", filtered[1].ID)
	}
}

func TestFilterEntriesAll(t *testing.T) {
	c, err := Load(corpusPath(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	ids := make([]string, len(c.Entries))
	for i, e := range c.Entries {
		ids[i] = e.ID
	}

	filtered, err := c.FilterEntries(ids)
	if err != nil {
		t.Fatalf("FilterEntries: %v", err)
	}
	if len(filtered) != len(c.Entries) {
		t.Errorf("len(filtered) = %d, want %d", len(filtered), len(c.Entries))
	}
}

func TestFilterEntriesMissing(t *testing.T) {
	c, err := Load(corpusPath(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	_, err = c.FilterEntries([]string{"adc-missing-header", "nonexistent"})
	if err == nil {
		t.Error("expected error for missing entry")
	}
}

func TestLoadNotFound(t *testing.T) {
	_, err := Load("/does/not/exist.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "bad.yaml")
	os.WriteFile(p, []byte("{{invalid"), 0644)

	_, err := Load(p)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

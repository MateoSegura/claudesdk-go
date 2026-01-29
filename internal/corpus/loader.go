// Package corpus parses and validates benchmark corpus YAML files.
package corpus

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Corpus defines a collection of benchmark entries sharing a container image
// and volume configuration.
type Corpus struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Image       string            `yaml:"image"`
	Volumes     map[string]string `yaml:"volumes"`
	Entries     []Entry           `yaml:"entries"`
}

// Entry is a single benchmark case: a broken commit with evaluation criteria.
type Entry struct {
	ID            string     `yaml:"id"`
	Issue         string     `yaml:"issue"`
	FixPR         string     `yaml:"fix_pr"`
	BrokenSHA     string     `yaml:"broken_sha"`
	Difficulty    string     `yaml:"difficulty"`
	Description   string     `yaml:"description"`
	Board         string     `yaml:"board"`
	AppPath       string     `yaml:"app_path"`
	SetupCommands []string   `yaml:"setup_commands"`
	Evaluation    Evaluation `yaml:"evaluation"`
}

// Evaluation defines how to judge whether Claude's fix was successful.
type Evaluation struct {
	Command         string `yaml:"command"`
	SuccessExitCode int    `yaml:"success_exit_code"`
}

// Load reads and parses a corpus YAML file from disk.
func Load(path string) (*Corpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading corpus file: %w", err)
	}

	var c Corpus
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing corpus YAML: %w", err)
	}

	return &c, nil
}

// Validate checks that the corpus has all required fields and that entries
// are well-formed.
func (c *Corpus) Validate() error {
	var errs []string

	if c.Name == "" {
		errs = append(errs, "corpus name is required")
	}
	if c.Image == "" {
		errs = append(errs, "corpus image is required")
	}
	if len(c.Entries) == 0 {
		errs = append(errs, "corpus must have at least one entry")
	}

	seen := make(map[string]bool)
	for i, e := range c.Entries {
		prefix := fmt.Sprintf("entry[%d]", i)

		if e.ID == "" {
			errs = append(errs, prefix+": id is required")
		} else if seen[e.ID] {
			errs = append(errs, prefix+": duplicate id "+e.ID)
		} else {
			seen[e.ID] = true
		}

		if e.Board == "" {
			errs = append(errs, prefix+" ("+e.ID+"): board is required")
		}
		if e.AppPath == "" {
			errs = append(errs, prefix+" ("+e.ID+"): app_path is required")
		}
		if e.BrokenSHA == "" {
			errs = append(errs, prefix+" ("+e.ID+"): broken_sha is required")
		}
		if e.Evaluation.Command == "" {
			errs = append(errs, prefix+" ("+e.ID+"): evaluation.command is required")
		}

		switch e.Difficulty {
		case "easy", "medium", "hard":
		case "":
			errs = append(errs, prefix+" ("+e.ID+"): difficulty is required")
		default:
			errs = append(errs, prefix+" ("+e.ID+"): invalid difficulty "+e.Difficulty)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("corpus validation failed:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// FilterEntries returns the subset of entries matching the given IDs.
// Returns an error if any requested ID is not found in the corpus.
func (c *Corpus) FilterEntries(ids []string) ([]Entry, error) {
	index := make(map[string]Entry)
	for _, e := range c.Entries {
		index[e.ID] = e
	}

	var result []Entry
	var missing []string
	for _, id := range ids {
		if e, ok := index[id]; ok {
			result = append(result, e)
		} else {
			missing = append(missing, id)
		}
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("entries not found: %s", strings.Join(missing, ", "))
	}
	return result, nil
}

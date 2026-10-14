package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/glw907/drafter/internal/atomicfile"
)

// StateSchema is the version of the state file; a resume under another
// version refuses.
const StateSchema = 1

const stateFile = "state.json"

// State is a run's resumable record, kept in the run directory.
type State struct {
	Schema         int                   `json:"schema"`
	DrafterVersion string                `json:"drafter_version"`
	RunID          string                `json:"run_id"`
	Repo           string                `json:"repo"`
	Project        string                `json:"project"`
	Branch         string                `json:"branch"`
	Head           string                `json:"head"`
	Stopped        string                `json:"stopped,omitempty"` // stop reason of a resumable run
	Pages          map[string]*PageState `json:"pages"`             // keyed by PageKey
}

// PageState is one page's progress in a run.
type PageState struct {
	Status string `json:"status"` // pending, running, held, accepted, or escalated
}

// LoadState reads the state file in dir. It refuses a state of another
// schema, naming both versions.
func LoadState(dir string) (*State, error) {
	path := filepath.Join(dir, stateFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state %s: %w", path, err)
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse state %s: %w", path, err)
	}
	if s.Schema != StateSchema {
		return nil, fmt.Errorf("state %s has schema %d, this drafter reads schema %d", path, s.Schema, StateSchema)
	}
	return &s, nil
}

// Save writes the state file in dir atomically.
func (s *State) Save(dir string) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	path := filepath.Join(dir, stateFile)
	if err := atomicfile.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}

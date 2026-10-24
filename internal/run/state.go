package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/glw907/drafter/internal/atomicfile"
)

// StateSchema is the version of the state file; a resume under another
// version refuses.
const StateSchema = 2

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
	// Inputs holds the run-directory copies of the files the run reads, keyed by role, with
	// the hash each had when copied.
	Inputs map[string]Input `json:"inputs,omitempty"`
}

// Input is one file copied into the run directory at start. Name is its path inside the run
// directory.
type Input struct {
	Name string `json:"name"`
	Hash string `json:"hash"`
	// Source names where the copy came from, for the prompts module. A state written before the
	// field existed has none.
	Source string `json:"source,omitempty"`
}

// PageState is one page's progress in a run.
type PageState struct {
	Status string `json:"status"` // pending, running, held, accepted, or escalated
	// Step names the pipeline step the page runs next. Its meaning belongs to the pipeline.
	Step string `json:"step,omitempty"`
	// Intents lists every submission unit the page made, in order. The last one is open while
	// its Settlement is empty.
	Intents []Intent `json:"intents,omitempty"`
	// Attempts counts the submissions for each stage and seat, keyed "stage.seat". Every
	// submission advances it, so a custom_id is never reused within a run.
	Attempts map[string]int `json:"attempts,omitempty"`
	// Work is the pipeline's working memory for the page, opaque to this package.
	Work json.RawMessage `json:"work,omitempty"`
	// Pre holds the hash of each file the page may write as it was before the run, keyed by
	// repo-relative path. An empty hash means the file did not exist.
	Pre map[string]string `json:"pre,omitempty"`
	// Writes lists the file writes the page started, in order.
	Writes []Write `json:"writes,omitempty"`
	// Restored is true once an ended page's record is complete and, for an escalated page,
	// its files are back to their pre-run state.
	Restored bool `json:"restored,omitempty"`
}

// Intent is one submission unit, one batch or one sync request, recorded before it is sent.
type Intent struct {
	ID       string          `json:"id"` // the ledger intent ID
	Stage    string          `json:"stage"`
	Attempt  int             `json:"attempt"` // the highest attempt among its requests
	Time     time.Time       `json:"time"`
	Mode     string          `json:"mode"`
	Worst    float64         `json:"worst"`
	Requests []IntentRequest `json:"requests"`
	BatchID  string          `json:"batch_id,omitempty"`
	// Saved is true once every response of the intent is on disk, in the page's record
	// directory, so a resume never needs the API for them again.
	Saved bool `json:"saved,omitempty"`
	// Handled is true once the page has acted on the responses.
	Handled bool `json:"handled,omitempty"`
	// Settlement is how the intent closed in the ledger: actual, estimated, or void. It is
	// empty while the intent is open.
	Settlement string `json:"settlement,omitempty"`
	// Created is the time the API reported for the batch, kept for adoption.
	Created time.Time `json:"created,omitzero"`
	// Poll is the last batch status a poll printed. A state written before the field existed has
	// none, and a resume prints the first status it sees.
	Poll *Poll `json:"poll,omitempty"`
}

// Poll is a batch's status and request counts as the last poll that printed saw them.
type Poll struct {
	Status     string    `json:"status"`
	Processing int       `json:"processing"`
	Succeeded  int       `json:"succeeded"`
	Errored    int       `json:"errored"`
	Canceled   int       `json:"canceled"`
	Expired    int       `json:"expired"`
	Seen       time.Time `json:"seen"`
}

// IntentRequest maps one custom_id to the seat it asks.
type IntentRequest struct {
	CustomID string `json:"custom_id"`
	Seat     string `json:"seat"`
}

// Write records one write of a page and its brief: the hashes the files had before and the
// hashes the saved answer produces. Post is empty until both files are written.
type Write struct {
	Pre  map[string]string `json:"pre"`
	Post map[string]string `json:"post,omitempty"`
	// Saved names the files in the page's record directory that hold the contents to write,
	// keyed by repo-relative path.
	Saved map[string]string `json:"saved"`
	// Restore marks the write that puts an escalated page's files back to their pre-run
	// state. Saved then names the pre-run snapshots, with no entry for a file the run created.
	Restore bool `json:"restore,omitempty"`
}

// Open returns the page's open intent, or nil.
func (p *PageState) Open() *Intent {
	if n := len(p.Intents); n > 0 && p.Intents[n-1].Settlement == "" {
		return &p.Intents[n-1]
	}
	return nil
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

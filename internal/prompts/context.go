package prompts

import (
	"encoding/json"
	"fmt"
)

// Round is the round a seat renders: Round1, Round2, or RoundFinal for the redraft and scoped
// reads after the final reader's read. It encodes as 1, 2, or "final".
type Round int

// The rounds a seat renders.
const (
	Round1 Round = iota + 1
	Round2
	RoundFinal
)

// MarshalJSON encodes r as 1, 2, or "final".
func (r Round) MarshalJSON() ([]byte, error) {
	switch r {
	case Round1:
		return []byte("1"), nil
	case Round2:
		return []byte("2"), nil
	case RoundFinal:
		return []byte(`"final"`), nil
	}
	return nil, fmt.Errorf("invalid round %d", int(r))
}

// UnmarshalJSON decodes 1, 2, or "final".
func (r *Round) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case "1":
		*r = Round1
	case "2":
		*r = Round2
	case `"final"`:
		*r = RoundFinal
	default:
		return fmt.Errorf("round must be 1, 2, or \"final\", got %s", b)
	}
	return nil
}

// Context is the one object every seat builder takes. Page, Inputs, and Narrow cross as raw
// JSON, and a nil Narrow encodes as null.
type Context struct {
	Page           json.RawMessage `json:"page"`
	Inputs         json.RawMessage `json:"inputs"`
	Round          Round           `json:"round"`
	Findings       string          `json:"findings"`
	AfterFinalRead bool            `json:"afterFinalRead"`
	Scope          string          `json:"scope"`
	Where          string          `json:"where"`
	Narrow         json.RawMessage `json:"narrow"`
	Turn           int             `json:"turn"`
	Facts          bool            `json:"facts"`
	// Markdown says the page's blueprint is a Markdown file, not YAML.
	Markdown bool `json:"markdown"`
	// Preferences is the text of the project's editing preferences file, verbatim, or empty.
	Preferences string `json:"preferences"`
}

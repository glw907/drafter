// Package smoke is the body of the live smoke: one batch holding one request per seat shape and
// one sync request, settled through the real ledger.
package smoke

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/ledger"
)

// Shape is one seat's request: its answer schema, effort, system prompt, and token limit.
type Shape struct {
	Seat      string
	System    string
	Prompt    string
	Effort    string
	MaxTokens int64
	Schema    json.RawMessage
}

// Params is what Run needs besides the shapes.
type Params struct {
	Options      batch.Options
	StateDir     string
	Model        string
	Prices       config.Prices
	Caps         ledger.Caps
	PollInterval time.Duration
	Sleep        func(ctx context.Context, d time.Duration) error
	Now          func() time.Time
}

// Report is what a smoke run saw.
type Report struct {
	RunID    string
	BatchID  string
	Results  []batch.Result
	Sync     batch.Result
	Counted  map[string]int64
	Resumed  []string
	Voided   []string
	Problems []string
}

type intentRecord struct {
	Intent    string
	BatchID   string
	Worst     float64
	Requests  int
	CustomIDs []string
	Settled   bool
}

type record struct {
	RunID string
	Model string
	Batch intentRecord
	Sync  intentRecord
}

// Shapes returns one synthetic shape per seat.
func Shapes() []Shape { return nil }

// Run runs the smoke.
func Run(ctx context.Context, p Params, shapes []Shape) (Report, error) {
	return Report{}, errors.New("not implemented")
}

// Check reports whether text parses to s's schema.
func Check(s Shape, text string) error { return errors.New("not implemented") }

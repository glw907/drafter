package pipeline

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/handoff"
)

// The reasons a run stops, resumable. A stopped run escalates no page.
const (
	StopCycleCap       = "cycle cap"
	StopSpendLimit     = "spend limit"
	StopTransport      = "transport"
	StopInfrastructure = "infrastructure"
	StopTreeRed        = "tree red outside drafter's pages"
)

// Page statuses in a Summary.
const (
	StatusAccepted  = "accepted"
	StatusEscalated = "escalated"
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusHeld      = "held"
)

// Options is what Run needs.
type Options struct {
	Config   *config.Config
	Handoffs []handoff.Handoff
	API      API
	// Resume is the ID of the stopped run to continue, or empty for a new run.
	Resume string
	// DryRun assembles the first stage's requests and counts their tokens, and submits nothing.
	DryRun bool
	// Version is the drafter version recorded in state.
	Version string
	// Now stamps run IDs and the cycle; time.Now when nil.
	Now func() time.Time
	// Sleep waits between batch polls; a timer when nil.
	Sleep func(ctx context.Context, d time.Duration) error
	// PollInterval is the wait between batch polls; thirty seconds when zero.
	PollInterval time.Duration
	// Out receives the dry run's report; Err receives status lines. Either may be nil.
	Out, Err io.Writer
}

// PageResult is one page's end state.
type PageResult struct {
	Path   string
	Status string
	// Rule is the stop rule an escalated page failed on.
	Rule string
	Cost float64
}

// Estimate is one request of a dry run.
type Estimate struct {
	Seat      string
	CustomID  string
	Tokens    int64
	MaxTokens int64
	Worst     float64
}

// Summary is what a run did.
type Summary struct {
	RunID string
	Pages []PageResult
	// Stop is the reason the run stopped, resumable, or empty when it finished.
	Stop string
	// ResumeLine is the command that continues a stopped run.
	ResumeLine string
	// Estimates is the dry run's first stage, one entry per request.
	Estimates []Estimate
	// EstimateStage names the stage the estimates are for.
	EstimateStage string
	// EstimateTotal is the stage's worst case in US dollars.
	EstimateTotal float64
}

// Run takes every handoff's page through the pipeline.
func Run(ctx context.Context, o Options) (Summary, error) {
	return Summary{}, errors.New("pipeline: not implemented")
}

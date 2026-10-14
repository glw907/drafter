// Package ledger keeps drafter's append-only spend ledger: reservations made before a submission
// and settlements made after it, checked against the page and cycle caps under a file lock.
package ledger

import (
	"errors"
	"io"
	"time"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/config"
)

var (
	ErrPageCap  = errors.New("page cap")
	ErrCycleCap = errors.New("cycle ceiling")
)

type Entry struct {
	RunID string `json:"run_id"`
	Repo  string `json:"repo"`
	Page  string `json:"page"`
	Seat  string `json:"seat"`
	Model string `json:"model"`
	Mode  string `json:"mode"`
}

type Reservation struct {
	Entry
	Intent   string
	Requests int
	Worst    float64
}

type Settlement struct {
	Entry
	Intent string
	Key    string
	Usage  batch.Usage
	Cost   float64
	Kind   string
}

type Caps struct {
	Page, Cycle float64
	CycleStart  time.Time
}

type Row struct {
	Repo, Seat string
	Cost       float64
	Requests   int
	Estimated  int
}

type OpenSummary struct {
	Count int
	Worst float64
}

type Report struct {
	Rows  []Row
	Total float64
	Open  OpenSummary
}

type Ledger struct {
	Now  func() time.Time
	Warn io.Writer

	hook func()
}

func Open(stateDir string) *Ledger { return &Ledger{Now: time.Now} }

func (l *Ledger) Reserve(r Reservation, c Caps) error { return nil }

func (l *Ledger) Settle(s Settlement) error { return nil }

func (l *Ledger) OpenIntents(runID string) ([]Reservation, error) { return nil, nil }

func (l *Ledger) Spend(since, until time.Time) (Report, error) { return Report{}, nil }

func (l *Ledger) PageSpend(runID, page string) (float64, error) { return 0, nil }

func Cost(p config.Prices, model string, u batch.Usage, batchMode bool) (float64, error) {
	return 0, nil
}

func Worst(p config.Prices, model string, inputTokens, maxTokens int64, batchMode bool) (float64, error) {
	return 0, nil
}

func CycleStart(now time.Time, day int) time.Time { return time.Time{} }

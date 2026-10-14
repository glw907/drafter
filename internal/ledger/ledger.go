// Package ledger keeps drafter's append-only spend ledger: reservations made before a submission
// and settlements made after it, checked against the page and cycle caps under a file lock.
package ledger

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/glw907/drafter/internal/batch"
)

// ErrPageCap and ErrCycleCap are returned, wrapped with the figures, by a Reserve that would
// cross the page cap or the cycle ceiling.
var (
	ErrPageCap  = errors.New("page cap reached")
	ErrCycleCap = errors.New("cycle ceiling reached")
)

// Entry names what a line is for.
type Entry struct {
	RunID string `json:"run_id"`
	Repo  string `json:"repo"`
	Page  string `json:"page"`
	Seat  string `json:"seat"`
	Model string `json:"model"`
	Mode  string `json:"mode"`
}

// Reservation is the worst case of one submission unit: one batch, or one sync request. Every
// resubmission is a new intent with a reservation of its own.
type Reservation struct {
	Entry
	Intent   string
	Requests int
	Worst    float64 // US dollars
}

// Settlement records what one request of an intent cost. Kind is "actual", "estimated" (a sync
// request whose response was lost, at Worst / Requests), or "void" (an intent the API never
// billed, at zero, closing the whole intent). Key is unique per request: a settlement whose key
// is already in the ledger is a no-op.
type Settlement struct {
	Entry
	Intent string
	Key    string
	Usage  batch.Usage
	Cost   float64 // US dollars
	Kind   string
}

// Caps are the limits a reservation is checked against, in US dollars. CycleStart opens the
// billing cycle the ceiling covers.
type Caps struct {
	Page       float64
	Cycle      float64
	CycleStart time.Time
}

// Row is the spend of one repo and seat. Estimated counts the lines settled at an estimate.
type Row struct {
	Repo      string
	Seat      string
	Cost      float64
	Requests  int
	Estimated int
}

// OpenSummary counts the intents reserved and not yet closed, and their worst cases.
type OpenSummary struct {
	Count int
	Worst float64
}

// Report is what Spend returns: settled cost by repo and seat, and the open reservations, which
// are never part of the cost.
type Report struct {
	Rows  []Row
	Total float64
	Open  OpenSummary
}

// Ledger is the spend ledger in one state directory. Processes sharing the directory serialize
// their checks and appends on ledger.lock. A read takes the lock shared, so it waits for a writer
// mid-append and never sees a half-written line; readers do not wait for each other.
type Ledger struct {
	Now  func() time.Time // stamps each line; time.Now from Open
	Warn io.Writer        // receives torn-line warnings; os.Stderr when nil

	dir  string
	hook func() // called between a Reserve's check and its append; nil in production
}

// Open returns the ledger kept in stateDir. It touches nothing until the first read or append. A
// read creates ledger.lock in an existing state directory and never the directory; the first
// append creates both. Nothing may remove or replace ledger.lock while a holder runs, since
// mutual exclusion depends on the file keeping its inode.
func Open(stateDir string) *Ledger {
	return &Ledger{Now: time.Now, dir: stateDir}
}

// Reserve appends r's reserve line unless it would cross a cap. A total plus r.Worst equal to a
// cap passes. The lock is held for the check and the append only, never across a network call.
func (l *Ledger) Reserve(r Reservation, c Caps) error {
	if r.Intent == "" || r.Requests < 1 {
		return errors.New("reservation needs an intent and at least one request")
	}
	unlock, err := l.lock()
	if err != nil {
		return err
	}
	defer unlock()
	st, err := l.read()
	if err != nil {
		return err
	}
	if it, ok := st.intents[r.Intent]; ok && it.reserved {
		return fmt.Errorf("intent %q is already reserved", r.Intent)
	}
	if page := st.pageTotal(r.RunID, r.Page); page+r.Worst > c.Page {
		return fmt.Errorf("%w: page %s holds %.4f, this reservation adds %.4f, cap %.4f",
			ErrPageCap, r.Page, page, r.Worst, c.Page)
	}
	if cycle := st.cycleTotal(c.CycleStart); cycle+r.Worst > c.Cycle {
		return fmt.Errorf("%w: cycle holds %.4f, this reservation adds %.4f, ceiling %.4f",
			ErrCycleCap, cycle, r.Worst, c.Cycle)
	}
	if l.hook != nil {
		l.hook()
	}
	return l.append(st, line{
		V: version, Type: typeReserve, Time: l.stamp(), Entry: r.Entry,
		Intent: r.Intent, Requests: r.Requests, Worst: r.Worst,
	})
}

// Settle appends s's settle line. A key already settled is a no-op; an empty key is an error.
func (l *Ledger) Settle(s Settlement) error {
	if s.Key == "" {
		return errors.New("settlement has no key")
	}
	if s.Intent == "" {
		return errors.New("settlement has no intent")
	}
	if !slices.Contains([]string{"actual", "estimated", "void"}, s.Kind) {
		return fmt.Errorf("settlement kind %q is not actual, estimated, or void", s.Kind)
	}
	unlock, err := l.lock()
	if err != nil {
		return err
	}
	defer unlock()
	st, err := l.read()
	if err != nil {
		return err
	}
	if st.keys[s.Key] {
		return nil
	}
	usage := s.Usage
	return l.append(st, line{
		V: version, Type: typeSettle, Time: l.stamp(), Entry: s.Entry,
		Intent: s.Intent, Key: s.Key, Usage: &usage, Cost: s.Cost, Kind: s.Kind,
	})
}

// OpenIntents returns the run's reservations not yet closed, in the order they were made.
func (l *Ledger) OpenIntents(runID string) ([]Reservation, error) {
	st, err := l.readShared()
	if err != nil {
		return nil, err
	}
	var open []Reservation
	for _, id := range st.order {
		if it := st.intents[id]; it.open() && it.res.RunID == runID {
			open = append(open, it.res)
		}
	}
	return open, nil
}

// Spend reports the settled lines stamped at or after since and before until; a zero time leaves
// that end open. Open reservations are summarized whatever their age.
func (l *Ledger) Spend(since, until time.Time) (Report, error) {
	st, err := l.readShared()
	if err != nil {
		return Report{}, err
	}
	var rep Report
	rows := map[[2]string]*Row{}
	for _, s := range st.settles {
		if s.Kind == "void" ||
			(!since.IsZero() && s.Time.Before(since)) ||
			(!until.IsZero() && !s.Time.Before(until)) {
			continue
		}
		k := [2]string{s.Repo, s.Seat}
		row, ok := rows[k]
		if !ok {
			row = &Row{Repo: s.Repo, Seat: s.Seat}
			rows[k] = row
		}
		row.Cost += s.Cost
		row.Requests++
		if s.Kind == "estimated" {
			row.Estimated++
		}
		rep.Total += s.Cost
	}
	for _, row := range rows {
		rep.Rows = append(rep.Rows, *row)
	}
	slices.SortFunc(rep.Rows, func(a, b Row) int {
		return strings.Compare(a.Repo+"\x00"+a.Seat, b.Repo+"\x00"+b.Seat)
	})
	for _, id := range st.order {
		if it := st.intents[id]; it.open() {
			rep.Open.Count++
			rep.Open.Worst += it.res.Worst
		}
	}
	return rep, nil
}

// PageSpend is the page's settled spend plus its open intents at their worst case.
func (l *Ledger) PageSpend(runID, page string) (float64, error) {
	st, err := l.readShared()
	if err != nil {
		return 0, err
	}
	return st.pageTotal(runID, page), nil
}

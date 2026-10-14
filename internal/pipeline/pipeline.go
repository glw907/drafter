package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/handoff"
	"github.com/glw907/drafter/internal/ledger"
	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/internal/prompts"
	"github.com/glw907/drafter/internal/run"
)

// The reasons a run stops, resumable. A stopped run escalates no page.
const (
	StopCycleCap       = "cycle cap"
	StopSpendLimit     = "spend limit"
	StopTransport      = "transport"
	StopInfrastructure = "infrastructure"
	StopTreeRed        = "tree red outside drafter's pages"
)

// Page statuses in a Summary and in state.
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

// runner holds one run.
type runner struct {
	o     Options
	cfg   *config.Config // the configuration with seat agents pointing at the run directory's copies
	api   API
	led   *ledger.Ledger
	now   func() time.Time
	root  string
	dir   string
	state *run.State
	// suffix is the random part of the run ID, which starts every custom_id.
	suffix string
	facts  *plan.Facts
	pages  []*page
	mods   map[string]*prompts.Module
	stop   string
	locks  []*run.Lock
	// inflight holds the batches submitted and not yet read, in submission order.
	inflight []*flight
}

// Run takes every handoff's page through the pipeline.
func Run(ctx context.Context, o Options) (Summary, error) {
	r, err := newRunner(ctx, o)
	if err != nil {
		return Summary{}, err
	}
	defer r.release()
	if o.DryRun {
		return r.dryRun(ctx)
	}
	if err := r.loop(ctx); err != nil {
		return r.summary(), err
	}
	if err := r.finish(); err != nil {
		return r.summary(), err
	}
	return r.summary(), nil
}

func (r *runner) release() {
	for _, l := range r.locks {
		_ = l.Release()
	}
}

func (r *runner) warnf(format string, a ...any) {
	if r.o.Err != nil {
		_, _ = fmt.Fprintf(r.o.Err, format+"\n", a...)
	}
}

// summary reports every page's state.
func (r *runner) summary() Summary {
	s := Summary{RunID: r.state.RunID, Stop: r.stop}
	for _, p := range r.pages {
		res := PageResult{Path: p.h.Page.Path, Status: p.ps.Status}
		if p.w.Rec.Escalation != nil {
			res.Rule = p.w.Rec.Escalation.Rule
		}
		res.Cost, _ = r.led.PageSpend(r.state.RunID, p.h.Page.Path)
		s.Pages = append(s.Pages, res)
	}
	if r.stop != "" {
		s.ResumeLine = "drafter page --resume " + r.state.RunID
	}
	return s
}

// halt stops the run, resumable. The first reason wins.
func (r *runner) halt(reason string) {
	if r.stop == "" {
		r.stop = reason
	}
}

// save writes the run's state, after folding each page's working memory into it.
func (r *runner) save() error {
	for _, p := range r.pages {
		raw, err := p.w.marshal()
		if err != nil {
			return fmt.Errorf("encode working state of %s: %w", p.h.Page.Path, err)
		}
		p.ps.Work = raw
	}
	r.state.Stopped = r.stop
	return r.state.Save(r.dir)
}

// slots reports how many more pages may start.
func (r *runner) slots() int {
	n := r.cfg.PagesInFlight
	for _, p := range r.pages {
		if p.ps.Status == StatusRunning || p.ps.Status == StatusHeld {
			n--
		}
	}
	return n
}

// activate starts pending pages while slots remain.
func (r *runner) activate() {
	for _, p := range r.pages {
		if r.slots() <= 0 {
			return
		}
		if p.ps.Status == StatusPending {
			p.ps.Status = StatusRunning
		}
	}
}

// loop runs waves until every page has ended or the run stops.
func (r *runner) loop(ctx context.Context) error {
	for r.stop == "" {
		r.activate()
		var staged []*stage
		for _, p := range r.pages {
			if p.ps.Status != StatusRunning || p.ps.Open() != nil {
				continue
			}
			stg, err := p.advance(ctx)
			if err != nil {
				return err
			}
			if stg != nil {
				staged = append(staged, stg)
			}
			if r.stop != "" {
				break
			}
		}
		if err := r.save(); err != nil {
			return err
		}
		if r.stop != "" {
			break
		}
		for _, stg := range staged {
			if p := stg.page; p.ps.Status != StatusRunning {
				continue
			}
			if err := r.submit(ctx, stg); err != nil {
				return err
			}
			if r.stop != "" {
				break
			}
		}
		if r.stop != "" {
			break
		}
		if len(r.inflight) > 0 {
			if err := r.await(ctx); err != nil {
				return err
			}
			if err := r.save(); err != nil {
				return err
			}
			continue
		}
		if len(staged) > 0 {
			continue
		}
		more, err := r.releaseHeld(ctx)
		if err != nil {
			return err
		}
		if !more {
			break
		}
	}
	return r.save()
}

// finish writes the end state of a run that did not stop.
func (r *runner) finish() error {
	if r.stop != "" {
		r.warnf("stopped: %s; resume with: drafter page --resume %s", r.stop, r.state.RunID)
		return r.save()
	}
	return r.save()
}

// releaseHeld gates every held page again once nothing else is running. It reports whether
// another wave can make progress. A held page whose gate is still red with nothing of its own,
// with no other page left to change the tree, stops the run.
func (r *runner) releaseHeld(ctx context.Context) (bool, error) {
	var held []*page
	for _, p := range r.pages {
		switch p.ps.Status {
		case StatusRunning:
			return true, nil
		case StatusHeld:
			held = append(held, p)
		case StatusPending:
			return r.slots() > 0, nil
		}
	}
	if len(held) == 0 {
		return false, nil
	}
	progressed := false
	for _, p := range held {
		released, err := p.regate(ctx)
		if err != nil {
			return false, err
		}
		progressed = progressed || released
		if r.stop != "" {
			return false, nil
		}
	}
	if !progressed {
		r.halt(StopTreeRed)
		return false, nil
	}
	return true, nil
}

// ErrDirty is the error for a worktree with uncommitted changes.
var ErrDirty = errors.New("the worktree has uncommitted changes")

func contextKey(h handoff.Handoff) string { return string(h.Context) }

// moduleFor returns the prompts module for the handoff's context.
func (r *runner) moduleFor(h handoff.Handoff, src []byte, name string) (*prompts.Module, error) {
	k := contextKey(h)
	if m, ok := r.mods[k]; ok {
		return m, nil
	}
	host, err := hostFor(h, r.o.Handoffs)
	if err != nil {
		return nil, err
	}
	m, err := prompts.Load(name, src, host)
	if err != nil {
		return nil, err
	}
	r.mods[k] = m
	return m, nil
}

func relTo(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil {
		return rel
	}
	return p
}

func sortedSeats(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func lines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

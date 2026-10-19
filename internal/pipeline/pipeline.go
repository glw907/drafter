package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/glw907/drafter/internal/blueprint"
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
	// Since is the revision a blueprint read compared against, resolved to a commit, and Scopes
	// holds each page's comparison keyed by page path. Both are recorded in the pages' state, so
	// a resume needs neither.
	Since  string
	Scopes map[string]blueprint.Scope
	// Changes holds each scoped page's change block, keyed by page path: what the page's read
	// judges beside the blueprint. It is recorded in the pages' state, so a resume sends the same
	// bytes.
	Changes map[string]string
	// Command names the command that started the run, for the resume hint; "drafter page" when
	// empty.
	Command string
	// ResumeArgs are the arguments the resume hint repeats after the run ID: the blueprints or
	// handoff files as given, or --workflow-run and its path.
	ResumeArgs []string
}

// command is the command a stopped run is resumed with.
func (r *runner) command() string {
	if r.o.Command == "" {
		return "drafter page"
	}
	return r.o.Command
}

// PageResult is one page's end state.
type PageResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	// Rule is the stop rule an escalated page failed on.
	Rule string  `json:"rule,omitempty"`
	Cost float64 `json:"cost"`
	// Scope is what the page's read was limited to when the run compared against an earlier
	// revision, and nil for a read of the whole blueprint with no comparison.
	Scope *blueprint.Scope `json:"scope,omitempty"`
	// Accepted is the git blob id of the blueprint a blueprint page's read accepted, set only when
	// the page ended accepted.
	Accepted string `json:"accepted,omitempty"`
	// Demoted counts the consistency conflicts a change read reported as a warning, already present
	// or unplaced, and AlreadyPresent the non-blocking findings of the blueprint read that say so.
	Demoted        int `json:"demoted,omitempty"`
	AlreadyPresent int `json:"already_present,omitempty"`
	// Findings are the blueprint read's findings, blocking or not, in the order the read gave them.
	Findings []FindingResult `json:"findings,omitempty"`
	// Warnings are the record's warnings: the blueprint check's warnings and the conflicts a
	// change read set aside.
	Warnings []string `json:"warnings,omitempty"`
	// Detail is what an escalation says beyond its rule, such as the conflicts that blocked.
	Detail string `json:"detail,omitempty"`
}

// FindingResult is one finding of a page's blueprint read.
type FindingResult struct {
	Location string `json:"location"`
	Text     string `json:"text"`
	Blocking bool   `json:"blocking"`
}

// Estimate is one request of a dry run.
type Estimate struct {
	Seat      string  `json:"seat"`
	CustomID  string  `json:"custom_id"`
	Tokens    int64   `json:"tokens"`
	MaxTokens int64   `json:"max_tokens"`
	Worst     float64 `json:"worst"`
}

// Summary is what a run did.
type Summary struct {
	RunID string       `json:"run_id,omitempty"`
	Pages []PageResult `json:"pages"`
	// Stop is the reason the run stopped, resumable, or empty when it finished.
	Stop string `json:"stop,omitempty"`
	// ResumeLine is the command that continues a stopped run.
	ResumeLine string `json:"resume,omitempty"`
	// Estimates is the dry run's first stage, one entry per request.
	Estimates []Estimate `json:"estimates,omitempty"`
	// EstimateStage names the stage the estimates are for.
	EstimateStage string `json:"estimate_stage,omitempty"`
	// EstimateTotal is the stage's worst case in US dollars.
	EstimateTotal float64 `json:"estimate_total,omitempty"`
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
		res := PageResult{Path: p.h.Page.Path, Status: p.ps.Status, Scope: p.w.Scope}
		if p.w.Rec.Escalation != nil {
			res.Rule, res.Detail = p.w.Rec.Escalation.Rule, p.w.Rec.Escalation.Detail
		}
		res.Findings, res.Warnings = p.w.Rec.blueprintFindings(), p.w.Rec.Warnings
		res.Demoted, res.AlreadyPresent = p.w.Demoted, p.w.Rec.alreadyPresent()
		res.Cost, _ = r.led.PageSpend(r.state.RunID, p.h.Page.Path)
		if p.ps.Status == StatusAccepted {
			res.Accepted = p.w.Accepted
		}
		s.Pages = append(s.Pages, res)
	}
	if r.stop != "" {
		s.ResumeLine = r.resumeCommand()
	}
	return s
}

// resumeCommand is the command that continues the stopped run.
func (r *runner) resumeCommand() string {
	return ResumeCommand(r.command(), r.cfg.Mode == "sync", r.state.RunID, r.o.ResumeArgs)
}

// ResumeCommand is the command line that continues a stopped run: the sync flag when the run's mode
// is sync, since a resume under another mode fails the configuration check, then args as the
// command was given them.
func ResumeCommand(command string, sync bool, runID string, args []string) string {
	var b strings.Builder
	b.WriteString(command)
	if sync {
		b.WriteString(" --sync")
	}
	b.WriteString(" --resume " + runID)
	for _, a := range args {
		b.WriteString(" " + shellWord(a))
	}
	return b.String()
}

// shellWord quotes a for a POSIX shell when it holds a character the shell would split or expand.
func shellWord(a string) string {
	safe := func(c rune) bool {
		return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("_./:@%+=,-", c)
	}
	if a != "" && !strings.ContainsFunc(a, func(c rune) bool { return !safe(c) }) {
		return a
	}
	return "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
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
		r.warnf("stopped: %s; resume with: %s", r.stop, r.resumeCommand())
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

func lines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/glw907/drafter/internal/atomicfile"
	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/gate"
	"github.com/glw907/drafter/internal/ledger"
	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/internal/run"
)

// resultsExpire is how long after a batch's creation its results stay downloadable.
const resultsExpire = 29 * 24 * time.Hour

// resume continues a stopped run. The caps, the key, and workspace_id are read fresh; the prompts
// module, the agent definitions, and the plan and framing are the run's own, checked against the
// hashes recorded at start.
func (r *runner) resume(ctx context.Context) (*runner, error) {
	cfg := r.o.Config
	if r.o.DryRun {
		return nil, errors.New("--dry-run and --resume cannot be combined")
	}
	repo, err := run.Repo(r.root)
	if err != nil {
		return nil, err
	}
	r.dir = filepath.Join(cfg.StateDir, "runs", repo, r.o.Resume)
	if r.state, err = run.LoadState(r.dir); err != nil {
		return nil, fmt.Errorf("resume %s: %w", r.o.Resume, err)
	}
	r.suffix = r.o.Resume[strings.IndexByte(r.o.Resume, '-')+1:]
	if err := r.takeWorktreeLock(); err != nil {
		return nil, err
	}
	l, err := run.TryLock(filepath.Join(r.dir, "run.lock"))
	if err != nil {
		return nil, err
	}
	r.locks = append(r.locks, l)

	if err := r.checkPosition(); err != nil {
		return nil, err
	}
	if err := r.o.API.CheckWorkspace(ctx); err != nil {
		return nil, fmt.Errorf("check workspace: %w", err)
	}
	if err := r.useCopies(); err != nil {
		return nil, err
	}
	if err := r.rehydrate(); err != nil {
		return nil, err
	}
	if cfg.Facts.Dir != "" {
		if r.facts, err = plan.LoadFacts(cfg.Facts.Dir); err != nil {
			return nil, err
		}
	}
	if err := r.loadModules(); err != nil {
		return nil, err
	}
	if err := r.checkTree(); err != nil {
		return nil, err
	}
	r.stop = ""
	if err := r.settleOpen(ctx); err != nil {
		return nil, err
	}
	if err := r.save(); err != nil {
		return nil, err
	}
	return r, nil
}

// checkPosition requires the recorded branch, and a HEAD that is the recorded commit or a
// descendant of it.
func (r *runner) checkPosition() error {
	branch, err := r.gitOut("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	if got := strings.TrimSpace(branch); got != r.state.Branch {
		return fmt.Errorf("the run was on branch %s and the worktree is on %s", r.state.Branch, got)
	}
	cmd := exec.Command("git", "-C", r.root, "merge-base", "--is-ancestor", r.state.Head, "HEAD")
	cmd.Env = gate.Env(r.o.Config)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("HEAD is not the commit the run started at (%s) or a descendant of it", r.state.Head)
	}
	return nil
}

// useCopies builds the run's configuration from the copies in the run directory, which win over
// the files they were copied from, and checks each copy still has the hash it was saved with.
func (r *runner) useCopies() error {
	cfg := *r.o.Config
	cfg.Seat = maps.Clone(r.o.Config.Seat)
	r.cfg = &cfg
	path := func(role string) (string, error) {
		in, ok := r.state.Inputs[role]
		if !ok {
			return "", fmt.Errorf("the run's state has no copy of %s", role)
		}
		p := filepath.Join(r.dir, in.Name)
		got, err := hashFile(p)
		if err != nil {
			return "", fmt.Errorf("read the run's copy of %s: %w", role, err)
		}
		if got != in.Hash {
			return "", fmt.Errorf("the run's copy of %s changed: %s", role, p)
		}
		return p, nil
	}
	var err error
	if r.cfg.Prompts, err = path("prompts"); err != nil {
		return err
	}
	for name, s := range r.cfg.Seat {
		if _, ok := r.state.Inputs["agent."+name]; !ok {
			continue
		}
		if s.Agent, err = path("agent." + name); err != nil {
			return err
		}
		r.cfg.Seat[name] = s
	}
	return nil
}

// rehydrate rebuilds each page from the handoffs and the state. The handoffs must be the run's
// pages, each with the plan and framing it was checked against.
func (r *runner) rehydrate() error {
	seen := map[string]bool{}
	for _, h := range r.o.Handoffs {
		if err := h.Verify(); err != nil {
			return err
		}
		key := run.PageKey(h.Page.Path)
		ps, ok := r.state.Pages[key]
		if !ok {
			return fmt.Errorf("page %s is not in run %s", h.Page.Path, r.state.RunID)
		}
		seen[key] = true
		p, err := r.newPage(h)
		if err != nil {
			return err
		}
		p.ps = ps
		p.w = work{Rec: newRecord()}
		if len(ps.Work) > 0 {
			if err := json.Unmarshal(ps.Work, &p.w); err != nil {
				return fmt.Errorf("read the working state of %s: %w", h.Page.Path, err)
			}
		}
		r.pages = append(r.pages, p)
	}
	for key := range r.state.Pages {
		if !seen[key] {
			return fmt.Errorf("run %s has page %s, which no handoff names", r.state.RunID, key)
		}
	}
	return nil
}

// expected lists the hashes a page file may have at resume: what it was before the run, before
// each write, and after each write.
func (p *page) expected(rel string) []string {
	out := []string{p.ps.Pre[rel]}
	for _, w := range p.ps.Writes {
		out = append(out, w.Pre[rel], w.Post[rel])
		if w.Post == nil {
			// The write was interrupted: the saved answer is what the file becomes.
			if data, err := os.ReadFile(filepath.Join(p.dir, w.Saved[rel])); err == nil {
				out = append(out, hashBytes(data))
			}
		}
	}
	return out
}

// checkTree requires that the only changes in the worktree are drafter's own files at hashes the
// state accounts for. Any other change stops the resume naming the file.
func (r *runner) checkTree() error {
	out, err := r.gitOut("status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return err
	}
	owned := map[string]*page{}
	index := map[string]bool{}
	for _, p := range r.pages {
		for _, rel := range p.files() {
			owned[rel] = p
		}
		for rel := range p.w.IndexPre {
			index[rel] = true
		}
	}
	for _, ln := range lines(out) {
		if len(ln) < 4 {
			continue
		}
		rel := strings.Trim(ln[3:], `"`)
		if index[rel] {
			continue
		}
		p, ok := owned[rel]
		if !ok {
			return fmt.Errorf("%s changed and is not a file of this run; resolve it before resuming", rel)
		}
		got, err := p.hashOf(rel)
		if err != nil {
			return err
		}
		if !slices.Contains(p.expected(rel), got) {
			return fmt.Errorf("%s changed since the run last wrote it; resolve it before resuming", rel)
		}
	}
	return nil
}

// settleOpen brings the ledger and the pages up to date with what the stopped run left: it voids
// reservations the state never named, finishes writes and restores, and settles or adopts each
// open intent.
func (r *runner) settleOpen(ctx context.Context) error {
	known := map[string]bool{}
	for _, p := range r.pages {
		for _, in := range p.ps.Intents {
			known[in.ID] = true
		}
	}
	open, err := r.led.OpenIntents(r.state.RunID)
	if err != nil {
		return err
	}
	for _, res := range open {
		if known[res.Intent] {
			continue
		}
		err := r.led.Settle(ledger.Settlement{Entry: res.Entry, Intent: res.Intent, Key: res.Intent + "/void", Kind: "void"})
		if err != nil {
			return fmt.Errorf("void %s: %w", res.Intent, err)
		}
	}
	for _, p := range r.pages {
		if err := r.recoverPage(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) recoverPage(ctx context.Context, p *page) error {
	switch p.ps.Status {
	case StatusAccepted, StatusEscalated:
		if !p.ps.Restored {
			return p.finalize(p.ps.Status)
		}
		return nil
	}
	if err := p.finishWrite(); err != nil {
		return err
	}
	n := len(p.ps.Intents)
	if p.w.PendingGate != "" && p.ps.Open() == nil {
		// The write that opened this gate came from the last intent's answer.
		if err := p.gateStep(ctx, p.w.PendingGate); err != nil {
			return err
		}
		if n > 0 {
			p.ps.Intents[n-1].Handled = true
		}
		return nil
	}
	if n == 0 {
		return nil
	}
	return r.recoverIntent(ctx, p, n-1)
}

// finishWrite completes a write the run was in the middle of: files at the post-write hashes
// stand, and files at the pre-write hashes are written again from the saved answer, at no cost.
func (p *page) finishWrite() error {
	n := len(p.ps.Writes)
	if n == 0 || p.ps.Writes[n-1].Post != nil || p.ps.Writes[n-1].Restore {
		// A restore interrupted by a stop is finished by finalize, which is safe to run again.
		return nil
	}
	w := &p.ps.Writes[n-1]
	post := map[string]string{}
	for _, rel := range p.files() {
		data, err := os.ReadFile(filepath.Join(p.dir, w.Saved[rel]))
		if err != nil {
			return fmt.Errorf("read the saved answer for %s: %w", rel, err)
		}
		want := hashBytes(data)
		got, err := p.hashOf(rel)
		if err != nil {
			return err
		}
		switch got {
		case want:
		case w.Pre[rel]:
			if err := os.MkdirAll(filepath.Dir(p.abs(rel)), 0o755); err != nil {
				return err
			}
			if err := atomicfile.WriteFile(p.abs(rel), data, 0o644); err != nil {
				return fmt.Errorf("write %s from the saved answer: %w", rel, err)
			}
		default:
			return fmt.Errorf("%s is neither as it was before the write nor as the write left it; resolve it before resuming", rel)
		}
		post[rel] = want
	}
	w.Post = post
	return p.r.save()
}

// loadResponses reads the saved responses of an intent from the page's record directory.
func (p *page) loadResponses(in *run.Intent) ([]batch.Result, error) {
	var out []batch.Result
	for _, rq := range in.Requests {
		data, err := os.ReadFile(filepath.Join(p.dir, "responses", rq.CustomID+".json"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var res batch.Result
		if err := json.Unmarshal(data, &res); err != nil {
			return nil, fmt.Errorf("read saved response %s: %w", rq.CustomID, err)
		}
		out = append(out, res)
	}
	return out, nil
}

// recoverIntent settles or adopts the page's last intent.
func (r *runner) recoverIntent(ctx context.Context, p *page, idx int) error {
	in := &p.ps.Intents[idx]
	switch {
	case in.Handled || in.Settlement == "void":
		return nil
	case in.Settlement != "" && !in.Saved:
		// Settled at an estimate with no response to read: the step runs again as a new
		// attempt.
		return nil
	case in.Saved:
		results, err := p.loadResponses(in)
		if err != nil {
			return err
		}
		if in.Settlement == "" {
			if err := r.settleResults(p, idx, results); err != nil {
				return err
			}
		}
		return r.process(ctx, p, idx, results)
	case in.Mode == "sync":
		return r.settleLost(p, idx)
	}
	return r.reattach(ctx, p, idx)
}

// settleLost settles a sync intent with no saved response at its reservation, marked estimated,
// unless the ledger already closed it. The request is sent again as a new attempt.
func (r *runner) settleLost(p *page, idx int) error {
	in := &p.ps.Intents[idx]
	open, err := r.led.OpenIntents(r.state.RunID)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(open, func(res ledger.Reservation) bool { return res.Intent == in.ID }) {
		return r.settleEstimated(p, idx)
	}
	in.Settlement = "estimated"
	return r.save()
}

// reattach adopts a batch the run created, finding it by its custom_ids when the state never
// recorded its ID, and queues it to be read. A batch whose results have expired settles at its
// reservation.
func (r *runner) reattach(ctx context.Context, p *page, idx int) error {
	in := &p.ps.Intents[idx]
	if in.BatchID == "" {
		found, ok, err := r.findBatch(ctx, in)
		if err != nil {
			return err
		}
		if !ok {
			return r.voidIntent(p, idx)
		}
		in.BatchID, in.Created = found.ID, found.CreatedAt
		if err := r.save(); err != nil {
			return err
		}
	}
	if !in.Created.IsZero() && r.now().Sub(in.Created) > resultsExpire {
		if err := r.record(p, idx, nil); err != nil {
			return err
		}
		return r.process(ctx, p, idx, nil)
	}
	r.inflight = append(r.inflight, &flight{page: p, idx: idx})
	return nil
}

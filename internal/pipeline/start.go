package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/glw907/drafter/internal/atomicfile"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/gate"
	"github.com/glw907/drafter/internal/handoff"
	"github.com/glw907/drafter/internal/ledger"
	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/internal/prompts"
	"github.com/glw907/drafter/internal/run"
)

// dryID stands in for the run ID's random suffix in a dry run, which writes nothing.
const dryID = "dryrun00"

// newRunner validates the options and prepares a run: a new one, or the resume of a stopped one.
// A dry run prepares everything in memory and creates nothing on disk.
func newRunner(ctx context.Context, o Options) (*runner, error) {
	cfg := o.Config
	if cfg == nil || o.API == nil {
		return nil, errors.New("pipeline: Options needs a Config and an API")
	}
	if err := cfg.Validate(config.ScopeRun); err != nil {
		return nil, err
	}
	if cfg.Facts.Dir != "" {
		if err := plan.CheckFactsDir(cfg.Facts.Dir); err != nil {
			return nil, fmt.Errorf("facts.dir: %w", err)
		}
	}
	if len(o.Handoffs) == 0 {
		return nil, errors.New("no handoffs to run")
	}
	root := o.Handoffs[0].Project
	for _, h := range o.Handoffs {
		if h.Project != root {
			return nil, fmt.Errorf("handoffs name more than one project: %s and %s", root, h.Project)
		}
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	r := &runner{o: o, api: o.API, now: now, root: root, mods: map[string]*prompts.Module{}}
	r.led = ledger.Open(cfg.StateDir)
	r.led.Now = now
	var err error
	if o.Resume != "" {
		_, err = r.resume(ctx)
	} else {
		_, err = r.begin(ctx)
	}
	if err != nil {
		// A failed start must not leave the worktree locked for the life of the process.
		r.release()
		return nil, err
	}
	return r, nil
}

// gitOut runs git in the project and returns its standard output.
func (r *runner) gitOut(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", r.root}, args...)...)
	cmd.Env = gate.Env(r.o.Config)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// requireClean refuses a worktree with any uncommitted change, untracked files included.
func (r *runner) requireClean() error {
	out, err := r.gitOut("status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		return nil
	}
	ls := lines(out)
	if len(ls) > 5 {
		ls = append(ls[:5], fmt.Sprintf("and %d more", len(ls)-5))
	}
	return fmt.Errorf("%w; commit or remove: %s", ErrDirty, strings.Join(ls, "; "))
}

// takeLocks takes the worktree lock and, once the run directory exists, its own.
func (r *runner) takeWorktreeLock() error {
	gitDir, err := r.gitOut("rev-parse", "--absolute-git-dir")
	if err != nil {
		return err
	}
	l, err := run.TryLock(filepath.Join(strings.TrimSpace(gitDir), "drafter.lock"))
	if err != nil {
		return fmt.Errorf("another drafter run holds this worktree: %w", err)
	}
	r.locks = append(r.locks, l)
	return nil
}

func (r *runner) begin(ctx context.Context) (*runner, error) {
	cfg := r.o.Config
	dry := r.o.DryRun
	if !dry {
		if _, err := run.Prune(cfg.StateDir, cfg.RetentionDays, r.now()); err != nil {
			return nil, err
		}
		if err := r.takeWorktreeLock(); err != nil {
			return nil, err
		}
	}
	if err := r.requireClean(); err != nil {
		return nil, err
	}
	if err := r.o.API.CheckWorkspace(ctx); err != nil {
		return nil, fmt.Errorf("check workspace: %w", err)
	}

	r.state = &run.State{
		Schema: run.StateSchema, DrafterVersion: r.o.Version, Project: r.root,
		Pages: map[string]*run.PageState{}, Inputs: map[string]run.Input{},
	}
	repo, err := run.Repo(r.root)
	if err != nil {
		return nil, err
	}
	r.state.Repo = repo
	id := run.NewID(r.now())
	r.suffix = dryID
	if !dry {
		r.state.RunID = id
		r.suffix = id[strings.IndexByte(id, '-')+1:]
		if r.dir, err = run.Create(cfg.StateDir, repo, id); err != nil {
			return nil, err
		}
		l, err := run.TryLock(filepath.Join(r.dir, "run.lock"))
		if err != nil {
			return nil, err
		}
		r.locks = append(r.locks, l)
	} else {
		r.state.RunID = "dry-run"
	}
	branch, err := r.gitOut("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, err
	}
	head, err := r.gitOut("rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	r.state.Branch, r.state.Head = strings.TrimSpace(branch), strings.TrimSpace(head)

	if err := r.copyInputs(); err != nil {
		return nil, err
	}
	if err := r.addPages(); err != nil {
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
	if !dry {
		if err := r.save(); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// copyInputs copies the prompts module, each seat's agent definition, and the resolved
// configuration into the run directory and builds the run's configuration from the copies.
func (r *runner) copyInputs() error {
	cfg := *r.o.Config
	cfg.Seat = maps.Clone(r.o.Config.Seat)
	r.cfg = &cfg
	if r.o.DryRun {
		return nil
	}
	put := func(role, name string, data []byte) (string, error) {
		path := filepath.Join(r.dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		if err := atomicfile.WriteFile(path, data, 0o644); err != nil {
			return "", err
		}
		r.state.Inputs[role] = run.Input{Name: name, Hash: hashBytes(data)}
		return path, nil
	}
	module, err := os.ReadFile(r.cfg.Prompts)
	if err != nil {
		return fmt.Errorf("read prompts module: %w", err)
	}
	if r.cfg.Prompts, err = put("prompts", "prompts.js", module); err != nil {
		return err
	}
	for name, s := range r.cfg.Seat {
		if s.Agent == "" {
			continue
		}
		data, err := os.ReadFile(s.Agent)
		if err != nil {
			return fmt.Errorf("read agent definition of seat %s: %w", name, err)
		}
		if s.Agent, err = put("agent."+name, "agents/"+name+".md", data); err != nil {
			return err
		}
		r.cfg.Seat[name] = s
	}
	rows := r.o.Config.Sources()
	for i := range rows {
		if rows[i].Key == "key_command" && rows[i].Value != "" && rows[i].Value != "unset" {
			rows[i].Value = "(set)"
		}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	if _, err = put("config", "config.json", append(data, '\n')); err != nil {
		return err
	}
	fp, err := fingerprintJSON(r.o.Config)
	if err != nil {
		return err
	}
	_, err = put("fingerprint", "fingerprint.json", fp)
	return err
}

// addPages makes a page of every handoff and takes its pre-run snapshots.
func (r *runner) addPages() error {
	seen := map[string]bool{}
	for _, h := range r.o.Handoffs {
		if err := h.Verify(); err != nil {
			return err
		}
		if seen[h.Page.Path] {
			return fmt.Errorf("page %s is in the run twice", h.Page.Path)
		}
		seen[h.Page.Path] = true
		p, err := r.newPage(h)
		if err != nil {
			return err
		}
		p.ps = &run.PageState{Status: StatusPending, Step: stPlan, Pre: map[string]string{}}
		if p.blueprint {
			p.ps.Step = stBpCheck
		}
		p.w = work{Rec: newRecord()}
		if sc, ok := r.o.Scopes[h.Page.Path]; ok {
			p.w.Since, p.w.Scope, p.w.Change = r.o.Since, &sc, r.o.Changes[h.Page.Path]
		}
		r.state.Pages[p.key] = p.ps
		r.pages = append(r.pages, p)
		if err := p.snapshotPre(); err != nil {
			return err
		}
		p.initRecord()
	}
	return nil
}

// loadModules loads the prompts module once per distinct handoff context.
func (r *runner) loadModules() error {
	src, err := os.ReadFile(r.cfg.Prompts)
	if err != nil {
		return fmt.Errorf("read prompts module: %w", err)
	}
	for _, p := range r.pages {
		if p.mod, err = r.moduleFor(p.h, src, filepath.Base(r.o.Config.Prompts)); err != nil {
			return err
		}
	}
	return nil
}

// hostFor builds the host a module loads under: the handoff's context, or, for a handoff with
// none, the run's own pages.
func hostFor(h handoff.Handoff, all []handoff.Handoff) (prompts.Host, error) {
	if len(h.Context) > 0 {
		var c struct {
			Args     json.RawMessage `json:"args"`
			Resolved json.RawMessage `json:"resolved"`
		}
		if err := json.Unmarshal(h.Context, &c); err != nil {
			return prompts.Host{}, fmt.Errorf("handoff %s: context is not {args, resolved}: %w", h.Page.Path, err)
		}
		if len(c.Args) == 0 {
			c.Args = json.RawMessage("{}")
		}
		if len(c.Resolved) == 0 {
			return prompts.Host{}, fmt.Errorf("handoff %s: context has no resolved pages", h.Page.Path)
		}
		return prompts.Host{Args: c.Args, Resolved: c.Resolved}, nil
	}
	type entry struct {
		Path string `json:"path"`
		Job  string `json:"job"`
	}
	var pages []entry
	for _, o := range all {
		pages = append(pages, entry{o.Page.Path, o.Inputs.Job})
	}
	resolved, err := json.Marshal(map[string]any{"pages": pages, "index": ""})
	if err != nil {
		return prompts.Host{}, err
	}
	return prompts.Host{Args: json.RawMessage("{}"), Resolved: resolved}, nil
}

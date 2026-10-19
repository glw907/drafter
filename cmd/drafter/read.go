package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/glw907/drafter/internal/assemble"
	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/handoff"
	"github.com/glw907/drafter/internal/key"
	"github.com/glw907/drafter/internal/pipeline"
	"github.com/glw907/drafter/internal/plan"
	runstate "github.com/glw907/drafter/internal/run"
	"github.com/spf13/cobra"
)

// statusUnchanged is the status of a page a read did not send: nothing changed since the revision
// given to --since, or the page is not part of the run being resumed.
const statusUnchanged = "unchanged"

type readFlags struct {
	resume, since      string
	dryRun, sync, json bool
}

func newReadCmd() *cobra.Command {
	var f readFlags
	cmd := &cobra.Command{
		Use:   "read [flags] <blueprint>...",
		Short: "Check blueprints and read them through the API",
		Long: `Run stage 1 of the pipeline on each blueprint: the local check, then one batch holding the
consistency read, when the project has a facts store, and the blueprint read. A blueprint that
passes both is accepted, and when the whole run ends with no stop, drafter records each
accepted blueprint's blob id in drafted.lock. It writes no page. The tree must be clean, so each
read runs on a committed blueprint, and every input is read from the commit the run starts at.

The command needs [seat.blueprint] in the configuration, and the same required settings as
drafter page, gate.command included, though it never runs the gate. --dry-run counts the tokens of the
stage's requests and submits nothing. --resume continues a stopped run; give the same
blueprints again.

--since <rev> reads the change since that revision. A page with no change is not read. A page
that changed is read whole, with the diff of its blueprint, the changed fact bullets, and its
outline entry alongside, and it is blocked only on a defect the change adds, alters, or causes;
an older defect is reported as already present and does not block. The consistency read blocks
only on a conflict that touches a fact the change touched. A page with no earlier blueprint to
diff against is read as a whole. A change read writes no drafted.lock entry. A resume takes no
--since: the run recorded each page's scope.

The output has one line per blueprint with its verdict, scope, and cost. Under it, one line per
finding of the blueprint read, marked blocking or non-blocking, then one per warning and the
detail of an escalation, so a fix can start from the output. With --json each page carries
these as findings (location, text, blocking), warnings, and detail, left out when empty. A
stopped run prints the command that resumes it, with the blueprints.

The exit status is 0 when every blueprint was accepted, 1 when any was set aside, 2 when the run
could not start or hit an error, and 3 when the run stopped and can be resumed, which outranks 1.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usageError(cmd, "read needs at least one blueprint file")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if f.since != "" && f.resume != "" {
				return usageError(cmd, "--since and --resume cannot be used together; the run recorded its scope")
			}
			return runRead(cmd, args, &f)
		},
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.resume, "resume", "", "continue the stopped run with this ID")
	cmd.Flags().StringVar(&f.since, "since", "", "read the change in each blueprint since this revision")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "estimate the stage and submit nothing")
	cmd.Flags().BoolVar(&f.sync, "sync", false, "send ordinary Messages calls instead of a batch")
	cmd.Flags().BoolVar(&f.json, "json", false, "print the result as one JSON object")
	return cmd
}

func runRead(cmd *cobra.Command, args []string, f *readFlags) error {
	cannot := func(err error) error { return exitError{code: exitCannotRun, msg: err.Error()} }
	first, err := filepath.Abs(args[0])
	if err != nil {
		return cannot(err)
	}
	root, err := plan.GitRoot(filepath.Dir(first))
	if err != nil {
		return cannot(err)
	}
	c, err := config.Load(root, config.Flags{Sync: f.sync, JSON: f.json})
	if err != nil {
		return cannot(err)
	}
	if err := c.Validate(config.ScopeRun); err != nil {
		return cannot(err)
	}
	if err := c.ValidateSeat("blueprint"); err != nil {
		return cannot(fmt.Errorf("drafter read needs the blueprint seat: %w", err))
	}
	head, st, err := startCommit(c, root, f.resume)
	if err != nil {
		return cannot(err)
	}
	handoffs, err := blueprintHandoffs(c, root, head, args)
	if err != nil {
		return cannot(err)
	}
	var skipped []pipeline.PageResult
	var since string
	scopes := map[string]blueprint.Scope{}
	changes := map[string]string{}
	switch {
	case st != nil:
		handoffs, skipped = inRun(handoffs, st)
	case f.since != "":
		if since, err = gitText(root, "rev-parse", "--verify", f.since+"^{commit}"); err != nil {
			return cannot(fmt.Errorf("cannot resolve --since %s: %w", f.since, err))
		}
		handoffs, skipped = scopePages(c, root, since, head, f.since, handoffs, scopes, changes)
	}
	out := cmd.OutOrStdout()
	if len(handoffs) == 0 {
		if st != nil {
			return cannot(fmt.Errorf("none of the blueprints is in run %s", f.resume))
		}
		return writeReadSummary(out, c, pipeline.Summary{Pages: skipped}, f.dryRun)
	}
	k, err := key.Resolve(c)
	if err != nil {
		return cannot(err)
	}
	client := batch.New(batch.Options{Key: k, BaseURL: c.APIBaseURL, WorkspaceID: c.WorkspaceID})
	s, err := pipeline.Run(cmd.Context(), pipeline.Options{
		Config: c, Handoffs: handoffs, API: client, Resume: f.resume, DryRun: f.dryRun, Since: since, Scopes: scopes, Changes: changes,
		Version: version(), Out: cmd.OutOrStdout(), Err: cmd.ErrOrStderr(), Command: "drafter read",
	})
	if err != nil {
		msg := err.Error()
		if s.RunID != "" {
			msg += "; resume with drafter read --resume " + s.RunID
		}
		return cannot(errors.New(msg))
	}
	if err := recordAccepted(c, root, s, f.dryRun); err != nil {
		return cannot(err)
	}
	if !f.dryRun && s.Stop == "" {
		for _, r := range s.Pages {
			if scopedAccept(r) {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s: accepted on a scoped read; drafted.lock keeps its earlier entry\n", r.Path)
			}
		}
	}
	s.Pages = append(s.Pages, skipped...)
	if err := writeReadSummary(out, c, s, f.dryRun); err != nil {
		return err
	}
	code := exitCodeFor(s)
	if f.dryRun {
		code = dryExitCode(s)
	}
	if code != 0 {
		return exitError{code: code}
	}
	return nil
}

// gitText runs git in dir and returns its standard output.
func gitText(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// startCommit is the commit a read's inputs come from: the run's recorded start commit on a
// resume, else HEAD. It returns the loaded state of a resumed run, and nil for a new one.
func startCommit(c *config.Config, root, resume string) (string, *runstate.State, error) {
	if resume == "" {
		head, err := gitText(root, "rev-parse", "HEAD")
		return head, nil, err
	}
	repo, err := runstate.Repo(root)
	if err != nil {
		return "", nil, err
	}
	st, err := runstate.LoadState(filepath.Join(c.StateDir, "runs", repo, resume))
	if err != nil {
		return "", nil, fmt.Errorf("resume %s: %w", resume, err)
	}
	return st.Head, st, nil
}

// inRun keeps the handoffs whose page the resumed run holds. A page the run left out was
// unchanged when it started, so the resume has nothing to say about it.
func inRun(handoffs []handoff.Handoff, st *runstate.State) (kept []handoff.Handoff, skipped []pipeline.PageResult) {
	for _, h := range handoffs {
		if _, ok := st.Pages[runstate.PageKey(h.Page.Path)]; ok {
			kept = append(kept, h)
			continue
		}
		skipped = append(skipped, unchangedPage(h.Page.Path, "unchanged (not in run)"))
	}
	return kept, skipped
}

// unchangedPage is the result of a page the read did not send. Why holds the line to print.
func unchangedPage(path, line string) pipeline.PageResult {
	return pipeline.PageResult{Path: path, Status: statusUnchanged, Scope: &blueprint.Scope{Kind: blueprint.ScopeUnchanged, Why: line}}
}

// scopePages compares each page's blueprint at since with the run's start commit. It records a
// scope in scopes for every page that will be read, and the change block of each scoped page in
// changes, and returns the pages with nothing to read. A scoped page whose diff cannot be built
// reads whole.
func scopePages(c *config.Config, root, since, head, rev string, handoffs []handoff.Handoff, scopes map[string]blueprint.Scope, changes map[string]string) (kept []handoff.Handoff, skipped []pipeline.PageResult) {
	for _, h := range handoffs {
		old, cur := revisionAt(c, root, since, h), revisionAt(c, root, head, h)
		sc := blueprint.Compare(h.Plan, old, cur)
		if sc.Kind == blueprint.ScopeUnchanged {
			skipped = append(skipped, unchangedPage(h.Page.Path, "nothing changed since "+rev))
			continue
		}
		if sc.Kind == blueprint.ScopeScoped {
			block, err := changeBlock(root, since, head, h.Plan, sc, old, cur)
			if err != nil {
				sc = blueprint.Scope{Kind: blueprint.ScopeWhole, Why: "the change could not be built: " + err.Error()}
			} else {
				changes[h.Page.Path] = block
			}
		}
		scopes[h.Page.Path] = sc
		kept = append(kept, h)
	}
	return kept, skipped
}

// changeBlock builds the block a scoped read judges: the blueprint's diff between the two
// commits, each touched fact whose bullet changed, and the outline entry when it changed. The diff
// flags keep a user's git configuration out of the bytes.
func changeBlock(root, since, head, file string, sc blueprint.Scope, old, cur blueprint.Revision) (string, error) {
	diff, err := exec.Command("git", "-C", root, "diff", "--no-color", "--no-ext-diff", "--no-textconv", since, head, "--", file).Output()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	var facts []assemble.FactChange
	for _, id := range sc.Facts {
		if !blueprint.FactChanged(old.Facts, cur.Facts, id) {
			continue
		}
		fc := assemble.FactChange{ID: id}
		if f, ok := old.Facts.Get(id); ok {
			fc.Old = &f
		}
		if f, ok := cur.Facts.Get(id); ok {
			fc.New = &f
		}
		facts = append(facts, fc)
	}
	var oldEntry, newEntry []byte
	if old.Entry != nil && cur.Entry != nil {
		if oldEntry, err = pipeline.EntryJSON(*old.Entry); err != nil {
			return "", err
		}
		if newEntry, err = pipeline.EntryJSON(*cur.Entry); err != nil {
			return "", err
		}
		if bytes.Equal(oldEntry, newEntry) {
			oldEntry, newEntry = nil, nil
		}
	}
	return assemble.ChangeBlock(string(diff), facts, oldEntry, newEntry), nil
}

// revisionAt gathers a page's blueprint, outline entry, and facts store as committed at rev. What
// rev lacks or cannot decode comes back empty, which Compare reads as a reason to read the whole
// page.
func revisionAt(c *config.Config, root, rev string, h handoff.Handoff) blueprint.Revision {
	show := func(rel string) ([]byte, error) {
		return exec.Command("git", "-C", root, "show", rev+":"+rel).Output()
	}
	var r blueprint.Revision
	if out, err := show(h.Plan); err == nil {
		r.Blueprint = out
	}
	if out, err := show(c.Blueprints.Outline); err == nil {
		if o, fs := blueprint.DecodeOutline(c.Blueprints.Outline, out); o != nil && !plan.HasError(fs) {
			for i := range o.Pages {
				if path.Clean(o.Pages[i].Path) == h.Page.Path {
					r.Entry = &o.Pages[i]
				}
			}
		}
	}
	if c.Facts.Dir == "" {
		return r
	}
	dir, err := filepath.Rel(root, c.Facts.Dir)
	if err != nil {
		return r
	}
	dir = filepath.ToSlash(dir)
	files := map[string]string{}
	names, err := gitText(root, "ls-tree", "--name-only", rev+":"+dir)
	if err != nil {
		// A facts directory git cannot list at rev (or a path that is not under root as git
		// reports it) would parse as an empty store and hide a fact-only change, so the page
		// reads whole instead.
		return blueprint.Revision{}
	}
	for name := range strings.SplitSeq(names, "\n") {
		if !strings.HasSuffix(name, ".md") || name == "README.md" {
			continue
		}
		if out, err := show(dir + "/" + name); err == nil {
			files[name] = string(out)
		}
	}
	r.Facts = plan.ParseFacts(files, root)
	return r
}

// scopedAccept reports whether the page was accepted on a change read, which judged the diff and
// so says nothing of the rest of the blueprint.
func scopedAccept(r pipeline.PageResult) bool {
	return r.Status == pipeline.StatusAccepted && r.Accepted != "" && r.Scope != nil && r.Scope.Kind == blueprint.ScopeScoped
}

// writeReadSummary prints the run's summary as JSON or text, whichever the config selects.
func writeReadSummary(w io.Writer, c *config.Config, s pipeline.Summary, dry bool) error {
	if c.Output == "json" {
		return writeSummaryJSON(w, c, s)
	}
	return writeSummaryText(w, c, s, dry)
}

// blueprintHandoffs builds one handoff per blueprint argument. Each page's outline entry comes
// from the outline as committed at head, so a resume describes the page as the run started.
func blueprintHandoffs(c *config.Config, root, head string, args []string) ([]handoff.Handoff, error) {
	outline, err := gitText(root, "show", head+":"+c.Blueprints.Outline)
	if err != nil {
		return nil, fmt.Errorf("read the outline at %s: %w", head, err)
	}
	o, fs := blueprint.DecodeOutline(c.Blueprints.Outline, []byte(outline))
	if o == nil || plan.HasError(fs) {
		return nil, fmt.Errorf("the outline at %s is malformed; run drafter check", c.Blueprints.Outline)
	}
	var out []handoff.Handoff
	for _, a := range args {
		rel, err := repoRelative(root, a)
		if err != nil {
			return nil, err
		}
		pagePath, ok := blueprint.PagePath(c.Blueprints.Root, rel)
		if !ok {
			return nil, fmt.Errorf("%s is not a .blueprint.yaml file under %s", rel, c.Blueprints.Root)
		}
		var entry *blueprint.Page
		for i := range o.Pages {
			if path.Clean(o.Pages[i].Path) == pagePath {
				entry = &o.Pages[i]
			}
		}
		if entry == nil || !entry.Managed {
			return nil, fmt.Errorf("%s names the page %s, which the outline does not list as managed", rel, pagePath)
		}
		sum, err := handoff.Checksum(root, rel, "")
		if err != nil {
			return nil, err
		}
		ctx, err := json.Marshal(map[string]any{"resolved": map[string]any{"pages": []any{entryJSON(*entry)}}})
		if err != nil {
			return nil, err
		}
		out = append(out, handoff.Handoff{
			Version: handoff.Version, Project: root,
			Page:     handoff.Page{Path: pagePath, Brief: strings.TrimSuffix(blueprint.BlueprintPath(c.Blueprints.Root, pagePath), ".blueprint.yaml") + ".brief.json", Type: entry.Type},
			Plan:     rel,
			Inputs:   handoff.Inputs{Job: entry.Job},
			Context:  ctx,
			Checksum: sum, File: a,
		})
	}
	return out, nil
}

// entryJSON is an outline entry in the shape of the chain's JSON outlines, which the prompts
// module reads from resolved.pages.
func entryJSON(p blueprint.Page) map[string]any {
	return map[string]any{
		"path": p.Path, "slug": p.Slug, "title": p.Title, "pageType": p.Type, "job": p.Job,
		"covers": nonNil(p.Covers), "outOfScope": nonNil(p.OutOfScope), "figure": p.Figure,
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// recordAccepted writes each accepted blueprint's blob id into drafted.lock as the accepted id,
// under the worktree lock. It writes only when an accepted id changed, so a read that changes
// nothing leaves the file as it was.
//
// A dry run and a run that stopped write nothing: the pipeline released its locks when it
// returned, so only a run that ended with every page decided has accepted ids worth recording,
// and a crash before this write loses an acceptance that reading again recovers.
func recordAccepted(c *config.Config, root string, s pipeline.Summary, dry bool) error {
	if dry || s.Stop != "" {
		return nil
	}
	gitDir, err := gitText(root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return err
	}
	l, err := runstate.TryLock(filepath.Join(gitDir, "drafter.lock"))
	if err != nil {
		return fmt.Errorf("another drafter run holds this worktree: %w", err)
	}
	defer func() { _ = l.Release() }()
	file := filepath.Join(root, filepath.FromSlash(path.Join(path.Dir(c.Blueprints.Outline), "drafted.lock")))
	lock, err := blueprint.ReadLock(file)
	if err != nil {
		return err
	}
	changed := false
	for _, r := range s.Pages {
		if r.Status != pipeline.StatusAccepted || r.Accepted == "" || scopedAccept(r) {
			continue
		}
		e := lock.Pages[r.Path]
		if e.Accepted == r.Accepted {
			continue
		}
		e.Accepted = r.Accepted
		if lock.Pages == nil {
			lock.Pages = map[string]blueprint.LockEntry{}
		}
		lock.Pages[r.Path] = e
		changed = true
	}
	if !changed {
		return nil
	}
	return blueprint.WriteLock(file, lock)
}

package main

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/handoff"
	"github.com/glw907/drafter/internal/key"
	"github.com/glw907/drafter/internal/pipeline"
	"github.com/glw907/drafter/internal/plan"
	"github.com/spf13/cobra"
)

// runPageBlueprints drafts the pages the arguments name from their blueprints.
func runPageBlueprints(cmd *cobra.Command, args []string, f *pageFlags) error {
	cannot := func(err error) error { return exitError{code: exitCannotRun, msg: err.Error()} }
	root, err := projectRoot(args[0])
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
	for _, seat := range []string{"blueprint", "coherence"} {
		if err := c.ValidateSeat(seat); err != nil {
			return cannot(fmt.Errorf("drafter page on a blueprint needs the %s seat: %w", seat, err))
		}
	}
	blueprints, err := blueprintArgs(c, root, args)
	if err != nil {
		return cannot(err)
	}
	head, st, err := startCommit(c, root, f.resume)
	if err != nil {
		return cannot(err)
	}
	handoffs, err := blueprintHandoffs(c, root, head, blueprints)
	if err != nil {
		return cannot(err)
	}
	var d draftPlan
	if st != nil {
		d.handoffs, d.skipped = inRun(handoffs, st)
	} else if d, err = scopeFromLock(c, root, head, handoffs, f.whole, cmd.ErrOrStderr()); err != nil {
		return cannot(err)
	}
	if len(d.handoffs) == 0 {
		if st != nil {
			return cannot(fmt.Errorf("none of the pages is in run %s", f.resume))
		}
		s := pipeline.Summary{Pages: d.skipped}
		if name, src, err := c.Module(); err == nil {
			s.Prompts, s.PromptsHash = name, config.ShortHash(src)
		} else {
			s.Prompts = c.ModuleName()
		}
		return finishPage(cmd, c, s, f.dryRun)
	}
	if err := checkStockTracks(c, root, head, d.handoffs); err != nil {
		return cannot(err)
	}
	if err := stockGatePreflight(c.Gate.Command, root); err != nil {
		return cannot(err)
	}
	k, err := key.Resolve(c)
	if err != nil {
		return cannot(err)
	}
	client := batch.New(batch.Options{Key: k, BaseURL: c.APIBaseURL, WorkspaceID: c.WorkspaceID})
	s, err := pipeline.Run(cmd.Context(), pipeline.Options{
		Config: c, Handoffs: d.handoffs, API: client, Resume: f.resume, DryRun: f.dryRun,
		Scopes: d.scopes, Changes: d.changes, Draft: true, SkipRead: d.skipRead,
		ReadScopes: d.read.scopes, ReadChanges: d.read.changes, Bars: d.read.bars, BarFrom: d.read.barFrom,
		Version: version(), Out: cmd.OutOrStdout(), Err: cmd.ErrOrStderr(), ResumeArgs: args,
	})
	if err != nil {
		msg := err.Error()
		if s.RunID != "" {
			msg += "; resume with " + pipeline.ResumeCommand("drafter page", c.Mode, s.RunID, args)
		}
		return cannot(errors.New(msg))
	}
	lockErr := recordDrafted(c, root, head, d.handoffs, s, f.dryRun, cmd.ErrOrStderr())
	s.Pages = append(s.Pages, d.skipped...)
	if err := writeReadSummary(cmd.OutOrStdout(), c, s, f.dryRun); err != nil {
		return err
	}
	if lockErr != nil {
		msg := lockErr.Error()
		if s.RunID != "" {
			msg += "; resume with " + pipeline.ResumeCommand("drafter page", c.Mode, s.RunID, args)
		}
		return cannot(errors.New(msg))
	}
	return pageExit(s, f.dryRun)
}

// projectRoot is the git root of the project an argument sits in. The argument may name a page
// whose directory does not exist yet, so the search starts at its nearest existing ancestor.
func projectRoot(arg string) (string, error) {
	abs, err := filepath.Abs(arg)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(abs)
	for {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return plan.GitRoot(dir)
}

// blueprintArgs turns each argument into the path of its blueprint: a blueprint stays as given,
// and a page maps to the blueprint at the page's path under the blueprint root.
func blueprintArgs(c *config.Config, root string, args []string) ([]string, error) {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if !strings.HasSuffix(a, ".md") {
			out = append(out, a)
			continue
		}
		rel, err := repoRelative(root, a)
		if err != nil {
			return nil, err
		}
		out = append(out, filepath.Join(root, filepath.FromSlash(blueprint.BlueprintPath(c.Blueprints.Root, rel))))
	}
	return out, nil
}

// checkStockTracks refuses, before any request, a page the stock module cannot draft: the stock
// module carries only the Google standard. A project module is not checked.
func checkStockTracks(c *config.Config, root, head string, handoffs []handoff.Handoff) error {
	if c.Prompts != "" {
		return nil
	}
	data, err := gitShow(root, head, c.Blueprints.Outline)
	if err != nil {
		return fmt.Errorf("read the outline at %s: %w", head, err)
	}
	o, _ := blueprint.DecodeOutline(c.Blueprints.Outline, data)
	if o == nil {
		return fmt.Errorf("the outline at %s is malformed; run drafter check", c.Blueprints.Outline)
	}
	var reasons []string
	for _, h := range handoffs {
		if why := stockTrackRefusal(o, h.Page.Path); why != "" {
			reasons = append(reasons, why)
		}
	}
	if len(reasons) > 0 {
		return errors.New(strings.Join(reasons, "\n"))
	}
	return nil
}

// recordDrafted writes the lock entry of each page the run accepted, under the worktree lock. It
// writes only when an entry changed, and writes nothing for a dry run or a run that stopped: the
// resume writes the entries when it finishes.
//
// The reads' outcomes are written first, in the same write (recordReads): an accept that judged
// the whole blueprint, or the change since a bar, sets the accepted id and clears the bar, and a
// whole read that blocked records its bar. A scoped read here diffs from the entry's own
// blueprint, so a scoped accept sets the accepted id only when the entry holds no bar and its
// accepted id is its blueprint; otherwise the entry stays and warn says why. A drafted page's
// entry then keeps the accepted id recordReads left, so a skipped read keeps the id the entry
// held, and it carries no bar.
func recordDrafted(c *config.Config, root, start string, handoffs []handoff.Handoff, s pipeline.Summary, dry bool, warn io.Writer) error {
	if dry || s.Stop != "" {
		return nil
	}
	plans := map[string]string{}
	for _, h := range handoffs {
		plans[h.Page.Path] = h.Plan
	}
	return updateLock(c, root, func(lock *blueprint.Lock) (bool, error) {
		bases := map[string]string{}
		for p, e := range lock.Pages {
			bases[p] = e.Blueprint
		}
		read, kept := recordReads(lock, start, s, bases)
		scopedKept(warn, s, kept)
		drafted := false
		for _, r := range s.Pages {
			bp, ok := plans[r.Path]
			if !ok || r.Status != pipeline.StatusAccepted {
				continue
			}
			prior := lock.Pages[r.Path]
			e, err := blueprint.DraftedEntry(root, start, bp, r.Path, prior.Accepted, prior.Facts)
			if err != nil {
				return false, err
			}
			if e.Page == prior.Page && e.Blueprint == prior.Blueprint && e.Accepted == prior.Accepted && maps.Equal(e.Facts, prior.Facts) && maps.Equal(e.Sections, prior.Sections) && prior.Blocked == nil {
				continue
			}
			if lock.Pages == nil {
				lock.Pages = map[string]blueprint.LockEntry{}
			}
			lock.Pages[r.Path] = e
			drafted = true
		}
		if drafted {
			warnMovedInputs(c, root, start, warn)
		}
		return read || drafted, nil
	})
}

// warnMovedInputs names the facts and outline files that changed between the start commit and HEAD.
// The next run reads the entry's draft as the commit before the lock's, so an input committed on
// its own after the run started can go unseen.
func warnMovedInputs(c *config.Config, root, start string, w io.Writer) {
	head, err := gitText(root, "rev-parse", "HEAD")
	if err != nil || head == start {
		return
	}
	paths := []string{c.Blueprints.Outline}
	if c.Facts.Dir != "" {
		if rel, err := filepath.Rel(root, c.Facts.Dir); err == nil && filepath.IsLocal(rel) {
			paths = append(paths, filepath.ToSlash(rel))
		}
	}
	out, err := gitText(root, append([]string{"diff", "--name-only", start, head, "--"}, paths...)...)
	if err != nil || out == "" {
		return
	}
	_, _ = fmt.Fprintf(w, "warning: %s changed after the run started, so the next run may not see the change; commit the page, its brief, and drafted.lock with nothing between them and the run's start\n", strings.ReplaceAll(out, "\n", ", "))
}

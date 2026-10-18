package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

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

type readFlags struct {
	resume             string
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

The exit status is 0 when every blueprint was accepted, 1 when any was set aside, 2 when the run
could not start or hit an error, and 3 when the run stopped and can be resumed, which outranks 1.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usageError(cmd, "read needs at least one blueprint file")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error { return runRead(cmd, args, &f) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.resume, "resume", "", "continue the stopped run with this ID")
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
	head, err := startCommit(c, root, f.resume)
	if err != nil {
		return cannot(err)
	}
	handoffs, err := blueprintHandoffs(c, root, head, args)
	if err != nil {
		return cannot(err)
	}
	k, err := key.Resolve(c)
	if err != nil {
		return cannot(err)
	}
	client := batch.New(batch.Options{Key: k, BaseURL: c.APIBaseURL, WorkspaceID: c.WorkspaceID})
	s, err := pipeline.Run(cmd.Context(), pipeline.Options{
		Config: c, Handoffs: handoffs, API: client, Resume: f.resume, DryRun: f.dryRun,
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
	out := cmd.OutOrStdout()
	if c.Output == "json" {
		err = writeSummaryJSON(out, s)
	} else {
		err = writeSummaryText(out, s, f.dryRun)
	}
	if err != nil {
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
// resume, else HEAD.
func startCommit(c *config.Config, root, resume string) (string, error) {
	if resume == "" {
		return gitText(root, "rev-parse", "HEAD")
	}
	repo, err := runstate.Repo(root)
	if err != nil {
		return "", err
	}
	st, err := runstate.LoadState(filepath.Join(c.StateDir, "runs", repo, resume))
	if err != nil {
		return "", fmt.Errorf("resume %s: %w", resume, err)
	}
	return st.Head, nil
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
		if r.Status != pipeline.StatusAccepted || r.Accepted == "" {
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

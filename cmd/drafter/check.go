package main

import (
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/plan"
	"github.com/spf13/cobra"
)

type checkFlags struct {
	root string
	json bool
}

func newCheckCmd() *cobra.Command {
	var f checkFlags
	cmd := &cobra.Command{
		Use:   "check [flags] [<blueprint>...]",
		Short: "Validate the project's blueprints",
		Long: `Validate every blueprint under the project's blueprint root, or the ones named, offline.
It decodes each blueprint, runs the plan checks on its view (ids, reader state, recovery,
links, cross-links, and fact citability), and adds the blueprint checks: each managed page
has one blueprint, every [[id]] reference resolves, every section says why, every use opens
with Only or Omit, page types declare an anatomy, drafted.lock agrees with the disk and
the outline. The handler check warns when a failure is handled by a fact, a verbatim sentence,
or a link or handoff back to the section of the failing step, since none gives the reader a
next action. With no blueprint named it also verifies the facts store: each fact's id, tag, and
source pointers against the repository. Findings print as file:line: severity: check: message
(fix). The exit status is 0 when no finding is an error, 1 when any is, and 2 when the command
could not run.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runCheck(cmd, args, &f) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.root, "root", "", "project root (default: the git top level of the working directory)")
	cmd.Flags().BoolVar(&f.json, "json", false, "print the findings as one JSON array")
	return cmd
}

// projectFor resolves the project root and the blueprint settings for a command. Every error is
// one the command cannot run past, so it comes back as exit status 2.
func projectFor(rootFlag string) (blueprint.Project, error) {
	root := rootFlag
	if root == "" {
		var err error
		if root, err = plan.GitRoot("."); err != nil {
			return blueprint.Project{}, exitError{code: exitCannotRun, msg: err.Error()}
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return blueprint.Project{}, exitError{code: exitCannotRun, msg: err.Error()}
	}
	cfg, err := config.Load(root, config.Flags{})
	if err != nil {
		return blueprint.Project{}, exitError{code: exitCannotRun, msg: err.Error()}
	}
	return blueprint.Project{
		Root:          root,
		BlueprintRoot: path.Clean(cfg.Blueprints.Root),
		Outline:       path.Clean(cfg.Blueprints.Outline),
		FactsDir:      cfg.Facts.Dir,
		Required:      cfg.Blueprints.Required,
	}, nil
}

// repoRelative turns a file argument, relative to the working directory, into a slash path
// relative to root.
func repoRelative(root, arg string) (string, error) {
	abs, err := filepath.Abs(arg)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%s is outside the project root %s", arg, root)
	}
	return filepath.ToSlash(rel), nil
}

func runCheck(cmd *cobra.Command, args []string, f *checkFlags) error {
	p, err := projectFor(f.root)
	if err != nil {
		return err
	}
	named := make([]string, 0, len(args))
	for _, a := range args {
		rel, err := repoRelative(p.Root, a)
		if err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
		named = append(named, rel)
	}
	findings, err := p.Check(named)
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	storeLine := ""
	if len(named) == 0 && p.FactsDir != "" {
		store, checked, skipped, err := plan.VerifyFacts(p.FactsDir, p.Root, nil)
		if err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
		findings = append(findings, store...)
		storeLine = fmt.Sprintf("facts store: %d pointers checked, %d skipped\n", checked, skipped)
	}
	out, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	if f.json {
		if err := writeJSON(out, findings); err != nil {
			return err
		}
	} else {
		var b strings.Builder
		for _, x := range findings {
			fmt.Fprintf(&b, "%s:%d: %s: %s: %s (%s)\n", x.File, x.Line, x.Severity, x.Check, x.Message, x.Fix)
		}
		if _, err := io.WriteString(out, b.String()); err != nil {
			return err
		}
	}
	errs, warns := count(findings)
	if _, err := io.WriteString(stderr, storeLine+plural(errs, "error")+", "+plural(warns, "warning")+"\n"); err != nil {
		return err
	}
	if errs > 0 {
		return exitError{code: exitFindings}
	}
	return nil
}

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/internal/vale"
	"github.com/spf13/cobra"
)

type gateFlags struct {
	root, page string
}

func newGateCmd() *cobra.Command {
	var f gateFlags
	cmd := &cobra.Command{
		Use:   "gate --page <page> [flags]",
		Short: "Check one drafted page: its blueprint, cited facts, links, and prose style",
		Long: `Check the one page the stock gate.command names. It runs from the git top level of the
working directory, or from --root, and makes no network call. In order it checks:

1. That Vale can run: vale is on PATH, the project has a .vale.ini, and each style it names is
   installed under the project.
2. That the page file exists.
3. The page's blueprint, if it has one: the blueprint checks, then the facts the blueprint places
   or cites, against the repository. With no blueprint the command says so and goes on.
4. The page's internal links: each relative link resolves to a file in the repository, and each
   #anchor matches a heading.
5. Vale, at the error level, on the page.

Findings print as file:line: severity: check: message (fix), and Vale's alerts as it prints them.
The exit status is 0 when nothing failed, 1 when any finding is an error or Vale reported an
error-level alert, and 2 when the command could not run.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runGate(cmd, &f) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.page, "page", "", "the page to check, a path in the project (required)")
	cmd.Flags().StringVar(&f.root, "root", "", "project root (default: the git top level of the working directory)")
	return cmd
}

func runGate(cmd *cobra.Command, f *gateFlags) error {
	if f.page == "" {
		return usageError(cmd, "gate needs --page <page>")
	}
	p, err := projectFor(f.root)
	if err != nil {
		return err
	}
	if err := vale.Preflight(p.Root); err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	page, err := gatePage(p.Root, f.page)
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	out := cmd.OutOrStdout()
	var findings []plan.Finding
	bp := blueprint.BlueprintPath(p.BlueprintRoot, page)
	switch _, err := os.Stat(filepath.Join(p.Root, filepath.FromSlash(bp))); {
	case err == nil:
		bpFindings, err := gateBlueprint(p, bp)
		if err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
		findings = append(findings, bpFindings...)
	case os.IsNotExist(err):
		if _, err := fmt.Fprintf(out, "no blueprint for %s; blueprint and fact checks skipped\n", page); err != nil {
			return err
		}
	default:
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	links, err := plan.CheckPageLinks(p.Root, page)
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	findings = append(findings, links...)
	var b strings.Builder
	for _, x := range findings {
		fmt.Fprintf(&b, "%s:%d: %s: %s: %s (%s)\n", x.File, x.Line, x.Severity, x.Check, x.Message, x.Fix)
	}
	if _, err := io.WriteString(out, b.String()); err != nil {
		return err
	}
	code, err := vale.Lint(p.Root, page, out)
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	errs, warns := count(findings)
	if _, err := io.WriteString(cmd.ErrOrStderr(), plural(errs, "error")+", "+plural(warns, "warning")+"\n"); err != nil {
		return err
	}
	if errs > 0 || code != 0 {
		return exitError{code: exitFindings}
	}
	return nil
}

// gatePage resolves the --page argument to a slash path inside root that names a file.
func gatePage(root, arg string) (string, error) {
	rel, err := repoRelative(root, arg)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil || info.IsDir() {
		return "", fmt.Errorf("no page at %s", rel)
	}
	return rel, nil
}

// gateBlueprint checks the one blueprint at bp, then the facts it places or cites.
func gateBlueprint(p blueprint.Project, bp string) ([]plan.Finding, error) {
	findings, err := p.Check([]string{bp})
	if err != nil {
		return nil, err
	}
	ids, err := p.Cited(bp)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 || p.FactsDir == "" {
		return findings, nil
	}
	store, _, _, err := plan.VerifyFacts(p.FactsDir, p.Root, ids)
	if err != nil {
		return nil, err
	}
	return append(findings, store...), nil
}

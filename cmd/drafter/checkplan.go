package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/glw907/drafter/internal/plan"
	"github.com/spf13/cobra"
)

type checkPlanFlags struct {
	facts, root string
	outlines    []string
	json        bool
}

func newCheckPlanCmd() *cobra.Command {
	var f checkPlanFlags
	cmd := &cobra.Command{
		Use:   "check-plan [flags] <plan>...",
		Short: "Validate page plans",
		Long: `Decode each plan file, then check its ids, reader state, recovery matrix, links and
anchors, and, with --facts, fact citability. Findings print as file:line: severity: check:
message (fix). The exit status is 0 when no finding is an error, 1 when any is, and 2 when
the command could not run.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usageError(cmd, "check-plan needs at least one plan file")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error { return runCheckPlan(cmd, args, &f) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.facts, "facts", "", "directory of facts files; without it each fact item gets a warning")
	cmd.Flags().StringArrayVar(&f.outlines, "outline", nil, "outline JSON file whose pages array lists planned pages; repeat it to read several")
	cmd.Flags().StringVar(&f.root, "root", "", "repository root that link targets resolve against (default: the git top level of the first plan)")
	cmd.Flags().BoolVar(&f.json, "json", false, "print the findings as one JSON array")
	return cmd
}

func runCheckPlan(cmd *cobra.Command, args []string, f *checkPlanFlags) error {
	opts := plan.CheckOptions{Root: f.root, FactsDir: f.facts}
	if f.facts != "" {
		if err := plan.CheckFactsDir(f.facts); err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
	}
	if len(f.outlines) > 0 {
		o, err := plan.LoadOutlines(f.outlines)
		if err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
		opts.Outline = o
	}
	if opts.Root == "" {
		root, err := plan.GitRoot(filepath.Dir(args[0]))
		if err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
		opts.Root = root
	}
	var all []plan.Finding
	for _, file := range args {
		findings, err := plan.CheckFile(file, opts)
		if err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
		all = append(all, findings...)
	}

	out, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	if f.json {
		if err := writeJSON(out, all); err != nil {
			return err
		}
	} else {
		var b strings.Builder
		for _, x := range all {
			fmt.Fprintf(&b, "%s:%d: %s: %s: %s (%s)\n", x.File, x.Line, x.Severity, x.Check, x.Message, x.Fix)
		}
		if _, err := io.WriteString(out, b.String()); err != nil {
			return err
		}
	}
	errs, warns := count(all)
	if _, err := io.WriteString(stderr, plural(errs, "error")+", "+plural(warns, "warning")+"\n"); err != nil {
		return err
	}
	if errs > 0 {
		return exitError{code: exitFindings}
	}
	return nil
}

func writeJSON(w io.Writer, findings []plan.Finding) error {
	if findings == nil {
		findings = []plan.Finding{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(findings)
}

func count(findings []plan.Finding) (errs, warns int) {
	for _, x := range findings {
		if x.Severity == plan.SeverityError {
			errs++
		} else {
			warns++
		}
	}
	return errs, warns
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"
	"strings"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/handoff"
	"github.com/glw907/drafter/internal/key"
	"github.com/glw907/drafter/internal/pipeline"
	"github.com/spf13/cobra"
)

type pageFlags struct {
	workflowRun, resume string
	dryRun, sync, json  bool
}

func newPageCmd() *cobra.Command {
	var f pageFlags
	cmd := &cobra.Command{
		Use:   "page [flags] [handoff-file...]",
		Short: "Run planned pages through the drafting pipeline",
		Long: `Take each handoff file's page through the pipeline: plan check, draft, gate, coverage,
review reads, redraft, and the final reader. The handoffs come from files, or from a chain's
Workflow run file with --workflow-run. The command spends on the API key the configuration names
and never commits.

--dry-run assembles the first stage's requests, counts their tokens, prints each request's size
and the stage's worst-case cost, and submits nothing. --resume continues a stopped run; give the
same handoffs again.

The exit status is 0 when every page was accepted, 1 when any page escalated, 2 when the run could
not start or hit an error, and 3 when the run stopped and can be resumed, which outranks 1.`,
		RunE: func(cmd *cobra.Command, args []string) error { return runPage(cmd, args, &f) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.workflowRun, "workflow-run", "", "read the handoffs from a chain's Workflow run file")
	cmd.Flags().StringVar(&f.resume, "resume", "", "continue the stopped run with this ID")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "estimate the first stage and submit nothing")
	cmd.Flags().BoolVar(&f.sync, "sync", false, "send ordinary Messages calls instead of a batch")
	cmd.Flags().BoolVar(&f.json, "json", false, "print the result as one JSON object")
	return cmd
}

// exitCodeFor maps a run's summary to the exit status of drafter page: a stopped run is 3, which
// outranks a run that finished with an escalated page, 1; a run with every page accepted is 0.
func exitCodeFor(s pipeline.Summary) int {
	if s.Stop != "" {
		return exitStopped
	}
	for _, p := range s.Pages {
		if p.Status != pipeline.StatusAccepted && p.Status != statusUnchanged {
			return exitFindings
		}
	}
	return 0
}

// dryExitCode is the exit status of a dry run: no page is accepted or stopped in one, so only a
// page the plan check already escalated makes it nonzero.
func dryExitCode(s pipeline.Summary) int {
	for _, p := range s.Pages {
		if p.Status == pipeline.StatusEscalated {
			return exitFindings
		}
	}
	return 0
}

func runPage(cmd *cobra.Command, args []string, f *pageFlags) error {
	handoffs, err := readHandoffs(cmd, args, f)
	if err != nil {
		return err
	}
	c, err := config.Load(handoffs[0].Project, config.Flags{Sync: f.sync, JSON: f.json})
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	if err := c.Validate(config.ScopeRun); err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	k, err := key.Resolve(c)
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	client := batch.New(batch.Options{Key: k, BaseURL: c.APIBaseURL, WorkspaceID: c.WorkspaceID})
	s, err := pipeline.Run(cmd.Context(), pipeline.Options{
		Config: c, Handoffs: handoffs, API: client, Resume: f.resume, DryRun: f.dryRun,
		Version: version(), Out: cmd.OutOrStdout(), Err: cmd.ErrOrStderr(),
	})
	if err != nil {
		msg := err.Error()
		if s.RunID != "" {
			msg += "; resume with drafter page --resume " + s.RunID
		}
		return exitError{code: exitCannotRun, msg: msg}
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

func readHandoffs(cmd *cobra.Command, args []string, f *pageFlags) ([]handoff.Handoff, error) {
	switch {
	case f.workflowRun != "" && len(args) > 0:
		return nil, usageError(cmd, "give handoff files or --workflow-run, not both")
	case f.workflowRun != "":
		hs, err := handoff.ReadWorkflowRun(f.workflowRun)
		if err != nil {
			return nil, exitError{code: exitCannotRun, msg: err.Error()}
		}
		return hs, nil
	case len(args) == 0:
		return nil, usageError(cmd, "page needs a handoff file or --workflow-run")
	}
	hs, err := handoff.ReadFiles(args...)
	if err != nil {
		return nil, exitError{code: exitCannotRun, msg: err.Error()}
	}
	return hs, nil
}

// version is the module version the binary was built at, or "dev".
func version() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

func writeSummaryJSON(w io.Writer, s pipeline.Summary) error {
	if s.Pages == nil {
		s.Pages = []pipeline.PageResult{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}

func writeSummaryText(w io.Writer, s pipeline.Summary, dry bool) error {
	var b strings.Builder
	if dry {
		for _, e := range s.Estimates {
			fmt.Fprintf(&b, "%s: %d input tokens, max_tokens %d, worst case $%.4f\n", e.Seat, e.Tokens, e.MaxTokens, e.Worst)
		}
		fmt.Fprintf(&b, "stage %s: estimated worst case $%.4f\n", s.EstimateStage, s.EstimateTotal)
	}
	for _, p := range s.Pages {
		if p.Status == statusUnchanged {
			fmt.Fprintf(&b, "%s: %s\n", p.Path, p.Scope.Why)
			continue
		}
		fmt.Fprintf(&b, "%s: %s", p.Path, p.Status)
		if p.Scope != nil {
			b.WriteString(" [" + scopeText(*p.Scope) + "]")
		}
		if p.Rule != "" {
			fmt.Fprintf(&b, " (%s)", p.Rule)
		}
		if !dry {
			fmt.Fprintf(&b, " $%.4f", p.Cost)
		}
		b.WriteString("\n")
	}
	if s.Stop != "" {
		fmt.Fprintf(&b, "stopped: %s; resume with: %s\n", s.Stop, s.ResumeLine)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// scopeText describes what a read of a page was limited to.
func scopeText(sc blueprint.Scope) string {
	if sc.Kind == blueprint.ScopeScoped {
		return "scoped: " + strings.Join(sc.IDs, ", ")
	}
	return sc.Kind + ": " + sc.Why
}

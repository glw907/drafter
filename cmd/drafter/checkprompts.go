package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/internal/prompts"
	"github.com/spf13/cobra"
)

type checkPromptsFlags struct {
	module, matrix, expect string
	json                   bool
}

func newCheckPromptsCmd() *cobra.Command {
	var f checkPromptsFlags
	cmd := &cobra.Command{
		Use:   "check-prompts [flags]",
		Short: "Render a matrix of seat prompts and check each one",
		Long: `Load the prompts module, render every case of the matrix, and check each prompt. The
module defaults to the project's prompts setting; with --module no configuration loads. The
matrix is a JSON object {host: {args, resolved}, cases: [{name, seat, context}]}. A prompt must
contain its context's page.path and, when the context has findings, the findings text whole and
each narrow.own.findings[].finding. With --expect, a JSON object mapping case names to prompts
(- reads standard input), every case must equal its expected prompt byte for byte, and a name
present on one side only fails. The exit status is 0 when every case passes, 1 when any fails,
and 2 when the module, matrix, or expect file can't be read, parsed, or loaded, or the matrix
has no cases.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runCheckPrompts(cmd, &f) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.module, "module", "", "prompts module file (default: the project's prompts setting)")
	cmd.Flags().StringVar(&f.matrix, "matrix", "", "matrix JSON file (required)")
	cmd.Flags().StringVar(&f.expect, "expect", "", "JSON file mapping case names to expected prompts; - reads standard input")
	cmd.Flags().BoolVar(&f.json, "json", false, "print the results as one JSON array")
	return cmd
}

type promptMatrix struct {
	Host struct {
		Args     json.RawMessage `json:"args"`
		Resolved json.RawMessage `json:"resolved"`
	} `json:"host"`
	Cases []promptCase `json:"cases"`
}

type promptCase struct {
	Name    string          `json:"name"`
	Seat    string          `json:"seat"`
	Context prompts.Context `json:"context"`
}

// promptFailure is one check a case failed.
type promptFailure struct {
	Check   string `json:"check"`
	Message string `json:"message"`
}

// promptResult is one case's outcome in the JSON output.
type promptResult struct {
	Case     string          `json:"case"`
	Seat     string          `json:"seat"`
	Pass     bool            `json:"pass"`
	Failures []promptFailure `json:"failures"`
}

func runCheckPrompts(cmd *cobra.Command, f *checkPromptsFlags) error {
	if f.matrix == "" {
		return usageError(cmd, "check-prompts needs --matrix")
	}
	cannot := func(format string, a ...any) error {
		return exitError{code: exitCannotRun, msg: fmt.Sprintf(format, a...)}
	}
	modulePath := f.module
	if modulePath == "" {
		root, err := plan.GitRoot(".")
		if err != nil {
			return cannot("find the project to read its prompts setting: %v (or pass --module)", err)
		}
		c, err := config.Load(root, config.Flags{})
		if err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
		if c.Prompts == "" {
			return cannot("prompts is not set; set it in drafter.toml or pass --module")
		}
		modulePath = c.Prompts
	}
	src, err := os.ReadFile(modulePath)
	if err != nil {
		return cannot("read module: %v", err)
	}
	raw, err := os.ReadFile(f.matrix)
	if err != nil {
		return cannot("read matrix: %v", err)
	}
	var m promptMatrix
	if err := decodeStrict(raw, &m); err != nil {
		return cannot("matrix %s: %v", f.matrix, err)
	}
	if len(m.Cases) == 0 {
		return cannot("matrix %s has no cases", f.matrix)
	}
	var expect map[string]string
	if f.expect != "" {
		var data []byte
		if f.expect == "-" {
			data, err = io.ReadAll(cmd.InOrStdin())
		} else {
			data, err = os.ReadFile(f.expect)
		}
		if err != nil {
			return cannot("read expect file: %v", err)
		}
		if err := decodeStrict(data, &expect); err != nil {
			return cannot("expect file %s: %v", f.expect, err)
		}
	}
	mod, err := prompts.Load(modulePath, src, prompts.Host{Args: m.Host.Args, Resolved: m.Host.Resolved})
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}

	results := make([]promptResult, 0, len(m.Cases))
	seen := make(map[string]bool, len(m.Cases))
	for _, c := range m.Cases {
		seen[c.Name] = true
		r := promptResult{Case: c.Name, Seat: c.Seat, Failures: []promptFailure{}}
		out, err := mod.Render(c.Seat, c.Context)
		if err != nil {
			r.Failures = append(r.Failures, promptFailure{"render", err.Error()})
		} else {
			r.Failures = append(r.Failures, containment(c.Context, out)...)
			if expect != nil {
				want, ok := expect[c.Name]
				switch {
				case !ok:
					r.Failures = append(r.Failures, promptFailure{"expect", "case missing from the expect file"})
				case want != out:
					r.Failures = append(r.Failures, promptFailure{"expect", firstDifference(want, out)})
				}
			}
		}
		r.Pass = len(r.Failures) == 0
		results = append(results, r)
	}
	for _, name := range slices.Sorted(maps.Keys(expect)) {
		if !seen[name] {
			results = append(results, promptResult{
				Case:     name,
				Failures: []promptFailure{{"expect", "case missing from the matrix"}},
			})
		}
	}

	stdout := cmd.OutOrStdout()
	if f.json {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			return err
		}
	} else {
		var b strings.Builder
		for _, r := range results {
			for _, x := range r.Failures {
				fmt.Fprintf(&b, "%s: %s: %s\n", r.Case, x.Check, x.Message)
			}
		}
		if _, err := io.WriteString(stdout, b.String()); err != nil {
			return err
		}
	}
	failed := 0
	for _, r := range results {
		if !r.Pass {
			failed++
		}
	}
	if _, err := io.WriteString(cmd.ErrOrStderr(), fmt.Sprintf("%d cases, %d failed\n", len(results), failed)); err != nil {
		return err
	}
	if failed > 0 {
		return exitError{code: exitFindings}
	}
	return nil
}

// decodeStrict decodes one JSON value into v, refusing unknown fields and trailing data.
func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("unexpected data after the JSON value")
	}
	return nil
}

// containment checks that out names the context's page and carries its findings text.
func containment(c prompts.Context, out string) []promptFailure {
	var fs []promptFailure
	var page struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(c.Page, &page); err != nil || page.Path == "" {
		fs = append(fs, promptFailure{"page", "the context's page.path is missing"})
	} else if !strings.Contains(out, page.Path) {
		fs = append(fs, promptFailure{"page", fmt.Sprintf("prompt does not contain page.path %q", page.Path)})
	}
	if c.Findings != "" && !strings.Contains(out, c.Findings) {
		fs = append(fs, promptFailure{"findings", "prompt does not contain the findings text whole"})
	}
	var narrow struct {
		Own *struct {
			Findings []struct {
				Finding string `json:"finding"`
			} `json:"findings"`
		} `json:"own"`
	}
	if len(c.Narrow) > 0 && string(c.Narrow) != "null" && json.Unmarshal(c.Narrow, &narrow) == nil && narrow.Own != nil {
		for _, x := range narrow.Own.Findings {
			if x.Finding != "" && !strings.Contains(out, x.Finding) {
				fs = append(fs, promptFailure{"findings", fmt.Sprintf("prompt does not contain narrow.own finding %q", x.Finding)})
			}
		}
	}
	return fs
}

// firstDifference names the first line where got departs from want, numbered from 1.
func firstDifference(want, got string) string {
	wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < max(len(wl), len(gl)); i++ {
		switch {
		case i >= len(wl):
			return fmt.Sprintf("line %d: prompt has an extra line %q", i+1, gl[i])
		case i >= len(gl):
			return fmt.Sprintf("line %d: prompt ends before expected %q", i+1, wl[i])
		case wl[i] != gl[i]:
			return fmt.Sprintf("line %d: want %q, got %q", i+1, wl[i], gl[i])
		}
	}
	return "prompts differ"
}

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/internal/prompts"
	"github.com/glw907/drafter/stock"
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
	var src []byte
	var err error
	var stockRoot string
	if f.module != "" {
		if src, err = os.ReadFile(f.module); err != nil {
			return cannot("read module: %v", err)
		}
	} else {
		root, err := plan.GitRoot(".")
		if err != nil {
			return cannot("find the project to read its prompts setting: %v (or pass --module)", err)
		}
		c, err := config.Load(root, config.Flags{})
		if err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
		if c.Prompts == "" && c.RequirePrompts {
			return cannot("%s", c.StockRefusal())
		}
		if modulePath, src, err = c.Module(); err != nil {
			return cannot("%v", err)
		}
		if c.Prompts == "" {
			stockRoot = root
		}
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

	var tracked map[string]bool
	if stockRoot != "" {
		if tracked, err = trackedPaths(stockRoot); err != nil {
			return cannot("%v", err)
		}
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
			r.Failures = append(r.Failures, containment(c.Seat, c.Context, out)...)
			if tracked != nil {
				r.Failures = append(r.Failures, stockPaths(c.Context, out, tracked)...)
				r.Failures = append(r.Failures, stockNames(out)...)
			}
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
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the JSON value")
	}
	return nil
}

// containment checks that out names the context's page and carries its findings text. The
// blueprint seat is the exception: its prompt must not name the page.
func containment(seat string, c prompts.Context, out string) []promptFailure {
	var fs []promptFailure
	var page struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(c.Page, &page); err != nil || page.Path == "" {
		fs = append(fs, promptFailure{"page", "the context's page.path is missing"})
	} else if seat == "blueprint" {
		// The blueprint seat's request inlines every repository path its prompt names, and the
		// page is not yet written, so naming the page's path would pull in the wrong file.
		if strings.Contains(out, page.Path) {
			fs = append(fs, promptFailure{"page", fmt.Sprintf("the blueprint prompt names page.path %q, which the request would inline", page.Path)})
		}
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

// stockDenylist holds the strings, matched without regard to case, that the stock module's
// renderings must not contain: each belongs to one project's tooling, not to a prompt every
// project receives.
var stockDenylist = []string{
	"cairn", "docs/internal", "docs-register", "sveltekit", "cloudflare", "wrangler", "glw907",
	"npm run check:", "option-map",
}

// stockNames fails once for each denylisted string that out holds.
func stockNames(out string) []promptFailure {
	var fs []promptFailure
	lower := strings.ToLower(out)
	for _, s := range stockDenylist {
		if strings.Contains(lower, s) {
			fs = append(fs, promptFailure{"names", fmt.Sprintf("prompt contains %q, which the stock module must not name", s)})
		}
	}
	return fs
}

// trackedPaths lists the files git tracks under root, and their directories, by slash path.
func trackedPaths(root string) (map[string]bool, error) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("list the files git tracks: %w", err)
	}
	set := map[string]bool{}
	for p := range strings.SplitSeq(string(out), "\x00") {
		for p != "" && p != "." {
			set[p] = true
			p = path.Dir(p)
		}
	}
	return set, nil
}

// stockPaths fails each path-shaped token of out that does not resolve. The tokens split as
// assemble splits them. The context's page.path and page.brief are the run's own outputs, so
// they are exempt.
func stockPaths(c prompts.Context, out string, tracked map[string]bool) []promptFailure {
	var page struct {
		Path  string `json:"path"`
		Brief string `json:"brief"`
	}
	_ = json.Unmarshal(c.Page, &page)
	var fs []promptFailure
	seen := map[string]bool{}
	for tok := range strings.FieldsFuncSeq(out, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || strings.ContainsRune("`'\"()[]<>,", r)
	}) {
		tok = strings.TrimRight(tok, ".,;:")
		tok, _, _ = strings.Cut(tok, "#")
		if seen[tok] {
			continue
		}
		seen[tok] = true
		name, isStock := strings.CutPrefix(tok, "drafter:")
		switch {
		case isStock:
			if _, ok := stock.Files(name); !ok {
				fs = append(fs, promptFailure{"path", fmt.Sprintf("prompt names %q, which is not a stock file", tok)})
			}
		case strings.Contains(tok, "/") && !strings.Contains(tok, "://"):
			if tok == page.Path || tok == page.Brief {
				continue
			}
			if !tracked[strings.TrimSuffix(tok, "/")] {
				fs = append(fs, promptFailure{"path", fmt.Sprintf("prompt names %q, which the project does not track", tok)})
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

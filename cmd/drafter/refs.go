package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/spf13/cobra"
)

func newRefsCmd() *cobra.Command {
	var f checkFlags
	cmd := &cobra.Command{
		Use:   "refs [flags] <fact-id>...",
		Short: "List every blueprint item and brief sentence that places or cites a fact",
		Long: `List each place in the project's blueprints that places or cites each fact: fact items, facts
lists, the introduction's facts, dispositions, and framing citations. Each prints as
file:line: item-id (kind), followed by "in <section-id>" for the section a correction redrafts. A
framing citation has the item id _framing-<part> and the kind example. Each brief sentence that
cites the fact follows, as brief-file:line: s<n>: sentence [page], where the page is the one the
brief belongs to. With more than one id, the lines are grouped under each id.

An id matches with or without its f: prefix. The exit status with one id is 0 when there is at
least one place, 1 when there is none, and 2 when the command could not run. With several ids it is
1 only when a named id is not in the facts store; an id the store holds that nothing cites is
reported on standard error and does not fail the run.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usageError(cmd, "refs needs at least one fact id")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error { return runRefs(cmd, args, &f) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.root, "root", "", "project root (default: the git top level of the working directory)")
	cmd.Flags().BoolVar(&f.json, "json", false, "print the references as one JSON array")
	return cmd
}

// refRow is one reference in the JSON output. Brief, Page, and Sentence are set for a brief hit.
type refRow struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Item     string `json:"item"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Section  string `json:"section,omitempty"`
	Brief    string `json:"brief,omitempty"`
	Page     string `json:"page,omitempty"`
	Sentence string `json:"sentence,omitempty"`
}

func runRefs(cmd *cobra.Command, ids []string, f *checkFlags) error {
	p, err := projectFor(f.root)
	if err != nil {
		return err
	}
	grouped := len(ids) > 1
	var rows []refRow
	var text strings.Builder
	var unknown, unplaced []string
	total := 0
	for _, id := range ids {
		hits, held, err := p.RefHits(id)
		if err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
		total += len(hits)
		if grouped && len(hits) > 0 {
			text.WriteString(id + "\n")
		}
		for _, h := range hits {
			rows = append(rows, refRow{h.File, h.Line, h.Item, h.Kind, id, h.Section, h.Brief, h.Page, h.Sentence})
			text.WriteString(refLine(h, grouped))
		}
		switch {
		case len(hits) > 0:
		case held:
			unplaced = append(unplaced, id)
		case p.FactsDir == "":
			unknown = append(unknown, fmt.Sprintf("no blueprint places or cites %s; the project has no facts store, so the id is not checked and framing citations are not read", id))
		default:
			unknown = append(unknown, fmt.Sprintf("fact %s is not in the facts store, and no blueprint places or cites it", id))
		}
	}
	out := cmd.OutOrStdout()
	if f.json {
		if err := writeRefsJSON(out, rows); err != nil {
			return err
		}
	} else if _, err := io.WriteString(out, text.String()); err != nil {
		return err
	}
	if !grouped {
		switch {
		case total > 0:
			return nil
		case len(unplaced) > 0:
			return exitError{code: exitFindings, msg: fmt.Sprintf("fact %s is in the facts store, but no blueprint places or cites it", ids[0])}
		}
		return exitError{code: exitFindings, msg: unknown[0]}
	}
	for _, id := range unplaced {
		msg := fmt.Sprintf("fact %s is in the facts store, but no blueprint places or cites it\n", id)
		if _, err := io.WriteString(cmd.ErrOrStderr(), msg); err != nil {
			return err
		}
	}
	if len(unknown) > 0 {
		return exitError{code: exitFindings, msg: strings.Join(unknown, "\n")}
	}
	return nil
}

// refLine is the text line for one hit, indented under its id when the run names several.
func refLine(h blueprint.Hit, grouped bool) string {
	var b strings.Builder
	if grouped {
		b.WriteString("  ")
	}
	if h.Kind == "brief" {
		b.WriteString(h.Brief)
		if h.Line > 0 {
			fmt.Fprintf(&b, ":%d", h.Line)
		}
		fmt.Fprintf(&b, ": %s: %s [%s]\n", h.Item, h.Sentence, h.Page)
		return b.String()
	}
	fmt.Fprintf(&b, "%s:%d: %s (%s)", h.File, h.Line, h.Item, h.Kind)
	if h.Section != "" {
		b.WriteString(" in " + h.Section)
	}
	b.WriteString("\n")
	return b.String()
}

func writeRefsJSON(w io.Writer, rows []refRow) error {
	if rows == nil {
		rows = []refRow{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

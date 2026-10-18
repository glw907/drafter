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
		Use:   "refs [flags] <fact-id>",
		Short: "List every blueprint item that places or cites a fact",
		Long: `List each place in the project's blueprints that places or cites the fact: fact items, facts
lists, the introduction's facts, dispositions, and framing citations. Each prints as
file:line: item-id (kind). A framing citation has the item id _framing-<part> and the kind
example. The exit status is 0 when there is at least one, 1 when there is none, and 2 when
the command could not run.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(cmd, "refs needs exactly one fact id")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error { return runRefs(cmd, args[0], &f) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.root, "root", "", "project root (default: the git top level of the working directory)")
	cmd.Flags().BoolVar(&f.json, "json", false, "print the references as one JSON array")
	return cmd
}

// refRow is one reference in the JSON output.
type refRow struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Item string `json:"item"`
	Kind string `json:"kind"`
}

func runRefs(cmd *cobra.Command, id string, f *checkFlags) error {
	p, err := projectFor(f.root)
	if err != nil {
		return err
	}
	refs, held, err := p.Refs(id)
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	out := cmd.OutOrStdout()
	if f.json {
		if err := writeRefsJSON(out, refs); err != nil {
			return err
		}
	} else {
		var b strings.Builder
		for _, r := range refs {
			fmt.Fprintf(&b, "%s:%d: %s (%s)\n", r.File, r.Line, r.Item, r.Kind)
		}
		if _, err := io.WriteString(out, b.String()); err != nil {
			return err
		}
	}
	if len(refs) > 0 {
		return nil
	}
	switch {
	case held:
		return exitError{code: exitFindings, msg: fmt.Sprintf("fact %s is in the facts store, but no blueprint places or cites it", id)}
	case p.FactsDir == "":
		return exitError{code: exitFindings, msg: fmt.Sprintf("no blueprint places or cites %s; the project has no facts store, so the id is not checked and framing citations are not read", id)}
	}
	return exitError{code: exitFindings, msg: fmt.Sprintf("fact %s is not in the facts store, and no blueprint places or cites it", id)}
}

func writeRefsJSON(w io.Writer, refs []blueprint.Ref) error {
	rows := make([]refRow, 0, len(refs))
	for _, r := range refs {
		rows = append(rows, refRow(r))
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

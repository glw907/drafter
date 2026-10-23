package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/glw907/drafter/internal/assemble"
	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/plan"
	"github.com/spf13/cobra"
)

func newExcerptCmd() *cobra.Command {
	var f checkFlags
	cmd := &cobra.Command{
		Use:   "excerpt [flags] <fact-id | pointer>",
		Short: "Print the source excerpt a read sends for a fact or a pointer",
		Long: `Print what a read sees of a fact's sources. For a fact id, such as f:abc123, it prints the
fact's bullet, then each excerpt block exactly as a read's request carries it. For a source
pointer, such as src/widget.ts#Panel.open or docs/owner.md:12-20, it prints that pointer's
block. A block that reads "no excerpt:" gives the reason a read sees nothing there. The exit
status is 0 when at least one block shows source lines, 1 when none does, and 2 when the command
could not run.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(cmd, "excerpt needs exactly one fact id or pointer")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error { return runExcerpt(cmd, args[0], &f) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.root, "root", "", "project root (default: the git top level of the working directory)")
	cmd.Flags().BoolVar(&f.json, "json", false, "print the bullet and blocks as one JSON object")
	return cmd
}

// excerptResult is the JSON output: the fact's id and bullet when the argument was a fact id.
type excerptResult struct {
	ID      string   `json:"id,omitempty"`
	Raw     string   `json:"raw,omitempty"`
	Blocks  []string `json:"blocks"`
	Excerpt bool     `json:"excerpt"`
}

// excerpterFor opens the project's excerpt renderer. A project with no drafter.toml still has
// one, rooted where its blueprints are.
func excerpterFor(ctx context.Context, p blueprint.Project) (*assemble.Excerpter, error) {
	cfg, err := config.Load(p.Root, config.Flags{})
	if err != nil {
		return nil, err
	}
	cfg.Root = p.Root
	return assemble.NewExcerpter(ctx, cfg)
}

func runExcerpt(cmd *cobra.Command, arg string, f *checkFlags) error {
	p, err := projectFor(f.root)
	if err != nil {
		return err
	}
	ex, err := excerpterFor(cmd.Context(), p)
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	var facts *plan.Facts
	if p.FactsDir != "" {
		if facts, err = plan.LoadFacts(p.FactsDir); err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
	}
	var res excerptResult
	commentary := 0
	switch {
	case facts != nil && facts.Count(arg) > 0:
		fact, ok := facts.Get(arg)
		if !ok {
			return exitError{code: exitCannotRun, msg: fmt.Sprintf("fact %s is held by more than one bullet", arg)}
		}
		res.ID, res.Raw, commentary = arg, fact.Raw, fact.Commentary
		if res.Blocks, res.Excerpt, err = ex.Fact(facts, arg); err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
	case !strings.ContainsAny(arg, "/#.:") || strings.HasPrefix(arg, "f:"):
		return exitError{code: exitCannotRun, msg: fmt.Sprintf("%s is not a fact in the facts store, and does not look like a source pointer", arg)}
	default:
		res.Blocks, res.Excerpt = ex.Pointer(arg)
	}
	if err := writeExcerpt(cmd.OutOrStdout(), &res, f.json); err != nil {
		return err
	}
	if res.Excerpt {
		return nil
	}
	return exitError{code: exitFindings, msg: "no block shows source lines" + commentaryNote(commentary) + "; point the fact at the code that shows the claim"}
}

// commentaryNote says how many pointers of a fact's Source: part were read as commentary because
// they sit in parentheses, which is the usual reason a fact that names code still shows none.
func commentaryNote(n int) string {
	switch n {
	case 0:
		return ""
	case 1:
		return " (1 pointer in parentheses was read as commentary)"
	}
	return fmt.Sprintf(" (%d pointers in parentheses were read as commentary)", n)
}

func writeExcerpt(w io.Writer, res *excerptResult, asJSON bool) error {
	if asJSON {
		if res.Blocks == nil {
			res.Blocks = []string{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	var parts []string
	if res.Raw != "" {
		parts = append(parts, res.Raw)
	}
	parts = append(parts, res.Blocks...)
	if len(parts) == 0 {
		return nil
	}
	_, err := io.WriteString(w, strings.Join(parts, "\n\n")+"\n")
	return err
}

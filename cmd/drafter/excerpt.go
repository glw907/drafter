package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/glw907/drafter/internal/assemble"
	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/plan"
	"github.com/spf13/cobra"
)

func newExcerptCmd() *cobra.Command {
	var f checkFlags
	var full bool
	cmd := &cobra.Command{
		Use:   "excerpt [flags] <fact-id | pointer>",
		Short: "Print the source excerpt a read sends for a fact or a pointer",
		Long: `Print what a read sees of a fact's sources. For a fact id, such as f:abc123, it prints the
fact's bullet, then each excerpt block exactly as a read's request carries it. For a source
pointer, such as src/widget.ts#Panel.open or docs/owner.md:12-20, it prints that pointer's
block. A block that reads "no excerpt:" gives the reason a read sees nothing there. A fact whose
sources are URLs prints each URL with its read date as a checklist to verify by hand. Output past
200 lines stops there and names the rest, unless --full is set. The exit status is 0 when at least
one block shows source lines or the fact's sources are URLs, 1 when none does, and 2 when the
command could not run.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(cmd, "excerpt needs exactly one fact id or pointer")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error { return runExcerpt(cmd, args[0], &f, full) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.root, "root", "", "project root (default: the git top level of the working directory)")
	cmd.Flags().BoolVar(&f.json, "json", false, "print the bullet and blocks as one JSON object")
	cmd.Flags().BoolVar(&full, "full", false, "print every line, not the first 200")
	return cmd
}

// excerptResult is the JSON output: the fact's id and bullet when the argument was a fact id.
type excerptResult struct {
	ID      string   `json:"id,omitempty"`
	Raw     string   `json:"raw,omitempty"`
	Blocks  []string `json:"blocks"`
	Excerpt bool     `json:"excerpt"`
	// Checklist holds one line per URL source of a fact that has no file source to show.
	Checklist []string `json:"checklist,omitempty"`
}

// maxExcerptLines is the longest text output without --full.
const maxExcerptLines = 200

var (
	urlRe      = regexp.MustCompile("https?://[^\\s`)\\]>]+")
	readDateRe = regexp.MustCompile(`read (\d{4}-\d{2}-\d{2})`)
)

// urlChecklist lists a bullet's URL sources as checklist lines, each with the date the bullet says
// it was read, or a note that it names none.
func urlChecklist(raw string) []string {
	var out []string
	locs := urlRe.FindAllStringIndex(raw, -1)
	for i, loc := range locs {
		end := len(raw)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		read := "no read date"
		if m := readDateRe.FindStringSubmatch(raw[loc[1]:end]); m != nil {
			read = "read " + m[1]
		}
		out = append(out, "- [ ] "+strings.TrimRight(raw[loc[0]:loc[1]], ".,;")+" ("+read+")")
	}
	return out
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

func runExcerpt(cmd *cobra.Command, arg string, f *checkFlags, full bool) error {
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
	if !res.Excerpt {
		res.Checklist = urlChecklist(res.Raw)
	}
	if err := writeExcerpt(cmd.OutOrStdout(), &res, f.json, full); err != nil {
		return err
	}
	if res.Excerpt || len(res.Checklist) > 0 {
		return nil
	}
	advice := "; point the fact at the code that shows the claim"
	if slices.ContainsFunc(res.Blocks, func(b string) bool { return strings.Contains(b, "no excerpt: prose") }) {
		advice = "; a prose source gives no excerpt, so verify the claim by hand, or drop it where the project's rules exclude prose sources"
	}
	return exitError{code: exitFindings, msg: "no block shows source lines" + commentaryNote(commentary) + advice}
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

func writeExcerpt(w io.Writer, res *excerptResult, asJSON, full bool) error {
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
	if len(res.Checklist) > 0 {
		parts = append(parts, strings.Join(res.Checklist, "\n"))
	}
	if len(parts) == 0 {
		return nil
	}
	text := strings.Join(parts, "\n\n")
	if lines := strings.Split(text, "\n"); !full && len(lines) > maxExcerptLines {
		text = strings.Join(lines[:maxExcerptLines], "\n") + fmt.Sprintf("\n... (%d more lines; --full prints them)", len(lines)-maxExcerptLines)
	}
	_, err := io.WriteString(w, text+"\n")
	return err
}

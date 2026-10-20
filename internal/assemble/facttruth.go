package assemble

import (
	"fmt"
	"strings"

	"github.com/glw907/drafter/internal/plan"
)

// The verdicts of the fact-truth read, one per fact.
const (
	VerdictHolds       = "holds"
	VerdictWider       = "wider"
	VerdictContradicts = "contradicts"
	VerdictNoExcerpt   = "no-excerpt"
)

// FactVerdicts lists the fact-truth verdicts in the order the schema's enum gives them.
var FactVerdicts = []string{VerdictHolds, VerdictWider, VerdictContradicts, VerdictNoExcerpt}

// FactTruthPrompt builds the fact-truth read's prompt for the page at path: each fact's whole
// bullet, tag included, and the rules for judging it against the source excerpts the request
// carries after the prompt. The prompt never asks for an edit; the read states the claim the
// code supports and leaves the bullet to its author.
func FactTruthPrompt(path string, facts []plan.Fact) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Check the facts a documentation page blueprint relies on against the source. The page is %s.\n", path)
	b.WriteString("Each fact below is one bullet of the project's facts store, as written: its id, its claim, its source pointers, and its status tag. ")
	b.WriteString("The source excerpts that follow quote the lines its pointers name. Judge each fact against its excerpts alone, and give each fact exactly one verdict:\n\n")
	b.WriteString("- holds: the excerpts show everything the bullet claims.\n")
	b.WriteString("- wider: the bullet claims more than the excerpts show. Some part of the claim, such as a count, a list, \"only\", \"every\", \"always\", or a scope, is not shown by any excerpt.\n")
	b.WriteString("- contradicts: an excerpt shows the claim is false. A bullet that presents a list or set of cases as complete is contradicts when an excerpt shows a case it leaves out.\n")
	b.WriteString("- no-excerpt: no excerpt bears on the claim.\n\n")
	b.WriteString("When a code line and a comment disagree, the code line outranks the comment. A prose excerpt shows what a document says, never what the code does. ")
	b.WriteString("For every verdict but holds, quote the excerpt lines the verdict rests on, and state the fix as the claim the code supports. Never ask for an edit to the bullet or the code.\n\nFacts:\n")
	for _, f := range facts {
		fmt.Fprintf(&b, "\n- %s", f.Raw)
	}
	return b.String()
}

func factTruthSchema() map[string]any {
	item := obj(map[string]any{
		"id":      str("The fact's id, as listed."),
		"verdict": map[string]any{"type": "string", "enum": FactVerdicts, "description": "The fact's verdict."},
		"excerpt": str("The excerpt lines a verdict other than holds rests on; empty for holds."),
		"fix":     str("The claim the code supports; empty for holds."),
	})
	return obj(map[string]any{
		"facts": map[string]any{"type": "array", "items": item, "description": "One entry for each listed fact."},
	})
}

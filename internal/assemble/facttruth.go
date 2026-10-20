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
// carries before the prompt. The prompt never asks for an edit; the read states the claim the
// code supports and leaves the bullet to its author.
func FactTruthPrompt(path string, facts []plan.Fact) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Check the facts a documentation page blueprint relies on against the source excerpts above. The page is %s.\n", path)
	b.WriteString("Each fact below is one bullet of the project's facts store, as written: its id, its claim, its source pointers, and its status tag. ")
	b.WriteString("Each excerpt above names the fact it was cut for. Judge each fact against the excerpts alone.\n\n")
	b.WriteString("Work through each fact the same way. Split its claim into parts: each clause, each item of a list, each count, and each qualifier, such as \"only\", \"every\", \"always\", \"never\", \"by default\", or a scope. ")
	b.WriteString("Find the excerpt line that shows each part, then give the fact exactly one verdict:\n\n")
	b.WriteString("- holds: an excerpt line shows every part of the claim, and no excerpt line departs from any part.\n")
	b.WriteString("- wider: some part of the claim is shown by no excerpt line.\n")
	b.WriteString("- contradicts: an excerpt line shows a part of the claim is false. A general claim, such as what a command exits with or what every write does, is contradicts when an excerpt shows a path that departs from it. ")
	b.WriteString("A bullet that names a set, such as the entries of a list, the bindings, the stages, or the cases that refuse or throw, presents it as complete unless it says otherwise (\"for example\", \"among them\"); it is contradicts when an excerpt shows a member it leaves out.\n")
	b.WriteString("- no-excerpt: no excerpt bears on the claim.\n\n")
	b.WriteString("Give holds only for a claim whose every part you can point to. ")
	b.WriteString("When a code line and a comment disagree, the code line outranks the comment. A prose excerpt shows what a document says, never what the code does, and so does a string in the code, such as a message or a description: it shows what the program says, not what it does. ")
	b.WriteString("For every verdict but holds, quote the excerpt lines the verdict rests on, and state the fix as the claim the code supports. Never ask for an edit to the bullet or the code.\n\n")
	fmt.Fprintf(&b, "There are %d facts. The answer has %d entries, one for each id below, in the order listed, each id copied exactly.\n\nFacts:\n", len(facts), len(facts))
	for _, f := range facts {
		fmt.Fprintf(&b, "\n- %s", f.Raw)
	}
	return b.String()
}

// sharedBlocks keeps the first block of each excerpt body whole and replaces each later block
// whose body is the same with one line naming that first block. Facts that cite one declaration
// or one small file would otherwise carry its lines once each, which doubles the request on a
// large blueprint. A "no excerpt:" body is short and kept as it is.
func sharedBlocks(blocks []string) []string {
	first := map[string]string{}
	out := make([]string, len(blocks))
	for i, b := range blocks {
		head, rest, _ := strings.Cut(b, "\n")
		body := rest[:strings.LastIndex(rest, "\n")+1]
		label, seen := first[body]
		switch {
		case strings.HasPrefix(body, "no excerpt:"):
			out[i] = b
		case seen:
			out[i] = head + "\nThe same lines as the block above labelled \"" + label + "\".\n" + rest[len(body):]
		default:
			_, l, _ := strings.Cut(head, ": ")
			first[body] = strings.TrimSuffix(l, " ===")
			out[i] = b
		}
	}
	return out
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

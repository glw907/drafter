package assemble

import (
	"fmt"
	"strings"

	"github.com/glw907/drafter/internal/plan"
)

// BlueprintBlock builds the block the blueprint read appends to its prompt: the blueprint's
// bytes, the page's outline entry, and the bullet of each fact the blueprint places or cites. The
// facts are those placedFacts finds on view, the blueprint's projection onto a plan, so a fact
// the blueprint leaves out is not sent. facts may be nil, which lists ids alone.
//
// The block states its own delimiters and none of them is a "=== file:" line, so the request's
// file inlining never reads it as a path.
func BlueprintBlock(view *plan.Plan, blueprint, entry []byte, facts *plan.Facts) string {
	var b strings.Builder
	b.WriteString("The blueprint, its outline entry, and the facts it places or cites follow. They are everything you read.\n\n")
	fmt.Fprintf(&b, "=== blueprint ===\n%s\n=== end blueprint ===\n\n", strings.TrimRight(string(blueprint), "\n"))
	fmt.Fprintf(&b, "=== outline entry ===\n%s\n=== end outline entry ===\n\n", strings.TrimRight(string(entry), "\n"))
	b.WriteString("=== facts placed or cited ===\n")
	ids := placedFacts(view)
	for _, id := range ids {
		fmt.Fprintf(&b, "- %s", id)
		switch f, ok := lookup(facts, id); {
		case facts == nil:
		case ok:
			fmt.Fprintf(&b, " [%s]: %s", f.Tag, strings.Join(strings.Fields(f.Text), " "))
		default:
			b.WriteString(": (not held by exactly one bullet of the facts store)")
		}
		b.WriteByte('\n')
	}
	if len(ids) == 0 {
		b.WriteString("(none)\n")
	}
	b.WriteString("=== end facts ===")
	return b.String()
}

func lookup(facts *plan.Facts, id string) (plan.Fact, bool) {
	if facts == nil {
		return plan.Fact{}, false
	}
	return facts.Get(id)
}

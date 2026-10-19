package assemble

import (
	"fmt"
	"strings"

	"github.com/glw907/drafter/internal/plan"
)

// FactsBlock builds the block the blueprint read sends second: the bullet of each fact the
// blueprint places or cites. The facts are those placedFacts finds on view, the blueprint's
// projection onto a plan, so a fact the blueprint leaves out is not sent. facts may be nil, which
// lists ids alone. The block opens by naming what follows it, because it precedes the blueprint.
//
// The block states its own delimiters and none of them is a "=== file:" line, so the request's
// file inlining never reads it as a path.
func FactsBlock(view *plan.Plan, facts *plan.Facts) string {
	var b strings.Builder
	b.WriteString("The facts the blueprint places or cites, then the blueprint and its outline entry, follow. They are everything you read, apart from the change block when one follows them.\n\n")
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

// BlueprintBlock builds the block the blueprint read sends third, ahead of any change block: the
// blueprint's bytes and the page's outline entry. Its delimiters, like FactsBlock's, are never a
// "=== file:" line.
func BlueprintBlock(blueprint, entry []byte) string {
	return fmt.Sprintf("=== blueprint ===\n%s\n=== end blueprint ===\n\n=== outline entry ===\n%s\n=== end outline entry ===",
		strings.TrimRight(string(blueprint), "\n"), strings.TrimRight(string(entry), "\n"))
}

func lookup(facts *plan.Facts, id string) (plan.Fact, bool) {
	if facts == nil {
		return plan.Fact{}, false
	}
	return facts.Get(id)
}

// FactChange is one fact whose bullet differs between the two revisions of a scoped read. A nil
// side means the store holds no single bullet for the id at that revision.
type FactChange struct {
	ID       string
	Old, New *plan.Fact
}

// ChangeBlock builds the block a scoped blueprint read appends after BlueprintBlock: the
// blueprint's diff, the old and new text of each changed fact bullet, and the outline entry before
// and after when it changed. A part with nothing in it is left out. The block states its own
// delimiters, none a "=== file:" line. A diff line starts with a space, "+", "-", "@", "\\", or a
// header word, so the request's file inlining never reads one as a path.
func ChangeBlock(diff string, facts []FactChange, oldEntry, newEntry []byte) string {
	var b strings.Builder
	b.WriteString("The change this read judges follows. It adds to the blueprint and the facts above, and is the diff of the blueprint, the old and new text of each fact bullet that changed, and the outline entry before and after when it changed.")
	if diff = strings.TrimRight(diff, "\n"); diff != "" {
		fmt.Fprintf(&b, "\n\n=== blueprint diff ===\n%s\n=== end blueprint diff ===", diff)
	}
	if len(facts) > 0 {
		b.WriteString("\n\n=== changed facts ===\n")
		for _, f := range facts {
			fmt.Fprintf(&b, "- %s\n  was: %s\n  now: %s\n", f.ID, factLine(f.Old), factLine(f.New))
		}
		b.WriteString("=== end changed facts ===")
	}
	if len(oldEntry) > 0 || len(newEntry) > 0 {
		fmt.Fprintf(&b, "\n\n=== outline entry change ===\nbefore:\n%s\nafter:\n%s\n=== end outline entry change ===",
			strings.TrimRight(string(oldEntry), "\n"), strings.TrimRight(string(newEntry), "\n"))
	}
	return b.String()
}

func factLine(f *plan.Fact) string {
	if f == nil {
		return "(not held by exactly one bullet of the facts store)"
	}
	return fmt.Sprintf("[%s] %s", f.Tag, strings.Join(strings.Fields(f.Text), " "))
}

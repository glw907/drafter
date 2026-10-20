package assemble

import (
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

func TestFactTruthPrompt(t *testing.T) {
	facts := plan.ParseFacts(map[string]string{"a.md": strings.Join([]string{
		"- `f:one` The first claim.  Source: `src/greet.go#Hello`. [verified: 2026-10-01]",
		"- `f:two` The second claim. [external: the vendor guide]",
	}, "\n")}, t.TempDir())
	var fs []plan.Fact
	for _, id := range []string{"f:one", "f:two"} {
		f, ok := facts.Get(id)
		if !ok {
			t.Fatalf("Get(%s) = false", id)
		}
		fs = append(fs, f)
	}
	got := FactTruthPrompt("docs/guide.md", fs)
	for _, want := range []string{
		"The page is docs/guide.md.",
		"- `f:one` The first claim. Source: `src/greet.go#Hello`. [verified: 2026-10-01]",
		"- `f:two` The second claim. [external: the vendor guide]",
		"- holds:", "- wider:", "- contradicts:", "- no-excerpt:",
		`"only", "every", "always"`,
		"presents a list or set of cases as complete is contradicts",
		"the code line outranks the comment",
		"never what the code does",
		"quote the excerpt lines",
		"Never ask for an edit",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt lacks %q:\n%s", want, got)
		}
	}
}

func TestSharedBlocks(t *testing.T) {
	blocks := []string{
		block("excerpt", "a.ts#run (fact f:a) lines 1-3", "one\ntwo\nthree"),
		block("excerpt", "b.ts:4 (fact f:a) lines 1-2", "other"),
		block("excerpt", "a.ts#run (fact f:b) lines 1-3", "one\ntwo\nthree"),
		block("excerpt", "c.md (fact f:a)", "no excerpt: prose"),
		block("excerpt", "c.md (fact f:b)", "no excerpt: prose"),
	}
	got := sharedBlocks(blocks)
	want := []string{
		blocks[0],
		blocks[1],
		"=== excerpt: a.ts#run (fact f:b) lines 1-3 ===\nThe same lines as the block above labelled \"a.ts#run (fact f:a) lines 1-3\".\n=== end excerpt: a.ts#run (fact f:b) lines 1-3 ===",
		blocks[3],
		blocks[4],
	}
	if !slices.Equal(got, want) {
		t.Errorf("sharedBlocks =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestFactTruthRequestCarriesOnlyExcerpts(t *testing.T) {
	f := newFixture(t)
	c := f.config()
	c.Seat["facttruth"] = c.Seat["draft"]
	facts := loadFacts(t, f)
	// The prompt names a path, which a seat that scans its prompt would inline whole.
	in := f.input(c, "Check src/panel.ts against the facts.")
	in.Seat, in.Answer, in.Facts, in.FactIDs = "facttruth", AnswerFactTruth, facts, []string{"f:greet"}
	got := requestText(t, in)
	if !strings.Contains(got, "=== excerpt: src/greet.go (fact f:greet) lines 1-") {
		t.Errorf("request lacks the fact's excerpt:\n%s", got)
	}
	if strings.Contains(got, "=== file: src/panel.ts ===") {
		t.Errorf("request inlines a file its prompt names:\n%s", got)
	}
	if !strings.Contains(got, "one entry for each listed fact") {
		t.Errorf("request lacks the answer's contract:\n%s", got)
	}
}

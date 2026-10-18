package assemble

import (
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

func TestChangeBlock(t *testing.T) {
	old, cur := &plan.Fact{Tag: "verified", Text: "A  key lives\n90 days."}, &plan.Fact{Tag: "external", Text: "A key lives 60 days."}
	tests := []struct {
		name         string
		diff         string
		facts        []FactChange
		oldE, newE   string
		want, absent []string
	}{
		{
			name: "every part", diff: "diff --git a/x b/x\n-old\n+new\n",
			facts: []FactChange{{ID: "f:a", Old: old, New: cur}, {ID: "f:b", Old: nil, New: cur}},
			oldE:  "{\"job\": \"A\"}", newE: "{\"job\": \"B\"}",
			want: []string{
				"=== blueprint diff ===\ndiff --git a/x b/x\n-old\n+new\n=== end blueprint diff ===",
				"- f:a\n  was: [verified] A key lives 90 days.\n  now: [external] A key lives 60 days.",
				"- f:b\n  was: (not held by exactly one bullet of the facts store)",
				"=== outline entry change ===\nbefore:\n{\"job\": \"A\"}\nafter:\n{\"job\": \"B\"}\n=== end outline entry change ===",
			},
		},
		{
			name: "a diff alone leaves out the other parts", diff: "-old\n+new\n",
			want:   []string{"=== blueprint diff ==="},
			absent: []string{"=== changed facts ===", "=== outline entry change ==="},
		},
		{
			name: "an unchanged entry and no diff", facts: []FactChange{{ID: "f:a", Old: old, New: cur}},
			want:   []string{"=== changed facts ==="},
			absent: []string{"=== blueprint diff ===", "=== outline entry change ==="},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChangeBlock(tt.diff, tt.facts, []byte(tt.oldE), []byte(tt.newE))
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("the block lacks %q\n%s", w, got)
				}
			}
			for _, a := range tt.absent {
				if strings.Contains(got, a) {
					t.Errorf("the block holds %q\n%s", a, got)
				}
			}
			for line := range strings.SplitSeq(got, "\n") {
				if strings.HasPrefix(line, "=== file:") {
					t.Errorf("a line reads as a file to inline: %q", line)
				}
			}
		})
	}
}

func TestLinkedPages(t *testing.T) {
	p := &plan.Plan{
		Page: plan.Page{Path: "docs/guides/a.md"},
		Sections: []plan.Section{{Items: []plan.Item{
			{Kind: "link", Target: "../reference/keys.md#Heading"},
			{Kind: "link", Target: "../reference/keys.md"},
			{Kind: "link", Target: "https://example.com/x"},
			{Kind: "link", Target: "#own-section"},
			{Kind: "step", Target: "ignored.md"},
			{Kind: "link", Target: "b.md"},
		}}},
	}
	if got, want := LinkedPages(p), []string{"docs/reference/keys.md", "docs/guides/b.md"}; !slices.Equal(got, want) {
		t.Errorf("LinkedPages = %v, want %v", got, want)
	}
}

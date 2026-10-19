package plan

import (
	"slices"
	"testing"
)

func TestHeadings(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []Heading
	}{
		{"atx levels and lines", "# One\n\ntext\n\n## Two\n### Three\n", []Heading{{1, 1, "one"}, {2, 5, "two"}, {3, 6, "three"}}},
		{"setext heading", "Title\n=====\n\nSub\n---\n", []Heading{{1, 1, "title"}, {2, 4, "sub"}}},
		{"repeat takes a suffix", "## Same\n\n## Same\n", []Heading{{2, 1, "same"}, {2, 3, "same-1"}}},
		{"fence is not a heading", "## Real\n\n```\n## Fake\n```\n", []Heading{{2, 1, "real"}}},
		{"an empty heading keeps its own line", "# One\n\ntext\n\n##\n\nmore\n\n## Two\n", []Heading{{1, 1, "one"}, {2, 5, ""}, {2, 9, "two"}}},
		{"an empty heading after a fence", "## A\n\n```\ncode\n```\n\n###\n", []Heading{{2, 1, "a"}, {3, 7, ""}}},
		{"front matter lines count", "---\ntitle: x\n---\n## After\n", []Heading{{2, 4, "after"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Headings([]byte(tt.src))
			if !slices.Equal(got, tt.want) {
				t.Errorf("Headings = %+v, want %+v", got, tt.want)
			}
		})
	}
}

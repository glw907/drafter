package coverage

import (
	"slices"
	"testing"
)

const spansPage = "# Title\n\nIntro.\n\n## One\n\nFirst.\n\n```\n## Two\n```\n\n## Two\n\nSecond.\n## Three\nThird."

func TestSpans(t *testing.T) {
	tests := []struct {
		name     string
		headings []string
		page     string
		pre      Span
		want     []Span
	}{
		{
			"every heading found, a fenced heading ignored",
			[]string{"One", "Two", "Three"}, spansPage,
			Span{Found: true, Heading: -1, Start: 0, End: 4},
			[]Span{{true, 4, 5, 12}, {true, 12, 13, 15}, {true, 15, 16, 17}},
		},
		{
			"a missing heading has no lines and the earlier section runs on",
			[]string{"One", "Four", "Three"}, spansPage,
			Span{Found: true, Heading: -1, Start: 0, End: 4},
			[]Span{{true, 4, 5, 15}, {false, -1, 0, 0}, {true, 15, 16, 17}},
		},
		{
			"headings match in order, so one out of order is not found",
			[]string{"Three", "One"}, spansPage,
			Span{Found: true, Heading: -1, Start: 0, End: 15},
			[]Span{{true, 15, 16, 17}, {false, -1, 0, 0}},
		},
		{
			"no heading found leaves the whole page as preamble",
			[]string{"Nine"}, "a\nb",
			Span{Found: true, Heading: -1, Start: 0, End: 2},
			[]Span{{false, -1, 0, 0}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pre, got := Spans(tt.headings, []byte(tt.page))
			if pre != tt.pre {
				t.Errorf("preamble = %+v, want %+v", pre, tt.pre)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("spans = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRegionsFollowSpans(t *testing.T) {
	pre, text, found := Regions([]string{"One", "Four", "Three"}, []byte(spansPage))
	if pre != "# Title\n\nIntro.\n" {
		t.Errorf("preamble = %q", pre)
	}
	want := []string{"\nFirst.\n\n```\n## Two\n```\n\n## Two\n\nSecond.", "", "Third."}
	if !slices.Equal(text, want) {
		t.Errorf("text = %q, want %q", text, want)
	}
	if !slices.Equal(found, []bool{true, false, true}) {
		t.Errorf("found = %v", found)
	}
}

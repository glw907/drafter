package plan

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"testing"
)

// TestSluggerFixtures runs github-slugger 2.0.0's own test cases, in order, on one slugger: the
// cases include repeats that must take -1, -2, and a slug that collides with an earlier suffix.
func TestSluggerFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/github-slugger/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Name, Input, Expected string }
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 78 {
		t.Fatalf("fixtures.json holds %d cases, want 78", len(cases))
	}
	sl := newSlugger()
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			if got := sl.slug(tt.Input); got != tt.Expected {
				t.Errorf("slug(%q) = %q, want %q", tt.Input, got, tt.Expected)
			}
		})
	}
}

// The two cases where JavaScript's toLowerCase differs from Go's simple mapping.
func TestSlugOfLowercasing(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"final sigma", "ΟΔΟΣ", "οδος"},
		{"medial sigma", "ΣΑΣ ΣΑΣ", "σας-σας"},
		{"sigma before a word after punctuation", "aΣ.b", "aσb"},
		{"sigma after punctuation", "A.Σ", "aς"},
		{"lone sigma", "Σ", "σ"},
		{"dotted capital i", "İstanbul", "i̇stanbul"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := slugOf(tt.in); got != tt.want {
				t.Errorf("slugOf(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestAnchorsOf(t *testing.T) {
	tests := []struct {
		name, markdown string
		want           []string
	}{
		{"atx heading", "# Real heading\n", []string{"real-heading"}},
		{"closing hashes are not text", "## Closed ##\n", []string{"closed"}},
		{"setext headings", "Title\n=====\n\nSub heading\n---\n", []string{"sub-heading", "title"}},
		{"fenced code is ignored", "# Real\n\n```sh\n# Fake\n```\n\n~~~\n## Also fake\n~~~\n", []string{"real"}},
		{"indented code is ignored", "# Real\n\n    # Fake\n", []string{"real"}},
		{"inline code and a link", "## Use `go run` with [the docs](https://x.example/a-b)\n", []string{"use-go-run-with-the-docs"}},
		{"repeats take a suffix", "# A\n## A\n## A\n", []string{"a", "a-1", "a-2"}},
		{"a repeat can collide with a suffix", "# A\n# A\n# A 1\n# A\n", []string{"a", "a-1", "a-1-1", "a-2"}},
		{"entities and escapes resolve", "# Fish &amp; chips\\*\n", []string{"fish--chips"}},
		{"inline html drops its tags", "# A <b>bold</b> move\n", []string{"a-bold-move"}},
		{"an image contributes nothing", "# Logo ![alt text](x.png) here\n", []string{"logo--here"}},
		{"emphasis keeps its text", "# *Fast* and **loose**\n", []string{"fast-and-loose"}},
		{"an autolink keeps its URL text", "# See <https://example.com/a>\n", []string{"see-httpsexamplecoma"}},
		{"front matter is not a heading", "---\ntitle: Front\n---\n\n# Real\n", []string{"real"}},
		{"a heading written as html is no anchor", "<h2>Raw</h2>\n\n# Md\n", []string{"md"}},
		{"headings nest in quotes and lists", "> ## Quoted\n\n- ### Listed\n", []string{"listed", "quoted"}},
		{"a code span with a line break", "A `x\ny` b\n---\n", []string{"a-x-y-b"}},
		{"a soft break is a deleted newline", "foo\nbar\n---\n", []string{"foobar"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slices.Sorted(maps.Keys(anchorsOf([]byte(tt.markdown))))
			if !slices.Equal(got, tt.want) {
				t.Errorf("anchors = %q, want %q", got, tt.want)
			}
		})
	}
}

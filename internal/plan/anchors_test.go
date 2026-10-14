package plan

import (
	"slices"
	"testing"
)

// Anchors lists headings and <a id> elements in document order. An element counts only where
// the parse reads it as raw HTML, never inside a code span, a fence, or an indented block.
func TestAnchors(t *testing.T) {
	tests := []struct {
		name, markdown string
		want           []string
	}{
		{"a heading and a repeat", "# A\n\n## B\n\n## B\n", []string{"a", "b", "b-1"}},
		{"an inline element", "# A\n\nText <a id=\"x\"></a> more.\n", []string{"a", "x"}},
		{"an element in a table cell", "| K | V |\n|---|---|\n| `k` | <a id=\"k-row\"></a>Value |\n", []string{"k-row"}},
		{"an element alone on its line", "<a id=\"alone\">\n\ntext\n", []string{"alone"}},
		{"an element inside a block-level tag", "<div><a id=\"wrapped\"></a></div>\n", []string{"wrapped"}},
		{"an element in a heading", "## Title<a id=\"in-heading\"></a>\n", []string{"title", "in-heading"}},
		{"single quotes and other attributes", "<a href='y' id='q'></a> <a class=c id=bare></a>\n", []string{"q", "bare"}},
		{"in document order", "<a id=\"first\"></a>\n\n# Second\n\n<a id=\"third\"></a>\n", []string{"first", "second", "third"}},
		{"the same id twice counts once", "<a id=\"x\"></a> <a id=\"x\"></a>\n", []string{"x"}},
		{"an element in a code span", "# A\n\nWrite `<a id=\"span\"></a>` to anchor.\n", []string{"a"}},
		{"an element in a fence", "# A\n\n```html\n<a id=\"fenced\"></a>\n```\n", []string{"a"}},
		{"an element in an indented block", "# A\n\n    <a id=\"indented\"></a>\n", []string{"a"}},
		{"an element in an html comment", "# A\n\n<!--\n<a id=\"commented\">\n-->\n", []string{"a"}},
		{"a name attribute does not count", "# A\n\n<a name=\"old\"></a>\n", []string{"a"}},
		{"a data-id attribute does not count", "# A\n\n<a data-id=\"d\"></a>\n", []string{"a"}},
		{"an empty id does not count", "# A\n\n<a id=\"\"></a>\n", []string{"a"}},
		{"a raw html heading is still no anchor", "<h2 id=\"raw\">Raw</h2>\n", nil},
		{"no anchors", "Just text.\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Anchors([]byte(tt.markdown)); !slices.Equal(got, tt.want) {
				t.Errorf("anchors = %q, want %q", got, tt.want)
			}
		})
	}
}

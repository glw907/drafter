package blueprint

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDecodeMarkdownForms(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "markdown", "fix-a-widget.blueprint.md"))
	if err != nil {
		t.Fatal(err)
	}
	m, _ := DecodeMarkdown("fix-a-widget.blueprint.md", data)
	if !slices.Equal(m.ConstraintLines, []int{16}) || !slices.Equal(m.Constraints.Facts, []string{"ef34gh"}) || m.Constraints.Text != "- Name no widget model." {
		t.Errorf("constraints = lines %v, %+v; want line 16, fact ef34gh, text %q", m.ConstraintLines, m.Constraints, "- Name no widget model.")
	}
	if len(m.BareFences) != 0 {
		t.Errorf("BareFences = %v, want none: the fence has a Code: bullet above it", m.BareFences)
	}
	if got, want := m.CodeSources(), []string{"/src/widget/log.sh#sample"}; !slices.Equal(got, want) {
		t.Errorf("CodeSources = %v, want %v", got, want)
	}
	if got, want := m.FactIDs(nil), []string{"ab12cd", "ef34gh", "ij56kl"}; !slices.Equal(got, want) {
		t.Errorf("FactIDs = %v, want %v", got, want)
	}

	v := m.View(Page{Path: "docs/fix-a-widget.md", Title: "Fix a widget", Type: "symptom row"}, nil)
	code := v.Sections[0].Items[2]
	if code.ID != "read-the-widget-log.p3" || code.Kind != "code" || code.Source != "/src/widget/log.sh#sample" || code.Text != "a sample of the log's lines." || code.Lines["source"] != 26 {
		t.Errorf("code item = %+v", code)
	}
	fig := v.Sections[2].Items[1]
	if fig.ID != "see-also.p2" || fig.Kind != "figure" || fig.Source != "/docs/img/spindle.svg" {
		t.Errorf("figure item = %+v", fig)
	}
}

func TestDecodeMarkdownFenceAttachment(t *testing.T) {
	head := "---\npage: A\n---\n\n# A\n\n## One\n\n*Shows it.*\n\n"
	tests := []struct {
		name, body string
		bare       []int // lines of fences with no Code: bullet; the head is 10 lines
	}{
		{"a fence right under the bullet", "- Code: a sample.\n```sh\nx\n```\n", nil},
		{"blank lines between", "- Code: a sample.\n\n\n```sh\nx\n```\n", nil},
		{"a comment line between", "- Code: a sample.\n<!-- snippet-check-skip: reason -->\n```sh\nx\n```\n", nil},
		{"no bullet above", "```sh\nx\n```\n", []int{11}},
		{"a plain bullet above", "- A step.\n\n```sh\nx\n```\n", []int{13}},
		{"a paragraph between", "- Code: a sample.\n\nText.\n\n```sh\nx\n```\n", []int{15}},
		{"one bullet serves one fence", "- Code: a sample.\n```sh\nx\n```\n```sh\ny\n```\n", []int{15}},
		{"a bullet in another section", "- Code: a sample.\n\n## Two\n\n*Shows it.*\n\n```sh\nx\n```\n", []int{17}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, findings := DecodeMarkdown("a.blueprint.md", []byte(head+tt.body))
			if len(findings) != 0 {
				t.Fatalf("findings = %+v", findings)
			}
			if !slices.Equal(m.BareFences, tt.bare) {
				t.Errorf("BareFences = %v, want %v", m.BareFences, tt.bare)
			}
		})
	}
}

func TestDecodeMarkdownSourceTagPlacement(t *testing.T) {
	head := "---\npage: A\n---\n\n# A\n\n"
	tests := []struct {
		name, body string
		line       int
	}{
		{"on a plain bullet", "## One\n\n*Job.*\n\n- A step. :source[/x.go]\n", 11},
		{"on a paragraph line", "## One\n\n*Job.*\n\nText :source[/x.go]\n", 11},
		{"above the first heading", "- Code: early. :source[/x.go]\n\n## One\n", 7},
		{"in a heading", "## One :source[/x.go]\n", 7},
		{"in a row", "## One\n\n*Job.*\n\n:::row\n**Symptom.** s :source[/x.go]\n:::\n", 12},
		{"in the constraints block", ":::constraints\n- Name none. :source[/x.go]\n:::\n", 8},
		{"on a reader line that is not a figure", ":::reader\n- Code: x. :source[/x.go]\n:::\n", 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, findings := DecodeMarkdown("a.blueprint.md", []byte(head+tt.body))
			if len(findings) == 0 || findings[0].Line != tt.line || !strings.Contains(findings[0].Message, ":source tag is not on a Code: or Figure: bullet") {
				t.Errorf("findings = %+v, want the :source error at line %d", findings, tt.line)
			}
		})
	}

	ok := "## One\n\n*Job.*\n\n- Code: a sample. :source[/x.go#Sym] :fact[ab12cd]\n- Figure: a diagram. :source[/x.svg]\n"
	if _, findings := DecodeMarkdown("a.blueprint.md", []byte(head+ok)); len(findings) != 0 {
		t.Errorf("findings = %+v", findings)
	}
	reader := ":::reader\n- Figure: the intro diagram. :source[/x.svg] :fact[ab12cd]\n:::\n"
	m, findings := DecodeMarkdown("a.blueprint.md", []byte(head+reader))
	if len(findings) != 0 || len(m.Sections) != 0 || !slices.Equal(m.Reader.Facts, []string{"ab12cd"}) || strings.Contains(m.Reader.Text, ":source") {
		t.Errorf("a reader-block figure = %+v, sections %v, reader %+v; want no finding, no section, the reader's fact, and no tag in its text", findings, m.Sections, m.Reader)
	}
}

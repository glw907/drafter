package blueprint

import (
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

func TestDecodeMarkdownSourceBrackets(t *testing.T) {
	head := "---\npage: A\n---\n\n# A\n\n## One\n\n*Job.*\n\n"
	tests := []struct {
		name, body, source string
		warnings           int
	}{
		{"a bracketed route segment", "- Code: a route. :source[/src/routes/[token]/+page.ts#load]\n", "/src/routes/[token]/+page.ts#load", 0},
		{"a rest segment", "- Code: a route. :source[/src/routes/[...rest]/+page.ts]\n", "/src/routes/[...rest]/+page.ts", 0},
		{"a tag after a bracketed one", "- Code: a route. :source[/r/[id]/x.ts] :fact[ab12cd]\n", "/r/[id]/x.ts", 0},
		{"two tags on one bullet", "- Code: a sample. :source[/a.go] :source[/b.go]\n", "/a.go", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, findings := DecodeMarkdown("a.blueprint.md", []byte(head+tt.body))
			for _, f := range findings {
				if f.Severity != plan.SeverityWarning {
					t.Fatalf("finding = %+v, want warnings only", f)
				}
			}
			if len(findings) != tt.warnings {
				t.Errorf("findings = %+v, want %d warnings", findings, tt.warnings)
			}
			l := m.Sections[0].Lines[0]
			if l.Source != tt.source {
				t.Errorf("Source = %q, want %q", l.Source, tt.source)
			}
			if strings.Contains(l.Text, ":source") {
				t.Errorf("Text = %q, want no :source tag left in it", l.Text)
			}
		})
	}
	_, findings := DecodeMarkdown("a.blueprint.md", []byte(head+"- Code: a sample. :source[/a.go] :source[/b.go]\n"))
	if len(findings) != 1 || findings[0].Line != 11 || !strings.Contains(findings[0].Message, "/b.go") {
		t.Errorf("findings = %+v, want one warning at line 11 naming /b.go", findings)
	}
}

func TestDecodeMarkdownFigureFence(t *testing.T) {
	head := "---\npage: A\n---\n\n# A\n\n## One\n\n*Job.*\n\n"
	body := "- Figure: a repro of the flow. :source[/x.svg]\n```repro\nstep one\nstep two\n```\n"
	m, findings := DecodeMarkdown("a.blueprint.md", []byte(head+body))
	if len(findings) != 0 || len(m.BareFences) != 0 {
		t.Fatalf("findings = %+v, bare fences %v; want neither", findings, m.BareFences)
	}
	l := m.Sections[0].Lines[0]
	if l.Kind != "figure" || l.Fence != "step one\nstep two" {
		t.Errorf("line = %+v, want a figure carrying the fence body", l)
	}
	v := m.View(Page{Path: "docs/a.md", Title: "A", Type: "t"}, nil)
	it := v.Sections[0].Items[0]
	if it.Kind != "figure" || it.Source != "/x.svg" || it.Text != "a repro of the flow.\nstep one\nstep two" {
		t.Errorf("figure item = %+v, want the fence appended to its text", it)
	}

	m, _ = DecodeMarkdown("a.blueprint.md", []byte(head+"- A plain bullet.\n```sh\nx\n```\n"))
	if len(m.BareFences) != 1 {
		t.Errorf("BareFences = %v, want the fence under a plain bullet still bare", m.BareFences)
	}
}

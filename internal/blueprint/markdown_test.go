package blueprint

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDecodeMarkdown(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "markdown", "fix-a-widget.blueprint.md"))
	if err != nil {
		t.Fatal(err)
	}
	m, findings := DecodeMarkdown("fix-a-widget.blueprint.md", data)
	if m == nil || len(findings) != 0 {
		t.Fatalf("DecodeMarkdown = %v, %v; want a blueprint and no findings", m, findings)
	}
	want := Front{Page: "Fix a widget", Type: "symptom row", Track: "operate", Exemplar: "guides/widget-troubleshooting", Line: 1}
	if m.Front != want {
		t.Errorf("Front = %+v, want %+v", m.Front, want)
	}
	if m.Text != string(data) {
		t.Error("Text is not the file as written")
	}

	tests := []struct {
		id, heading string
		level, line int
		job         string
		lines       []MDLine
	}{
		{"read-the-widget-log", "Read the widget log", 2, 16, "Shows the reader where the widget log lives.", []MDLine{
			{Line: 20, Text: "Every widget writes one log line per spin.", Facts: []string{"ab12cd"}},
			{Line: 21, Text: "The log rotates daily, so yesterday's spins sit in a dated file.", Facts: []string{"ef34gh", "ab12cd"}},
		}},
		{"widgetstalled", "`widget.stalled`", 3, 28, "Helps the reader whose widget stopped.", []MDLine{
			{Line: 32, Text: "Restart the widget with `widgetctl restart`.", Facts: []string{"ij56kl"}},
		}},
		{"see-also", "See also", 2, 40, "", []MDLine{
			{Line: 42, Text: "See [Widget reference](../reference/widget.md)."},
		}},
	}
	if len(m.Sections) != len(tests) {
		t.Fatalf("got %d sections, want %d: %+v", len(m.Sections), len(tests), m.Sections)
	}
	for i, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			s := m.Sections[i]
			if s.ID != tt.id || s.Heading != tt.heading || s.Level != tt.level || s.Line != tt.line || s.Job != tt.job {
				t.Errorf("section = %q %q level %d line %d job %q, want %q %q level %d line %d job %q",
					s.ID, s.Heading, s.Level, s.Line, s.Job, tt.id, tt.heading, tt.level, tt.line, tt.job)
			}
			if !slices.EqualFunc(s.Lines, tt.lines, func(a, b MDLine) bool {
				return a.Line == b.Line && a.Text == b.Text && slices.Equal(a.Facts, b.Facts)
			}) {
				t.Errorf("Lines = %+v, want %+v", s.Lines, tt.lines)
			}
		})
	}
}

func TestDecodeMarkdownFrontMatter(t *testing.T) {
	tests := []struct {
		name, data, want string
	}{
		{"none", "# Title\n", "has no front matter"},
		{"unclosed", "---\npage: A\n", "no closing ---"},
		{"unknown key", "---\npage: A\nkind: x\n---\n", "does not decode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, findings := DecodeMarkdown("x.blueprint.md", []byte(tt.data))
			if m != nil || len(findings) != 1 || !strings.Contains(findings[0].Message, tt.want) {
				t.Errorf("DecodeMarkdown = %v, %+v; want nil and one finding containing %q", m, findings, tt.want)
			}
		})
	}
}

func TestLoadPicksTheFormatBySuffix(t *testing.T) {
	md, _ := Load("a.blueprint.md", []byte("---\npage: A\n---\n\n## One\n"))
	if _, ok := md.(*Markdown); !ok {
		t.Errorf("Load(.blueprint.md) = %T, want *Markdown", md)
	}
	if got := md.SectionHeads(); !slices.Equal(got, []SectionHead{{ID: "one", Heading: "One"}}) {
		t.Errorf("SectionHeads = %+v", got)
	}
	if src, _ := Load("a.blueprint.md", []byte("no front matter")); src != nil {
		t.Errorf("Load of an undecodable file = %v, want a nil Source", src)
	}
	if src, _ := Load("a.blueprint.yaml", []byte("- not a mapping")); src != nil {
		t.Errorf("Load of an undecodable YAML file = %v, want a nil Source", src)
	}
}

func TestPagePathTakesBothSuffixes(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"blueprints/docs/a.blueprint.yaml", "docs/a.md", true},
		{"blueprints/docs/a.blueprint.md", "docs/a.md", true},
		{"blueprints/docs/a.md", "", false},
		{"blueprints/.blueprint.md", "", false},
	}
	for _, tt := range tests {
		got, ok := PagePath("blueprints", tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("PagePath(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
	if got := MarkdownPath("blueprints", "docs/a.md"); got != "blueprints/docs/a.blueprint.md" {
		t.Errorf("MarkdownPath = %q", got)
	}
}

func TestLocatePrefersMarkdown(t *testing.T) {
	root := t.TempDir()
	p := Project{Root: root, BlueprintRoot: "blueprints"}
	if got := p.Locate("docs/a.md"); got != "blueprints/docs/a.blueprint.yaml" {
		t.Errorf("Locate with neither file = %q, want the YAML path", got)
	}
	dir := filepath.Join(root, "blueprints", "docs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.blueprint.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := p.Locate("docs/a.md"); got != "blueprints/docs/a.blueprint.md" {
		t.Errorf("Locate with a markdown file = %q, want it", got)
	}
}

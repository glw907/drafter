package blueprint

import (
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

func TestCheckMarkdownForms(t *testing.T) {
	edit := func(old, new string) string {
		if !strings.Contains(goodMD, old) {
			t.Fatalf("goodMD has no %q", old)
		}
		return strings.Replace(goodMD, old, new, 1)
	}
	const constraints = ":::constraints\n- State no version range. :fact[bbbbbb]\n:::\n\n"
	extra := map[string]string{"src/key.go": "package src\n"}
	pass := []struct{ name, md string }{
		{"a constraints block under the reader block", edit("## Before you begin", constraints+"## Before you begin")},
		{"a code bullet with a fence and an existing source", edit("1. Generate", "- Code: the key helper. :source[/src/key.go#Key]\n\n```go\nfunc Key() {}\n```\n\n1. Generate")},
		{"a code bullet whose source is relative to the page", edit("1. Generate", "- Code: the key helper. :source[../src/key.go]\n\n```go\nx\n```\n\n1. Generate")},
		{"a comment line between bullet and fence", edit("1. Generate", "- Code: the key helper.\n<!-- snippet-check-skip: sketch -->\n```go\nx\n```\n\n1. Generate")},
		{"a figure bullet", edit("1. Generate", "- Figure: the rotation flow. :source[/src/key.go]\n\n1. Generate")},
		{"a figure in the reader block", edit("An operator who rotates keys. :fact[aaaaaa]\n", "An operator who rotates keys. :fact[aaaaaa]\n- Figure: the flow. :source[/src/key.go]\n")},
	}
	for _, tt := range pass {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mdProject(t, tt.md, extra).Check(nil)
			if err != nil {
				t.Fatal(err)
			}
			if errs := errorsOf(got); len(errs) != 0 {
				t.Errorf("errors = %+v, want none", errs)
			}
		})
	}

	tests := []struct {
		name string
		md   string
		anch string
		msg  string
	}{
		{"a fence with no code bullet", edit("1. Generate", "```go\nx\n```\n\n1. Generate"), "```go", "no Code: or Figure: bullet directly above it"},
		{"a fence under a plain bullet", edit("1. Generate", "- A note.\n\n```go\nx\n```\n\n1. Generate"), "```go", "no Code: or Figure: bullet directly above it"},
		{"a second constraints block", edit("## Before you begin", constraints+constraints+"## Before you begin"), ":::constraints\n- State no version range. :fact[bbbbbb]\n:::\n\n## Before", "second :::constraints"},
		{"a constraints block above the reader block", edit(":::reader", constraints+":::reader"), ":::constraints", "not between the :::reader block and the first heading"},
		{"a constraints block under a heading", edit("1. Generate", constraints+"1. Generate"), ":::constraints", "not between the :::reader block and the first heading"},
		{"a code source that does not exist", edit("1. Generate", "- Code: the key helper. :source[/src/missing.go]\n\n1. Generate"), "- Code:", "source src/missing.go does not exist"},
		{"a code source outside the repository", edit("1. Generate", "- Code: the key helper. :source[../../../../outside.go]\n\n1. Generate"), "- Code:", "leaves the repository"},
		{"a source tag on a plain bullet", edit("1. Generate", "- A note. :source[/src/key.go]\n\n1. Generate"), "- A note.", ":source tag is not on a Code: or Figure: bullet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mdProject(t, tt.md, extra).Check(nil)
			if err != nil {
				t.Fatal(err)
			}
			errs := slices.DeleteFunc(errorsOf(got), func(f plan.Finding) bool { return f.File != guideMD })
			if len(errs) != 1 || !strings.Contains(errs[0].Message, tt.msg) {
				t.Fatalf("errors = %+v, want exactly one containing %q", errs, tt.msg)
			}
			if want := strings.Count(tt.md[:strings.Index(tt.md, tt.anch)], "\n") + 1; errs[0].Line != want {
				t.Errorf("line = %d, want %d", errs[0].Line, want)
			}
		})
	}
}

func TestRefsMarkdownForms(t *testing.T) {
	md := strings.Replace(goodMD, "## Before you begin", ":::constraints\n- State no version range. :fact[bbbbbb]\n:::\n\n## Before you begin", 1)
	md = strings.Replace(md, "1. Generate", "- Code: the key helper. :source[/src/key.go] :fact[bbbbbb]\n\n1. Generate", 1)
	p := mdProject(t, md, map[string]string{"src/key.go": "package src\n"})
	refs, held, err := p.Refs("bbbbbb")
	if err != nil || !held {
		t.Fatalf("Refs = %v, held %v, %v", refs, held, err)
	}
	want := []Ref{
		{guideMD, lineOf(t, md, "State no version range."), "constraints", "constraints"},
		{guideMD, lineOf(t, md, "You need the admin role."), "before-you-begin.p1", "example"},
		{guideMD, lineOf(t, md, "Code: the key helper."), "rotate-the-key.p1", "code"},
		{guideMD, lineOf(t, md, "**Fix.**"), "rotate-the-key.row1", "example"},
	}
	if !slices.Equal(refs, want) {
		t.Errorf("Refs = %+v, want %+v", refs, want)
	}
}

func TestCompareMarkdownConstraints(t *testing.T) {
	entry := &Page{Path: guidePage, Title: "Rotate the widget key"}
	facts := plan.ParseFacts(map[string]string{"f.md": "# Facts\n\n- `f:bbbbbb` Rotating needs the admin role. Source: `src/rotate.go`. [verified]\n"}, "")
	rev := func(md string) Revision { return Revision{Blueprint: []byte(md), Entry: entry, Facts: facts} }
	with := func(line string) string {
		return strings.Replace(goodMD, "## Before you begin", ":::constraints\n"+line+"\n:::\n\n## Before you begin", 1)
	}
	if got := Compare(guideMD, rev(with("- A. :fact[bbbbbb]")), rev(with("- A. :fact[bbbbbb]"))); got.Kind != ScopeUnchanged {
		t.Errorf("identical constraints = %+v, want unchanged", got)
	}
	if got := Compare(guideMD, rev(with("- A. :fact[bbbbbb]")), rev(with("- B. :fact[bbbbbb]"))); got.Kind != ScopeScoped {
		t.Errorf("reworded constraint = %+v, want scoped", got)
	}
	if got := Compare(guideMD, rev(goodMD), rev(with("- A."))); got.Kind != ScopeScoped {
		t.Errorf("a new constraints block = %+v, want scoped", got)
	}
}

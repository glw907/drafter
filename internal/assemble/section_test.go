package assemble

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/stock"
)

const sectionDoc = "# Register\n\nintro\n\n## One\n\nONE-BODY\n\n### Deeper\n\nDEEP-BODY\n\n## Two\n\nTWO-BODY\n\n# Next\n\nNEXT-BODY\n"

func TestSectionTokens(t *testing.T) {
	f := newFixture(t)
	f.write(t, filepath.Join(f.root, "docs", "register.md"), sectionDoc)
	f.write(t, filepath.Join(f.root, "docs", "huge.md"), "# Huge\n\n## Small\n\nSMALL-BODY\n\n## Filler\n\n"+strings.Repeat("x", 300<<10)+"\n")
	f.write(t, filepath.Join(f.root, "docs", "untracked-register.md"), "## Hidden\n\nHIDDEN-BODY\n")
	f.git(t, "add", "docs/register.md", "docs/huge.md")

	tests := []struct {
		name   string
		prompt string
		in     []string // substrings the request must hold
		out    []string // substrings it must not hold
	}{
		{"a section stops at the next heading of its level", "Read docs/register.md#one now.",
			[]string{"=== excerpt: docs/register.md#one ===", "ONE-BODY"}, []string{"TWO-BODY", "NEXT-BODY"}},
		{"a deeper heading stays inside the section", "Read docs/register.md#one now.",
			[]string{"### Deeper", "DEEP-BODY"}, []string{"TWO-BODY"}},
		{"a higher-level heading ends the section", "Read docs/register.md#two now.",
			[]string{"TWO-BODY"}, []string{"NEXT-BODY", "ONE-BODY"}},
		{"the last section runs to the end of the file", "Read docs/register.md#next now.",
			[]string{"NEXT-BODY"}, []string{"TWO-BODY"}},
		{"a sentence mark after the token", "See docs/register.md#two.",
			[]string{"TWO-BODY"}, nil},
		{"a missing slug says so", "Read docs/register.md#absent now.",
			[]string{"=== excerpt: docs/register.md#absent ===\nno excerpt: no heading absent"}, nil},
		{"the path and the section both inline", "Read docs/register.md and docs/register.md#two now.",
			[]string{"=== file: docs/register.md ===", "=== excerpt: docs/register.md#two ==="}, nil},
		{"a section of a file over the limit inlines", "Read docs/huge.md#small now.",
			[]string{"SMALL-BODY"}, []string{"not inlined: 3"}},
		{"a section over the limit does not", "Read docs/huge.md#filler now.",
			[]string{"not inlined: 307"}, []string{"xxxxxxxx"}},
		{"an untracked path's section stays out", "Read docs/untracked-register.md#hidden now.",
			nil, []string{"HIDDEN-BODY"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requestText(t, f.input(f.config(), tt.prompt))
			for _, s := range tt.in {
				if !strings.Contains(got, s) {
					t.Errorf("request lacks %q", s)
				}
			}
			for _, s := range tt.out {
				if strings.Contains(got, s) {
					t.Errorf("request holds %q", s)
				}
			}
		})
	}
}

func TestDrafterTokens(t *testing.T) {
	f := newFixture(t)
	anatomies := mustFile(t, "anatomies.md")
	first := plan.Headings(anatomies)[0]
	firstLine, _, _ := strings.Cut(string(anatomies), "\n")
	tests := []struct {
		name   string
		prompt string
		in     []string
		out    []string
	}{
		{"an embedded file inlines whole", "Read drafter:introductions.md now.",
			[]string{"=== file: drafter:introductions.md ===", strings.TrimSpace(string(mustFile(t, "introductions.md")))}, nil},
		{"an unknown name says so", "Read drafter:nothing.md now.",
			[]string{"=== file: drafter:nothing.md ===\nnot inlined: no drafter file nothing.md"}, nil},
		{"an embedded excerpt", "Read drafter:anatomies.md#" + first.Slug + " now.",
			[]string{"=== excerpt: drafter:anatomies.md#" + first.Slug + " ===", firstLine}, []string{"no excerpt: no heading"}},
		{"an unknown embedded name has no excerpt", "Read drafter:nothing.md#x now.",
			[]string{"not inlined: no drafter file nothing.md"}, nil},
		{"the word drafter before a colon is prose", "The drafter: reads files.", nil, []string{"=== file: drafter:"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requestText(t, f.input(f.config(), tt.prompt))
			for _, s := range tt.in {
				if !strings.Contains(got, s) {
					t.Errorf("request lacks %q", s)
				}
			}
			for _, s := range tt.out {
				if strings.Contains(got, s) {
					t.Errorf("request holds %q", s)
				}
			}
		})
	}
}

func mustFile(t *testing.T, name string) []byte {
	t.Helper()
	data, ok := stock.Files(name)
	if !ok {
		t.Fatalf("no embedded file %s", name)
	}
	return data
}

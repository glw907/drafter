package plan

import (
	"os"
	"path/filepath"
	"testing"
)

// The valid plan sits at docs/guides/rotate-the-signing-key.md and carries one link item, to
// docs/reference/keys.md, so a cross-link to keys.md is satisfied and one to planned.md is not.
func TestCheckCrossLinks(t *testing.T) {
	const (
		guide   = `{"path":"docs/guides/rotate-the-signing-key.md","slug":"rotate"}`
		keys    = `{"path":"docs/reference/keys.md","slug":"keys"}`
		planned = `{"path":"docs/reference/planned.md","slug":"planned"}`
	)
	outline := func(pages, crossLinks string) string {
		return `{"pages":[` + pages + `],"crossLinks":[` + crossLinks + `]}`
	}
	tests := []struct {
		name     string
		outlines []string
		wants    []want
	}{
		{"a cross-link no link item follows warns",
			[]string{outline(guide+","+planned, `{"from":"docs/guides/rotate-the-signing-key.md","to":"docs/reference/planned.md","why":"the next read"}`)},
			[]want{{3, SeverityWarning, "links", `cross-links this page to "docs/reference/planned.md" (the next read), and no link item targets docs/reference/planned.md`}}},
		{"a cross-link a link item follows does not",
			[]string{outline(guide+","+keys, `{"from":"docs/guides/rotate-the-signing-key.md","to":"docs/reference/keys.md","why":"the lifetime"}`)},
			nil},
		{"an outline with no crossLinks does not",
			[]string{`{"pages":[` + guide + `,` + planned + `]}`},
			nil},
		{"a cross-link whose from and to are slugs",
			[]string{outline(guide+","+planned, `{"from":"rotate","to":"planned","why":"the next read"}`)},
			[]want{{3, SeverityWarning, "links", `cross-links this page to "planned" (the next read), and no link item targets docs/reference/planned.md`}}},
		{"a to slug that lives in a second outline",
			[]string{outline(guide, `{"from":"rotate","to":"keys","why":"the lifetime"}`), `{"pages":[` + keys + `]}`},
			nil},
		{"a to slug in a second outline that no link follows",
			[]string{outline(guide, `{"from":"rotate","to":"planned","why":"the next read"}`), `{"pages":[` + planned + `]}`},
			[]want{{3, SeverityWarning, "links", `no link item targets docs/reference/planned.md`}}},
		{"a to that resolves to no page warns",
			[]string{outline(guide, `{"from":"rotate","to":"nowhere","why":"the next read"}`)},
			[]want{{3, SeverityWarning, "links", `cross-links this page to "nowhere" (the next read), which names no page the outline lists`}}},
		{"a link to the target with an anchor counts",
			[]string{outline(guide+","+keys, `{"from":"rotate","to":"docs/reference/keys.md","why":"the lifetime"}`)},
			nil},
		{"a cross-link from another page is not this plan's",
			[]string{outline(guide+","+keys+","+planned, `{"from":"docs/reference/keys.md","to":"planned","why":"the next read"}`)},
			nil},
		{"a plan whose page is not in the outline is not checked",
			[]string{outline(keys+","+planned, `{"from":"docs/guides/rotate-the-signing-key.md","to":"planned","why":"the next read"}`)},
			nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			var files []string
			for i, body := range tt.outlines {
				f := filepath.Join(dir, string(rune('a'+i))+".json")
				if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
				files = append(files, f)
			}
			o, err := LoadOutlines(files)
			if err != nil {
				t.Fatal(err)
			}
			got := runCheck(t, readValid(t), func(co *CheckOptions) { co.Outline = o })
			assertFindings(t, got, tt.wants)
		})
	}
}

// A repo-relative link target is compared as a path, whatever its form.
func TestCheckCrossLinkTargetForms(t *testing.T) {
	const target = "../reference/keys.md#key-lifetime"
	dir := t.TempDir()
	f := filepath.Join(dir, "o.json")
	body := `{"pages":[{"path":"docs/guides/rotate-the-signing-key.md"},{"path":"docs/reference/keys.md"}],` +
		`"crossLinks":[{"from":"docs/guides/rotate-the-signing-key.md","to":"docs/reference/keys.md","why":"the lifetime"}]}`
	if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	o, err := LoadOutlines([]string{f})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, repl string
		wants      []want
	}{
		{"a relative path", target, nil},
		{"a root-relative path", "/docs/reference/keys.md", nil},
		{"a same-page anchor does not count", `"#create-the-new-key"`, []want{
			{3, SeverityWarning, "links", "no link item targets docs/reference/keys.md"},
		}},
		{"a URL does not count", "https://example.com/keys.md", []want{
			{3, SeverityWarning, "links", "no link item targets docs/reference/keys.md"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runCheck(t, mutate(t, target, tt.repl), func(co *CheckOptions) { co.Outline = o })
			assertFindings(t, got, tt.wants)
		})
	}
}

func TestLoadOutlineCrossLinks(t *testing.T) {
	f := filepath.Join(t.TempDir(), "o.json")
	body := `{"pages":[{"path":"docs/a.md","slug":"a"},{"path":"./docs/b.md","slug":"b"}],"crossLinks":[{"from":"a","to":"b","why":"w"}]}`
	if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	o, err := LoadOutline(f)
	if err != nil {
		t.Fatal(err)
	}
	for ref, want := range map[string]string{"a": "docs/a.md", "b": "docs/b.md", "docs/a.md": "docs/a.md", "./docs/b.md": "docs/b.md"} {
		if got, ok := o.resolve(ref); !ok || got != want {
			t.Errorf("resolve(%q) = %q, %v; want %q", ref, got, ok, want)
		}
	}
	if got, ok := o.resolve("c"); ok {
		t.Errorf("resolve(c) = %q, want no page", got)
	}
	if len(o.crossLinks) != 1 || o.crossLinks[0] != (crossLink{"a", "b", "w"}) {
		t.Errorf("crossLinks = %+v", o.crossLinks)
	}
}

package plan

import "testing"

func TestNewOutline(t *testing.T) {
	o := NewOutline(
		[]string{"docs/a.md", "./docs/b.md"},
		map[string]string{"a": "docs/a.md", "b": "./docs/b.md"},
		[]CrossLink{{From: "a", To: "b", Why: "w"}},
	)
	for ref, want := range map[string]string{"a": "docs/a.md", "b": "docs/b.md", "docs/a.md": "docs/a.md", "./docs/b.md": "docs/b.md"} {
		if got, ok := o.resolve(ref); !ok || got != want {
			t.Errorf("resolve(%q) = %q, %v; want %q", ref, got, ok, want)
		}
	}
	if _, ok := o.resolve("c"); ok {
		t.Error("resolve(c) found a page")
	}
	if !o.lists("docs/b.md") || o.lists("docs/c.md") {
		t.Error("lists disagrees with the paths given")
	}
	if len(o.crossLinks) != 1 || o.crossLinks[0] != (crossLink{"a", "b", "w"}) {
		t.Errorf("crossLinks = %+v", o.crossLinks)
	}
}

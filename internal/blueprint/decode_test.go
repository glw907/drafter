package blueprint

import (
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/catalog/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func decodeOK(t *testing.T, src string) *Blueprint {
	t.Helper()
	b, fs := Decode("x.blueprint.yaml", []byte(src))
	if b == nil || len(fs) > 0 {
		t.Fatalf("Decode = %v, %v; want a clean decode", b != nil, fs)
	}
	return b
}

func TestDecodeBase(t *testing.T) {
	b := decodeOK(t, readFixture(t, "base.blueprint.yaml"))
	if b.Sections[1].ID != "rotate" || b.Sections[1].Level != 2 || b.Sections[1].Why == "" {
		t.Errorf("section 1 = %+v, want id rotate, level 2 by default, and a why", b.Sections[1])
	}
	if len(b.Exemplars) != 1 || b.Exemplars[0].ManifestType != "Task guide" || b.Exemplars[0].Source != "guides/rotate-a-secret/" {
		t.Errorf("exemplars = %+v", b.Exemplars)
	}
	if !strings.Contains(b.Framing.Readers, "`f:aaaaaa`") || b.Framing.Lines["readers"] == 0 {
		t.Errorf("framing = %+v", b.Framing)
	}
}

func TestDecodeFields(t *testing.T) {
	base := readFixture(t, "base.blueprint.yaml")
	tests := []struct {
		name, from, to string
		check          func(t *testing.T, b *Blueprint)
	}{
		{"origin on any item", "        fact: f:bbbbbb\n", "        fact: f:bbbbbb\n        origin: human\n", func(t *testing.T, b *Blueprint) {
			if got := b.Sections[0].Items[0].Origin; got != "human" {
				t.Errorf("origin = %q", got)
			}
		}},
		{"style notes on a section", "    heading: Before you begin\n", "    heading: Before you begin\n    styleNotes:\n      - old: clunky\n        new: plain\n", func(t *testing.T, b *Blueprint) {
			if n := b.Sections[0].StyleNotes; len(n) != 1 || n[0].Old != "clunky" || n[0].New != "plain" {
				t.Errorf("styleNotes = %+v", n)
			}
		}},
		{"review human", "preconditions:", "review: human\npreconditions:", func(t *testing.T, b *Blueprint) {
			if b.Review != "human" {
				t.Errorf("review = %q", b.Review)
			}
		}},
		{"fact item without a use", "        use: |\n          Only the role requirement.\n", "", func(t *testing.T, b *Blueprint) {
			if it := b.Sections[0].Items[0]; it.Fact != "f:bbbbbb" || it.Use != "" {
				t.Errorf("item = %+v", it)
			}
		}},
		{"level 3 under a level 2", "dispositions:", "  - id: sub\n    heading: A subsection\n    level: 3\n    why: |\n      Detail.\n    opens: |\n      Detail follows.\n    items:\n      - id: h-sub\n        kind: handoff\n        text: |\n          See the reference.\ndispositions:", func(t *testing.T, b *Blueprint) {
			if s := b.Sections[3]; s.Level != 3 || s.ID != "sub" {
				t.Errorf("section = %+v", s)
			}
		}},
		{"figure item", "dispositions:", "  - id: fig\n    heading: A figure\n    why: |\n      Shows the flow.\n    opens: |\n      The figure shows the flow.\n    items:\n      - id: fig-1\n        kind: figure\n        text: |\n          The key flow.\ndispositions:", func(t *testing.T, b *Blueprint) {
			if it := b.Sections[3].Items[0]; it.Kind != "figure" {
				t.Errorf("item = %+v", it)
			}
		}},
		{"empty items above a level 3", "dispositions:", "  - id: parent\n    heading: A parent\n    why: |\n      Groups the detail.\n    opens: |\n      Two details follow.\n    items: []\n  - id: child\n    heading: A child\n    level: 3\n    why: |\n      Detail.\n    opens: |\n      Detail.\n    items:\n      - id: h-child\n        kind: handoff\n        text: |\n          See the reference.\ndispositions:", func(t *testing.T, b *Blueprint) {
			if len(b.Sections[3].Items) != 0 || b.Sections[4].Level != 3 {
				t.Errorf("sections = %+v", b.Sections[3:])
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(base, tt.from) {
				t.Fatalf("fixture lacks %q", tt.from)
			}
			b, fs := Decode("x.blueprint.yaml", []byte(strings.Replace(base, tt.from, tt.to, 1)))
			if len(fs) > 0 {
				t.Fatalf("findings = %v", fs)
			}
			tt.check(t, b)
		})
	}
}

func TestDecodeEveryKind(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range []string{"base.blueprint.yaml", "after/add-example.blueprint.yaml"} {
		b := decodeOK(t, readFixture(t, name))
		for _, s := range b.Sections {
			for _, it := range s.Items {
				seen[it.Kind] = true
			}
		}
	}
	seen["figure"] = true // covered by TestDecodeFields
	if got, want := slices.Sorted(maps.Keys(seen)), slices.Sorted(maps.Keys(kinds)); !slices.Equal(got, want) {
		t.Errorf("fixtures cover kinds %v, the kinds table has %v", got, want)
	}
	ex := decodeOK(t, readFixture(t, "after/add-example.blueprint.yaml"))
	for _, it := range ex.Sections[1].Items {
		if it.Kind == "example" && (it.Text == "" || it.Facts != nil) {
			t.Errorf("example item = %+v", it)
		}
	}
}

func TestDecodeRefusals(t *testing.T) {
	base := readFixture(t, "base.blueprint.yaml")
	tests := []struct {
		name, from, to, want string
	}{
		{"a section without an id", "  - id: before-you-begin\n    heading", "  - heading", `missing required field "id"`},
		{"an id shared by a section and an item", "      - id: needs-admin\n", "      - id: rotate\n", "share one namespace"},
		{"an id shared by two items", "      - id: s-verify\n", "      - id: r-verify\n", "share one namespace"},
		{"an id off the pattern", "  - id: verify\n    heading", "  - id: Verify\n    heading", "not a lowercase token"},
		{"a field outside the kind's set", "        fact: f:bbbbbb\n", "        fact: f:bbbbbb\n        text: no\n", `field "text" does not belong to kind "fact"`},
		{"a duplicate key", "version: 1\n", "version: 1\nversion: 1\n", "duplicate key"},
		{"a friction field", "preconditions:", "friction:\n  - slow\npreconditions:", `unknown field "friction"`},
		{"a page block", "preconditions:", "page:\n  path: docs/guide.md\npreconditions:", `unknown field "page"`},
		{"a registerSections field", "preconditions:", "registerSections: [a]\npreconditions:", `unknown field "registerSections"`},
		{"an unknown kind", "kind: handoff", "kind: widget", `unknown kind "widget"`},
		{"a prose field that decodes as a sequence", "        text: widget key reference\n", "        text: [[verify]]\n", "decodes as a list"},
		{"an unknown origin", "        fact: f:bbbbbb\n", "        fact: f:bbbbbb\n        origin: agent\n", `unknown origin "agent"`},
		{"an unknown review", "preconditions:", "review: agent\npreconditions:", `unknown review "agent"`},
		{"an unsupported version", "version: 1\n", "version: 2\n", "unsupported version 2"},
		{"a level off the list", "    heading: Verify the new key\n", "    heading: Verify the new key\n    level: 4\n", "level 4"},
		{"an example without text", "      - id: l-docs\n", "      - id: e-bare\n        kind: example\n      - id: l-docs\n", `kind "example" needs the field "text"`},
		{"an unknown mode", "mode: claim", "mode: guess", `unknown mode "guess"`},
		{"a subordinated disposition without a reference", "    disposition: cut\n", "    disposition: subordinated\n", "needs a reference"},
		{"empty items with no level 3 below", "    items:\n      - id: s-verify", "    items: []\n  - id: tail\n    heading: Tail\n    why: |\n      t\n    opens: |\n      t\n    items:\n      - id: s-verify", "must hold at least one entry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(base, tt.from) {
				t.Fatalf("fixture lacks %q", tt.from)
			}
			b, fs := Decode("x.blueprint.yaml", []byte(strings.Replace(base, tt.from, tt.to, 1)))
			if b == nil {
				t.Fatal("Decode returned nil for a mapping")
			}
			if !plan.HasError(fs) {
				t.Fatalf("findings = %v, want an error", fs)
			}
			if !slices.ContainsFunc(fs, func(f plan.Finding) bool { return strings.Contains(f.Message, tt.want) }) {
				t.Errorf("findings = %v, want one containing %q", fs, tt.want)
			}
		})
	}
}

func TestDecodeNotAMapping(t *testing.T) {
	for _, src := range []string{"", "- a\n", "version: 1\n---\nversion: 1\n"} {
		if b, fs := Decode("x.blueprint.yaml", []byte(src)); b != nil || !plan.HasError(fs) {
			t.Errorf("Decode(%q) = %v, %v; want nil and an error", src, b != nil, fs)
		}
	}
}

func TestDecodeOutline(t *testing.T) {
	o, fs := DecodeOutline("outline.yaml", []byte(readFixture(t, "outline.yaml")))
	if len(fs) > 0 {
		t.Fatalf("findings = %v", fs)
	}
	if p := o.Pages[0]; !p.Managed || p.Slug != "guide" || len(p.Covers) != 1 || o.Types[0].Anatomy != "Task guide" {
		t.Errorf("outline = %+v", o)
	}

	tests := []struct {
		name, src, want string
	}{
		{"an unmanaged entry needs only a path", "version: 1\ntypes:\n  - name: t\npages:\n  - path: CHANGELOG.md\n    managed: false\n", ""},
		{"a managed entry needs its fields", "version: 1\ntypes:\n  - name: t\npages:\n  - path: a.md\n", `a managed page needs "slug"`},
		{"a path listed twice", "version: 1\ntypes:\n  - name: t\npages:\n  - path: a.md\n    managed: false\n  - path: a.md\n    managed: false\n", "listed twice"},
		{"a path outside the repository", "version: 1\ntypes:\n  - name: t\npages:\n  - path: ../a.md\n    managed: false\n", "not a path inside the repository"},
		{"a type declared twice", "version: 1\ntypes:\n  - name: t\n  - name: t\npages:\n  - path: a.md\n    managed: false\n", "declared twice"},
		{"a crossLink without a reason", "version: 1\ntypes:\n  - name: t\npages:\n  - path: a.md\n    managed: false\ncrossLinks:\n  - from: a\n    to: b\n", `missing required field "why"`},
		{"an unknown field", "version: 1\ntypes:\n  - name: t\npages:\n  - path: a.md\n    managed: false\n    kept: true\n", `unknown field "kept"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, fs := DecodeOutline("outline.yaml", []byte(tt.src))
			if tt.want == "" {
				if len(fs) > 0 || o.Pages[0].Managed {
					t.Errorf("findings = %v, managed = %v; want none and false", fs, o.Pages[0].Managed)
				}
				return
			}
			if !slices.ContainsFunc(fs, func(f plan.Finding) bool { return strings.Contains(f.Message, tt.want) }) {
				t.Errorf("findings = %v, want one containing %q", fs, tt.want)
			}
		})
	}
}

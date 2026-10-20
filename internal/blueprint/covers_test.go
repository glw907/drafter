package blueprint

import (
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
	"go.yaml.in/yaml/v3"
)

const (
	coverKeySizes = "      - id: key-sizes\n        text: Which key sizes the service accepts.\n"
	coverRollback = "      - id: rollback\n        text: How to return to the old key.\n"
	coverString   = "      - Generate and verify a new key.\n"
)

// outlineWithCovers is the catalog outline with its covers list replaced by list.
func outlineWithCovers(t *testing.T, list string) string {
	t.Helper()
	return strings.Replace(readFixture(t, "outline.yaml"), coverString, list, 1)
}

func TestDecodeCovers(t *testing.T) {
	o, fs := DecodeOutline("outline.yaml", []byte(outlineWithCovers(t, coverString+coverKeySizes+coverRollback)))
	if len(fs) > 0 {
		t.Fatalf("findings = %v", fs)
	}
	got := o.Pages[0].Covers
	if len(got) != 3 || got[0].ID != "" || got[0].Text != "Generate and verify a new key." ||
		got[1].ID != "key-sizes" || got[1].Text != "Which key sizes the service accepts." || got[2].ID != "rollback" {
		t.Fatalf("covers = %+v", got)
	}
	if got[1].Lines["id"] == 0 || got[0].Line == 0 {
		t.Errorf("covers lost their lines: %+v", got)
	}

	tests := []struct {
		name, list, want string
	}{
		{"an unknown mapping key", coverString + "      - id: a\n        text: b\n        note: c\n", `covers[1]: unknown field "note"`},
		{"a mapping without an id", "      - text: b\n", `covers[0]: missing required field "id"`},
		{"a mapping without text", "      - id: a\n", `covers[0]: missing required field "text"`},
		{"an id that is not a token", "      - id: Key Sizes\n        text: b\n", `id "Key Sizes" is not a lowercase token`},
		{"a list element", "      - [a, b]\n", "must be a string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, fs := DecodeOutline("outline.yaml", []byte(outlineWithCovers(t, tt.list)))
			if !slices.ContainsFunc(fs, func(f plan.Finding) bool { return strings.Contains(f.Message, tt.want) }) {
				t.Errorf("findings = %v, want one containing %q", fs, tt.want)
			}
		})
	}
}

func TestCoverJSONStrings(t *testing.T) {
	cs := []Cover{{Text: "Generate and verify a new key."}, {ID: "key-sizes", Text: "Which key sizes the service accepts."}}
	want := []string{"Generate and verify a new key.", "key-sizes: Which key sizes the service accepts."}
	if got := CoverStrings(cs); !slices.Equal(got, want) {
		t.Errorf("CoverStrings = %q, want %q", got, want)
	}
}

func TestCoverRoundTripsThroughYAML(t *testing.T) {
	in := []Cover{{Text: "plain"}, {ID: "a", Text: "mapped"}}
	out, err := yaml.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if want := "- plain\n- id: a\n  text: mapped\n"; string(out) != want {
		t.Errorf("marshal = %q, want %q", out, want)
	}
}

func TestDispositionNamesOneTarget(t *testing.T) {
	const disposition = "  - fact: f:dddddd\n    disposition: cut\n    reason: |\n      The page leaves key sizes to the reference.\n"
	tests := []struct {
		name, replace, want string
	}{
		{"fact only", disposition, ""},
		{"covers only", "  - covers: key-sizes\n    disposition: cut\n    reason: Left to the reference.\n", ""},
		{"both", "  - fact: f:dddddd\n    covers: key-sizes\n    disposition: cut\n    reason: Left to the reference.\n", "names both a fact and a covers id"},
		{"neither", "  - disposition: cut\n    reason: Left to the reference.\n", "names neither a fact nor a covers id"},
		{"a covers id that is not a token", "  - covers: Key Sizes\n    disposition: cut\n    reason: Left to the reference.\n", "is not a lowercase token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := strings.Replace(readFixture(t, "base.blueprint.yaml"), disposition, tt.replace, 1)
			if tt.replace != disposition && src == readFixture(t, "base.blueprint.yaml") {
				t.Fatal("the fixture has no disposition to replace")
			}
			_, fs := Decode("x.blueprint.yaml", []byte(src))
			if tt.want == "" {
				if len(fs) > 0 {
					t.Errorf("findings = %v, want none", fs)
				}
				return
			}
			if !slices.ContainsFunc(fs, func(f plan.Finding) bool { return strings.Contains(f.Message, tt.want) }) {
				t.Errorf("findings = %v, want one containing %q", fs, tt.want)
			}
		})
	}
}

func TestCheckCovers(t *testing.T) {
	const (
		disposition = "    reason: |\n      The page leaves key sizes to the reference.\n"
		coverCut    = "  - covers: key-sizes\n    disposition: cut\n    reason: Left to the reference.\n"
		section     = "    heading: Before you begin\n"
	)
	mixed := coverString + coverKeySizes + coverRollback
	blueprint := func(sectionCovers, dispositions string) string {
		src := strings.Replace(readFixture(t, "base.blueprint.yaml"), section, section+sectionCovers, 1)
		return strings.Replace(src, disposition, disposition+dispositions, 1)
	}
	tests := []struct {
		name      string
		outline   string
		blueprint string
		file      string
		msg       string // empty means the covers check passes
	}{
		{"a section and a disposition claim every mapped id", mixed, blueprint("    covers: [rollback]\n", coverCut), "", ""},
		{"string entries need no claim", coverString, blueprint("", ""), "", ""},
		{"an unclaimed mapped id", mixed, blueprint("    covers: [rollback]\n", ""), "blueprints/outline.yaml", `covers id "key-sizes" of docs/guide.md is claimed by no section and no disposition`},
		{"a section names an unknown id", mixed, blueprint("    covers: [rollback, nowhere]\n", coverCut), guideBlueprint, `names the covers id "nowhere"`},
		{"a disposition names an unknown id", mixed, blueprint("    covers: [rollback]\n", coverCut+"  - covers: gone\n    disposition: cut\n    reason: x\n"), guideBlueprint, `names the covers id "gone"`},
		{"an id used twice in the entry", mixed + coverRollback, blueprint("    covers: [rollback]\n", coverCut), "blueprints/outline.yaml", `covers id "rollback" is used twice`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := catalogProject(t, map[string]string{
				"blueprints/outline.yaml": outlineWithCovers(t, tt.outline),
				guideBlueprint:            tt.blueprint,
			})
			got, err := p.Check(nil)
			if err != nil {
				t.Fatal(err)
			}
			hits := findingsFor(got, "covers")
			if tt.msg == "" {
				if len(hits) != 0 || plan.HasError(got) {
					t.Errorf("a clean twin has findings: %+v", got)
				}
				return
			}
			for _, f := range hits {
				if f.File == tt.file && f.Severity == plan.SeverityError && f.Line > 0 && strings.Contains(f.Message, tt.msg) {
					return
				}
			}
			t.Errorf("no error covers finding in %s containing %q: %+v", tt.file, tt.msg, hits)
		})
	}
}

package blueprint

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

// project writes a project under a temporary root holding the outline, one blueprint at the path
// rule's location for page, and the facts directory copied from factsDir, and returns the Project
// that checks it.
func project(t *testing.T, outline, page string, bp []byte, factsDir string, extra ...string) Project {
	t.Helper()
	root := t.TempDir()
	write := func(rel string, data []byte) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("blueprints/outline.yaml", []byte(outline))
	write(BlueprintPath("blueprints", page), bp)
	write(page, []byte("# Page\n"))
	for _, rel := range extra {
		write(rel, []byte("# Extra\n\n## Key lifetime\n"))
	}
	if factsDir != "" {
		entries, err := os.ReadDir(factsDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			data, err := os.ReadFile(filepath.Join(factsDir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			write("blueprints/facts/"+e.Name(), data)
		}
	}
	p := Project{Root: root, BlueprintRoot: "blueprints", Outline: "blueprints/outline.yaml"}
	if factsDir != "" {
		p.FactsDir = filepath.Join(root, "blueprints/facts")
	}
	return p
}

func TestEditCatalogFixturesPassCheck(t *testing.T) {
	outline := readFixture(t, "outline.yaml")
	cases := map[string]struct{ bp, facts string }{"base": {"base.blueprint.yaml", "testdata/catalog/facts"}}
	for _, row := range catalog {
		facts := "testdata/catalog/facts"
		if row.factsFile != "" {
			facts = filepath.Dir(row.factsFile)
		}
		cases[row.name] = struct{ bp, facts string }{"after/" + row.file + ".blueprint.yaml", facts}
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p := project(t, outline, "docs/guide.md", []byte(readFixture(t, c.bp)), c.facts)
			findings, err := p.Check(nil)
			if err != nil {
				t.Fatal(err)
			}
			if plan.HasError(findings) {
				t.Errorf("findings = %+v, want none that are errors", findings)
			}
		})
	}
}

// TestConvertedFixtureFindings pins what drafter check says about a freshly converted blueprint:
// the work left for a person, and nothing else.
func TestConvertedFixtureFindings(t *testing.T) {
	outline, err := ConvertOutline([]NamedData{{Name: "widget.outline.json", Data: convertFixture(t, "widget.outline.json")}})
	if err != nil {
		t.Fatal(err)
	}
	conv, _ := convertedFixture(t)
	p := project(t, string(outline), convertPage, conv.Data, "", "docs/reference/keys.md", "docs/guides/examples/check.sh")
	findings, err := p.Check(nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range findings {
		if f.Severity == plan.SeverityError {
			got = append(got, f.Check)
		}
	}
	slices.Sort(got)
	// Five sections have no why, one use does not open with Only or Omit, and the outline's type
	// names no anatomy. The ids error is the fixture plan's own: it places the key-store fact and
	// also disposes it, which v1's check-plan reports too.
	want := []string{"ids", "types", "use", "why", "why", "why", "why", "why"}
	if !slices.Equal(got, want) {
		for _, f := range findings {
			t.Logf("%s:%d %s %s: %s", f.File, f.Line, f.Severity, f.Check, f.Message)
		}
		t.Errorf("errors = %v, want %v", got, want)
	}
}

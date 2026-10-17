package blueprint

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

// writeTree writes each path of files, relative to dir, with its content.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		file := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// catalogProject builds the catalog fixtures into a project under a temp directory: the base
// blueprint for docs/guide.md, the outline, the facts store, and the page itself. extra files
// overwrite or add to the tree; an empty content removes the file.
func catalogProject(t *testing.T, extra map[string]string) Project {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"blueprints/docs/guide.blueprint.yaml": readFixture(t, "base.blueprint.yaml"),
		"blueprints/outline.yaml":              readFixture(t, "outline.yaml"),
		"blueprints/facts/facts.md":            readFixture(t, "facts/facts.md"),
		"docs/guide.md":                        "# Rotate the widget key\n",
	}
	maps.Copy(files, extra)
	writeTree(t, root, files)
	for name, content := range files {
		if content == "" {
			if err := os.Remove(filepath.Join(root, filepath.FromSlash(name))); err != nil {
				t.Fatal(err)
			}
		}
	}
	return Project{Root: root, BlueprintRoot: "blueprints", Outline: "blueprints/outline.yaml", FactsDir: filepath.Join(root, "blueprints", "facts")}
}

// withEdit returns the base blueprint with the first occurrence of old replaced by new.
func withEdit(t *testing.T, old, new string) string {
	t.Helper()
	base := readFixture(t, "base.blueprint.yaml")
	if !strings.Contains(base, old) {
		t.Fatalf("base blueprint has no %q", old)
	}
	return strings.Replace(base, old, new, 1)
}

func findingsFor(fs []plan.Finding, check string) []plan.Finding {
	var out []plan.Finding
	for _, f := range fs {
		if f.Check == check {
			out = append(out, f)
		}
	}
	return out
}

func TestCheckCatalogBasePasses(t *testing.T) {
	p := catalogProject(t, nil)
	got, err := p.Check(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("the base project has findings: %+v", got)
	}
}

// checkRow is one failing blueprint fixture: the edit that breaks the base, and the finding the
// check must report at the line the edit lands on.
type checkRow struct {
	name  string
	old   string
	new   string
	check string
	sev   string
	msg   string // a fragment of the message
}

func TestCheckBlueprintFindings(t *testing.T) {
	tests := []checkRow{
		{"unknown handler", "handledBy: h-ask-admin", "handledBy: nobody", "ids", "error", "is no item's id"},
		{"reader state", "requires: [new-key]", "requires: [nowhere]", "reader-state", "error", "no earlier step produces"},
		{"recovery", "        from: s-generate\n", "", "recovery", "error", "can fail"},
		{"links", "target: https://example.com/docs/keys", "target: docs/missing.md", "links", "error", "does not exist"},
		{"fact in no file", "fact: f:bbbbbb", "fact: f:zzzzzz", "facts", "error", "in no facts file"},
		{"fact from an example item", "        kind: code\n        text: |\n          The command that adds the new key.\n",
			"        kind: example\n        text: |\n          The command that adds the new key.\n        facts: [f:yyyyyy]\n", "facts", "error", `"f:yyyyyy" is in no facts file`},
		{"fact from framing prose", "for the two-key model.", "for the two-key model and `f:aaaaab`.", "facts", "error", `"f:aaaaab" is in no facts file`},
		{"section with no why", "    why: |\n      A reader checks the result before deleting anything.\n", "", "why", "error", `section "verify" has no why`},
		{"unknown section reference", "The reader needs the admin role before the first step.", "The reader needs the admin role before [[nowhere]] begins.", "refs", "error", "[[nowhere]]"},
		{"use without Only or Omit", "Only the role requirement.", "The role requirement.", "use", "error", "Only or Omit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := catalogProject(t, map[string]string{"blueprints/docs/guide.blueprint.yaml": withEdit(t, tt.old, tt.new)})
			got, err := p.Check(nil)
			if err != nil {
				t.Fatal(err)
			}
			hits := findingsFor(got, tt.check)
			var found *plan.Finding
			for i := range hits {
				if hits[i].Severity == tt.sev && strings.Contains(hits[i].Message, tt.msg) {
					found = &hits[i]
				}
			}
			if found == nil {
				t.Fatalf("no %s %s finding containing %q in %+v", tt.sev, tt.check, tt.msg, got)
			}
			if found.File != "blueprints/docs/guide.blueprint.yaml" || found.Line < 1 {
				t.Errorf("finding at %s:%d", found.File, found.Line)
			}
		})
	}
}

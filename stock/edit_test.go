package stock

import (
	"slices"
	"strings"
	"testing"
)

// renderCase renders one case of the fixture matrix by name.
func renderCase(t *testing.T, name string) (string, stockCase) {
	t.Helper()
	m, mod := loadMatrix(t, fixtureMatrix)
	i := slices.IndexFunc(m.Cases, func(c stockCase) bool { return c.Name == name })
	if i < 0 {
		t.Fatalf("no case %s in the matrix", name)
	}
	got, err := mod.Render(m.Cases[i].Seat, m.Cases[i].Context)
	if err != nil {
		t.Fatal(err)
	}
	return got, m.Cases[i]
}

func TestEditPassRendering(t *testing.T) {
	whole, _ := renderCase(t, "edit-whole")
	for _, want := range []string{
		"Edit pass over docs/count-items.md.", "`drafter:google-rubric.md`", "`drafter:google-tells.md`",
		"`drafter:anatomies.md#task-guide`", "An edit changes wording, never a claim.", "Finding 1 (edit, advisory)",
	} {
		if !strings.Contains(whole, want) {
			t.Errorf("edit-whole lacks %q", want)
		}
	}
	for _, banned := range []string{"Editing preferences.", "This pass follows a section redraft.", "Markdown file"} {
		if strings.Contains(whole, banned) {
			t.Errorf("edit-whole holds %q with no preferences, scope, or Markdown blueprint", banned)
		}
	}
	section, _ := renderCase(t, "edit-section")
	if !strings.Contains(section, "Change only the sections in scope: `add-items`.") {
		t.Errorf("edit-section is not bounded to its scope:\n%s", section)
	}
}

func TestPreferencesAreCarriedVerbatim(t *testing.T) {
	for _, name := range []string{"edit-markdown", "draft-markdown", "draft-redraft-preferences"} {
		t.Run(name, func(t *testing.T) {
			got, c := renderCase(t, name)
			if c.Context.Preferences == "" || !strings.Contains(got, c.Context.Preferences) {
				t.Errorf("%s lacks the preferences text verbatim:\n%s", name, got)
			}
		})
	}
	for _, name := range []string{"draft-whole", "draft-redraft", "edit-whole"} {
		if got, _ := renderCase(t, name); strings.Contains(got, "Editing preferences.") {
			t.Errorf("%s names preferences although the context carries none", name)
		}
	}
}

func TestMarkdownBlueprintReading(t *testing.T) {
	for _, name := range []string{"edit-markdown", "draft-markdown", "structure-markdown"} {
		t.Run(name, func(t *testing.T) {
			got, _ := renderCase(t, name)
			if !strings.Contains(got, "It is a Markdown file, not YAML.") || !strings.Contains(got, "`:fact[id]`") {
				t.Errorf("%s does not describe the Markdown blueprint", name)
			}
		})
	}
	draft, _ := renderCase(t, "draft-markdown")
	if strings.Contains(draft, "framing.introPlan") || strings.Contains(draft, "`fact` item") {
		t.Error("a Markdown draft prompt names YAML-only fields")
	}
	yaml, _ := renderCase(t, "draft-whole")
	if strings.Contains(yaml, "Markdown file") || !strings.Contains(yaml, "framing.introPlan") {
		t.Error("a YAML draft prompt changed")
	}
}

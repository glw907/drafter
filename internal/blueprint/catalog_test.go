package blueprint

import (
	"maps"
	"os"
	"reflect"
	"slices"
	"testing"

	"go.yaml.in/yaml/v3"
)

// catalogRow is one row of the edit catalog (docs/blueprint-format.md) with its after fixture.
// The rows "Start a new page" and "Follow a rename the gate demands" have none: the first is a
// whole blueprint, and the second edits page prose and fact bullets outside the blueprint.
type catalogRow struct {
	name  string
	file  string
	kinds []string // item kinds the edit may touch
	// factsFile names a changed copy of the facts file, for a row whose edit starts at a fact bullet.
	factsFile string
	// sections lets the edit add a section instead of staying inside one.
	sections bool
}

var catalog = []catalogRow{
	{name: "Record changed behavior", file: "record-behavior", kinds: []string{"fact"}, factsFile: "testdata/catalog/facts-changed/facts.md"},
	{name: "Add an example", file: "add-example", kinds: []string{"example"}},
	{name: "Correct a claim", file: "correct-claim", kinds: []string{"sentence"}},
	{name: "Add a failure case", file: "add-failure", kinds: []string{"failure", "handoff", "step"}},
	{name: "Add a caveat", file: "add-caveat", kinds: []string{"sentence"}},
	{name: "Add a link", file: "add-link", kinds: []string{"link"}},
	{name: "Remove a claim", file: "remove-claim", kinds: []string{"sentence"}},
	{name: "Add or move a section", file: "add-section", kinds: []string{"step"}, sections: true},
}

// itemsByID returns every item of b keyed on id, each stripped of its line numbers, with the id of
// the section that holds it.
func itemsByID(b *Blueprint) (items map[string]Item, home map[string]string) {
	items, home = map[string]Item{}, map[string]string{}
	for _, s := range b.Sections {
		for _, it := range s.Items {
			it.Line, it.Lines = 0, nil
			items[it.ID], home[it.ID] = it, s.ID
		}
	}
	return items, home
}

// withoutItems returns b's YAML with every section's items dropped, so two blueprints compare
// equal exactly when everything but the items matches. Line numbers are not part of the YAML.
func withoutItems(t *testing.T, b *Blueprint, keepSections bool) string {
	t.Helper()
	c := *b
	c.Sections = slices.Clone(b.Sections)
	for i := range c.Sections {
		c.Sections[i].Items = nil
	}
	if !keepSections {
		c.Sections = nil
	}
	out, err := yaml.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestEditCatalog(t *testing.T) {
	base := decodeOK(t, readFixture(t, "base.blueprint.yaml"))
	baseItems, baseHome := itemsByID(base)
	baseFacts, err := os.ReadFile("testdata/catalog/facts/facts.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range catalog {
		t.Run(row.name, func(t *testing.T) {
			after := decodeOK(t, readFixture(t, "after/"+row.file+".blueprint.yaml"))
			afterItems, afterHome := itemsByID(after)

			var changed []string
			sectionOf := map[string]bool{}
			for id, it := range afterItems {
				if old, ok := baseItems[id]; !ok || !reflect.DeepEqual(old, it) {
					changed = append(changed, id)
					sectionOf[afterHome[id]] = true
					if !slices.Contains(row.kinds, it.Kind) {
						t.Errorf("item %s of kind %s changed, but the row names only %v", id, it.Kind, row.kinds)
					}
				}
			}
			for id, it := range baseItems {
				if _, ok := afterItems[id]; !ok {
					changed = append(changed, id)
					sectionOf[baseHome[id]] = true
					if !slices.Contains(row.kinds, it.Kind) {
						t.Errorf("item %s of kind %s was removed, but the row names only %v", id, it.Kind, row.kinds)
					}
				}
			}
			if len(changed) == 0 {
				t.Fatal("the after fixture holds the same items as the base")
			}
			if len(sectionOf) != 1 {
				t.Errorf("the edit touches items in sections %v, want exactly one", slices.Sorted(maps.Keys(sectionOf)))
			}

			if got, want := withoutItems(t, after, !row.sections), withoutItems(t, base, !row.sections); got != want {
				t.Errorf("the edit changed more than items:\n%s\nagainst\n%s", got, want)
			}
			if row.sections && len(after.Sections) == len(base.Sections) {
				t.Error("the section row left the section list as it was")
			}

			if row.factsFile != "" {
				changedFacts, err := os.ReadFile(row.factsFile)
				if err != nil {
					t.Fatal(err)
				}
				if string(changedFacts) == string(baseFacts) {
					t.Error("the fact-bullet row leaves the facts file unchanged")
				}
			}
		})
	}
}

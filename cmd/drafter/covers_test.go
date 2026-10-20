package main

import (
	"slices"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
)

func TestEntryJSONWritesEachCoverAsOneString(t *testing.T) {
	page := blueprint.Page{Path: "docs/a.md", Covers: []blueprint.Cover{
		{Text: "Generate a key."},
		{ID: "grace", Text: "How long the old key keeps working."},
	}}
	e := entryJSON(&blueprint.Outline{}, page, "")
	want := []string{"Generate a key.", "grace: How long the old key keeps working."}
	got, ok := e["covers"].([]string)
	if !ok || !slices.Equal(got, want) {
		t.Errorf("covers = %#v, want %q", e["covers"], want)
	}
}

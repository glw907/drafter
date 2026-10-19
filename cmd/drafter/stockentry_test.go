package main

import (
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
)

func TestStockEntryJSONAnatomyAndRegister(t *testing.T) {
	o := &blueprint.Outline{
		Types: []blueprint.Type{{Name: "task guide", Anatomy: "Task guide"}, {Name: "symptom row", Anatomy: "Symptom row"}},
		Tracks: []blueprint.Track{
			{Name: "admin", Paths: []string{"docs/admin/**"}},
			{Name: "dev", Paths: []string{"docs/dev/**"}, Register: "docs/house.md"},
			{Name: "both", Paths: []string{"docs/admin/twice.md"}, Register: "docs/house.md"},
		},
	}
	tests := []struct {
		name, path, typ, anatomy, register string
	}{
		{"a track with no register", "docs/admin/a.md", "task guide", "Task guide", ""},
		{"a track with a register", "docs/dev/rows.md", "symptom row", "Symptom row", "docs/house.md"},
		{"no track", "docs/other/a.md", "task guide", "Task guide", ""},
		{"two tracks", "docs/admin/twice.md", "task guide", "Task guide", ""},
		{"an undeclared type", "docs/dev/x.md", "concept", "", "docs/house.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := entryJSON(o, blueprint.Page{Path: tt.path, Type: tt.typ})
			if e["anatomy"] != tt.anatomy {
				t.Errorf("anatomy = %v, want %q", e["anatomy"], tt.anatomy)
			}
			reg, ok := e["register"]
			if tt.register == "" && ok {
				t.Errorf("register = %v, want none", reg)
			}
			if tt.register != "" && reg != tt.register {
				t.Errorf("register = %v, want %q", reg, tt.register)
			}
		})
	}
}

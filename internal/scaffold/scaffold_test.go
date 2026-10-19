package scaffold

import (
	"slices"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/stock"
)

func TestOutlineTemplateNamesStockAnatomies(t *testing.T) {
	data, err := templates.ReadFile("templates/outline.yaml")
	if err != nil {
		t.Fatal(err)
	}
	o, fs := blueprint.DecodeOutline("outline.yaml", data)
	if o == nil || len(fs) > 0 {
		t.Fatalf("the template does not decode: %v", fs)
	}
	for _, ty := range o.Types {
		if !slices.Contains(stock.Anatomies, ty.Anatomy) {
			t.Errorf("type %q names %q, which is not a stock anatomy", ty.Name, ty.Anatomy)
		}
	}
}

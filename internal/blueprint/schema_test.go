package blueprint

import (
	"encoding/json"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

type object map[string]any

func loadSchema(t *testing.T, name string) object {
	t.Helper()
	b, err := os.ReadFile("../../schema/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var o object
	if err := json.Unmarshal(b, &o); err != nil {
		t.Fatal(err)
	}
	return o
}

// property returns the schema of o's named property, stepping into the element schema of an
// array.
func (o object) property(t *testing.T, name string) object {
	t.Helper()
	p, ok := o["properties"].(map[string]any)[name].(map[string]any)
	if !ok {
		t.Fatalf("schema has no property %q", name)
	}
	if items, ok := p["items"].(map[string]any); ok && p["type"] == "array" {
		if _, nested := items["properties"]; nested {
			return items
		}
	}
	return p
}

func (o object) fields() []string {
	props, _ := o["properties"].(map[string]any)
	return slices.Collect(maps.Keys(props))
}

func (o object) required() []string {
	var out []string
	list, _ := o["required"].([]any)
	for _, r := range list {
		name, _ := r.(string)
		out = append(out, name)
	}
	return out
}

// goFields returns the yaml names a struct declares and the subset tagged plan:"required".
func goFields(typ reflect.Type) (names, required []string) {
	for f := range typ.Fields() {
		name := f.Tag.Get("yaml")
		if name == "" || name == "-" {
			continue
		}
		names = append(names, name)
		if slices.Contains(strings.Split(f.Tag.Get("plan"), ","), "required") {
			required = append(required, name)
		}
	}
	return names, required
}

func sameSet(t *testing.T, what string, schema, goTypes []string) {
	t.Helper()
	s, g := slices.Sorted(slices.Values(schema)), slices.Sorted(slices.Values(goTypes))
	if !slices.Equal(s, g) {
		t.Errorf("%s: schema has %v, Go types have %v", what, s, g)
	}
}

func TestSchemasMatchGoTypes(t *testing.T) {
	bp := loadSchema(t, "blueprint.schema.json")
	section := bp.property(t, "sections")
	ol := loadSchema(t, "outline.schema.json")
	structs := []struct {
		name string
		typ  reflect.Type
		obj  object
	}{
		{"blueprint", reflect.TypeFor[Blueprint](), bp},
		{"introduction", reflect.TypeFor[plan.Introduction](), bp.property(t, "introduction")},
		{"framing", reflect.TypeFor[Framing](), bp.property(t, "framing")},
		{"exemplar", reflect.TypeFor[Exemplar](), bp.property(t, "exemplars")},
		{"section", reflect.TypeFor[Section](), section},
		{"style note", reflect.TypeFor[StyleNote](), section.property(t, "styleNotes")},
		{"disposition", reflect.TypeFor[plan.Disposition](), bp.property(t, "dispositions")},
		{"outline", reflect.TypeFor[Outline](), ol},
		{"type", reflect.TypeFor[Type](), ol.property(t, "types")},
		{"page", reflect.TypeFor[Page](), ol.property(t, "pages")},
		{"crossLink", reflect.TypeFor[CrossLink](), ol.property(t, "crossLinks")},
	}
	for _, tt := range structs {
		names, required := goFields(tt.typ)
		sameSet(t, tt.name+" fields", tt.obj.fields(), names)
		sameSet(t, tt.name+" required", tt.obj.required(), required)
	}

	defs, _ := bp["$defs"].(map[string]any)
	union := map[string]bool{}
	for kind, spec := range kinds {
		def, ok := defs["item-"+kind].(map[string]any)
		if !ok {
			t.Errorf("schema has no $defs/item-%s", kind)
			continue
		}
		o := object(def)
		fields := slices.Concat([]string{"id", "kind", "origin"}, spec.required, spec.optional)
		sameSet(t, kind+" fields", o.fields(), fields)
		sameSet(t, kind+" required", o.required(), slices.Concat([]string{"id", "kind"}, spec.required))
		for _, f := range fields {
			union[f] = true
		}
	}
	for name := range defs {
		if k, ok := strings.CutPrefix(name, "item-"); ok {
			if _, known := kinds[k]; !known {
				t.Errorf("schema defines item kind %q that the kinds table lacks", k)
			}
		}
	}
	itemNames, _ := goFields(reflect.TypeFor[Item]())
	sameSet(t, "item fields across kinds", slices.Collect(maps.Keys(union)), itemNames)
}

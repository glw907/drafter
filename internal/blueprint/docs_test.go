package blueprint

import (
	"os"
	"strings"
	"testing"
)

// propertyNames collects every property name the schema declares, at any depth.
func propertyNames(v any, into map[string]bool) {
	switch v := v.(type) {
	case map[string]any:
		if props, ok := v["properties"].(map[string]any); ok {
			for name := range props {
				into[name] = true
			}
		}
		for _, child := range v {
			propertyNames(child, into)
		}
	case []any:
		for _, child := range v {
			propertyNames(child, into)
		}
	}
}

func TestReferenceNamesEverySchemaField(t *testing.T) {
	doc, err := os.ReadFile("../../docs/blueprint-format.md")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, schema := range []string{"blueprint.schema.json", "outline.schema.json"} {
		propertyNames(map[string]any(loadSchema(t, schema)), names)
	}
	for _, lock := range []string{"page", "blueprint", "accepted", "sections"} {
		names[lock] = true
	}
	if len(names) < 50 {
		t.Fatalf("collected only %d schema fields", len(names))
	}
	for name := range names {
		if !strings.Contains(string(doc), "`"+name+"`") {
			t.Errorf("docs/blueprint-format.md never names the field `%s` in a code span", name)
		}
	}
	for kind := range kinds {
		if !strings.Contains(string(doc), "| `"+kind+"` |") {
			t.Errorf("docs/blueprint-format.md has no row for the item kind `%s`", kind)
		}
	}
}

package plan

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
	doc, err := os.ReadFile("../../docs/plan-format.md")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	propertyNames(map[string]any(loadSchema(t)), names)
	if len(names) < 30 {
		t.Fatalf("collected only %d schema fields", len(names))
	}
	for name := range names {
		if !strings.Contains(string(doc), "`"+name+"`") {
			t.Errorf("docs/plan-format.md never names the schema field `%s` in a code span", name)
		}
	}
}

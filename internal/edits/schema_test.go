package edits

import (
	"encoding/json"
	"strings"
	"testing"
)

// walk calls fn on every JSON object in v.
func walk(v any, fn func(map[string]any)) {
	switch x := v.(type) {
	case map[string]any:
		fn(x)
		for _, c := range x {
			walk(c, fn)
		}
	case []any:
		for _, c := range x {
			walk(c, fn)
		}
	}
}

func TestSchemaLimits(t *testing.T) {
	var s map[string]any
	if err := json.Unmarshal(Schema(), &s); err != nil {
		t.Fatalf("Schema is not JSON: %v", err)
	}
	banned := []string{"minLength", "maxLength", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf", "maxItems"}
	objects := 0
	walk(s, func(o map[string]any) {
		if o["type"] == "object" {
			objects++
			if o["additionalProperties"] != false {
				t.Errorf("object without additionalProperties false: %v", o)
			}
		}
		for _, k := range banned {
			if _, ok := o[k]; ok {
				t.Errorf("schema uses %s", k)
			}
		}
		if v, ok := o["minItems"]; ok && v != float64(0) && v != float64(1) {
			t.Errorf("minItems = %v, want 0 or 1", v)
		}
	})
	if objects != 3 {
		t.Errorf("schema has %d objects, want 3 (answer, edit, resolution)", objects)
	}
}

func TestSchemaStatesIndexBase(t *testing.T) {
	if !strings.Contains(string(Schema()), "0-based indexes into this answer's edits array") {
		t.Error("schema descriptions do not state the 0-based index base")
	}
}

func TestSchemaMatchesAnswer(t *testing.T) {
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(Schema(), &s); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"edits", "resolutions"} {
		if _, ok := s.Properties[k]; !ok {
			t.Errorf("schema lacks %q", k)
		}
	}
	if len(s.Required) != 2 {
		t.Errorf("required = %v, want both keys", s.Required)
	}
}

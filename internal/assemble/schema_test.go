package assemble

import (
	"encoding/json"
	"strings"
	"testing"
)

var excludedKeywords = []string{
	"minLength", "maxLength", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum",
	"multipleOf", "maxItems", "uniqueItems", "minContains", "maxContains", "pattern", "format",
}

// walk checks one schema node and everything below it, and returns its count of optional
// properties.
func walk(t *testing.T, where string, node any) int {
	t.Helper()
	optional := 0
	switch n := node.(type) {
	case map[string]any:
		for _, k := range excludedKeywords {
			if _, ok := n[k]; ok {
				t.Errorf("%s uses the excluded keyword %q", where, k)
			}
		}
		if v, ok := n["minItems"]; ok && v != 0.0 && v != 1.0 {
			t.Errorf("%s has minItems %v, want 0 or 1", where, v)
		}
		if n["type"] == "object" {
			if n["additionalProperties"] != false {
				t.Errorf("%s is an object without additionalProperties false", where)
			}
			props, _ := n["properties"].(map[string]any)
			required := map[string]bool{}
			if rs, ok := n["required"].([]any); ok {
				for _, r := range rs {
					name, _ := r.(string)
					required[name] = true
				}
			}
			for name := range props {
				if !required[name] {
					optional++
				}
			}
			for name := range required {
				if _, ok := props[name]; !ok {
					t.Errorf("%s requires %q, which it does not define", where, name)
				}
			}
		}
		for k, v := range n {
			optional += walk(t, where+"."+k, v)
		}
	case []any:
		for i, v := range n {
			optional += walk(t, where+"["+string(rune('0'+i%10))+"]", v)
		}
	}
	return optional
}

func TestSchemasKeepToStructuredOutputLimits(t *testing.T) {
	f := newFixture(t)
	for _, a := range []Answer{AnswerDraft, AnswerRead, AnswerReader1, AnswerReader2, AnswerEdits, AnswerCoverage, AnswerConsistency} {
		t.Run(string(a), func(t *testing.T) {
			in := f.input(f.config(), "p")
			in.Answer = a
			req, err := Build(t.Context(), in)
			if err != nil {
				t.Fatal(err)
			}
			var schema any
			if err := json.Unmarshal(req.Schema, &schema); err != nil {
				t.Fatalf("schema is not JSON: %v", err)
			}
			if n := walk(t, string(a), schema); n > 24 {
				t.Errorf("schema has %d optional parameters, want at most 24", n)
			}
		})
	}
}

func TestSchemaShapes(t *testing.T) {
	tests := []struct {
		answer   Answer
		required []string
		optional []string
	}{
		{AnswerDraft, []string{`"page"`, `"brief"`}, []string{"friction"}},
		{AnswerRead, []string{`"verdict"`, `"findings"`, `"summary"`, `"location"`, `"finding"`, `"rewrite"`, `"blocking"`}, []string{"friction"}},
		{AnswerReader1, []string{`"paraphrase"`, `"summary"`}, []string{"friction"}},
		{AnswerReader2, []string{`"verdict"`, `"findings"`}, []string{"friction"}},
		{AnswerConsistency, []string{`"conflicts"`, `"facts"`, `"with"`, `"conflict"`}, nil},
	}
	for _, tt := range tests {
		t.Run(string(tt.answer), func(t *testing.T) {
			schema, _, err := answerSchema(tt.answer)
			if err != nil {
				t.Fatal(err)
			}
			var top struct {
				Required []string `json:"required"`
			}
			if err := json.Unmarshal(schema, &top); err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.required {
				if !strings.Contains(string(schema), want) {
					t.Errorf("schema lacks %s", want)
				}
			}
			for _, name := range tt.optional {
				if !strings.Contains(string(schema), name) {
					t.Errorf("schema lacks %s", name)
				}
				for _, r := range top.Required {
					if r == name {
						t.Errorf("%s is required, want optional", name)
					}
				}
			}
		})
	}
}

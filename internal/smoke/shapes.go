package smoke

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Shape is one seat's request: its answer schema, effort, system prompt, and token limit.
// MaxTokens is 4096 because Haiku 5.5's thinking counts toward max_tokens.
type Shape struct {
	Seat      string
	System    string
	Prompt    string
	Effort    string
	MaxTokens int64
	Schema    json.RawMessage
}

// listField names the array each seat's synthetic answer carries beside its verdict.
var listField = []struct{ seat, field string }{
	{"draft", "sections"},
	{"structure", "findings"},
	{"editor", "edits"},
	{"fact", "claims"},
	{"reader", "questions"},
	{"coverage", "gaps"},
	{"consistency", "conflicts"},
}

// Shapes returns one synthetic shape per seat, in the seat order of pass 1. Pass 5 replaces
// these with the production builders.
func Shapes() []Shape {
	out := make([]Shape, 0, len(listField))
	for _, s := range listField {
		out = append(out, Shape{
			Seat:      s.seat,
			System:    fmt.Sprintf("You are the %s seat of a documentation pipeline. Answer only with the JSON the schema describes.", s.seat),
			Prompt:    fmt.Sprintf("Smoke test for the %s seat. Return verdict \"ok\" and an empty %s list.", s.seat, s.field),
			Effort:    "medium",
			MaxTokens: 4096,
			Schema:    schemaFor(s.field),
		})
	}
	return out
}

func schemaFor(field string) json.RawMessage {
	item := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"where": map[string]any{"type": "string"},
			"note":  map[string]any{"type": "string"},
		},
		"required":             []string{"where", "note"},
		"additionalProperties": false,
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"verdict": map[string]any{"type": "string", "enum": []string{"ok", "revise"}},
			field:     map[string]any{"type": "array", "items": item},
		},
		"required":             []string{"verdict", field},
		"additionalProperties": false,
	}
	b, err := json.Marshal(schema)
	if err != nil {
		panic(err) // the schema is built from literals
	}
	return b
}

// Check reports whether text is JSON that satisfies s's schema. It understands the keywords the
// shapes use: type, enum, required, properties, additionalProperties false, and items.
func Check(s Shape, text string) error {
	var schema map[string]any
	if err := json.Unmarshal(s.Schema, &schema); err != nil {
		return fmt.Errorf("schema of %s: %w", s.Seat, err)
	}
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return fmt.Errorf("%s answer is not JSON: %w", s.Seat, err)
	}
	if err := validate(schema, v, "$"); err != nil {
		return fmt.Errorf("%s answer: %w", s.Seat, err)
	}
	return nil
}

func validate(schema map[string]any, v any, path string) error {
	if enum, ok := schema["enum"].([]any); ok {
		if slices.Contains(enum, v) {
			return nil
		}
		return fmt.Errorf("%s is %v, not one of %v", path, v, enum)
	}
	switch schema["type"] {
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("%s is not an object", path)
		}
		props, _ := schema["properties"].(map[string]any)
		if schema["additionalProperties"] == false {
			for k := range obj {
				if _, ok := props[k]; !ok {
					return fmt.Errorf("%s has unexpected property %q", path, k)
				}
			}
		}
		req, _ := schema["required"].([]any)
		for _, r := range req {
			if k, _ := r.(string); obj[k] == nil {
				return fmt.Errorf("%s lacks required property %q", path, k)
			}
		}
		for k, sub := range props {
			val, present := obj[k]
			if !present {
				continue
			}
			ss, _ := sub.(map[string]any)
			if err := validate(ss, val, path+"."+k); err != nil {
				return err
			}
		}
	case "array":
		arr, ok := v.([]any)
		if !ok {
			return fmt.Errorf("%s is not an array", path)
		}
		items, _ := schema["items"].(map[string]any)
		for i, el := range arr {
			if err := validate(items, el, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case "string":
		if _, ok := v.(string); !ok {
			return fmt.Errorf("%s is not a string", path)
		}
	case "integer", "number":
		if _, ok := v.(float64); !ok {
			return fmt.Errorf("%s is not a number", path)
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("%s is not a boolean", path)
		}
	default:
		return fmt.Errorf("%s: schema type %v is not supported", path, strings.TrimSpace(fmt.Sprint(schema["type"])))
	}
	return nil
}

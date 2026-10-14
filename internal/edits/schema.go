package edits

import "encoding/json"

// Schema returns the answer's JSON Schema, sent as structured output. It
// keeps to the structured-output limits: every object closes with
// additionalProperties false and no length, range, or array-size keyword
// appears.
func Schema() json.RawMessage {
	str := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	obj := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{
			"type":                 "object",
			"properties":           props,
			"required":             required,
			"additionalProperties": false,
		}
	}
	edit := obj(map[string]any{
		"file": map[string]any{
			"type":        "string",
			"enum":        []string{FilePage, FileBrief},
			"description": "The file to edit: page or brief.",
		},
		"old": str("The exact span to replace. It must occur exactly once in the file as the earlier edits in this answer left it."),
		"new": str("The replacement text. It must differ from old."),
	}, "file", "old", "new")
	resolution := obj(map[string]any{
		"finding": map[string]any{
			"type":        "integer",
			"description": "The finding's number.",
		},
		"status": map[string]any{
			"type":        "string",
			"enum":        []string{StatusEdited, StatusDeclined},
			"description": "edited when edits address the finding; declined when it needs no change.",
		},
		"edits": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "integer"},
			"description": "0-based indexes into this answer's edits array that address the finding. The first edit is 0.",
		},
		"reason": str("Why the finding was declined, or how the edits address it."),
	}, "finding", "status", "edits", "reason")
	s := obj(map[string]any{
		"edits": map[string]any{
			"type":        "array",
			"items":       edit,
			"description": "Exact edits, applied in order. Every edit must be claimed by a resolution.",
		},
		"resolutions": map[string]any{
			"type":        "array",
			"items":       resolution,
			"description": "Exactly one resolution per numbered finding, by its number. Edit indexes are 0-based into this answer's edits.",
		},
	}, "edits", "resolutions")
	b, err := json.Marshal(s)
	if err != nil {
		panic("edits: schema does not marshal: " + err.Error())
	}
	return b
}

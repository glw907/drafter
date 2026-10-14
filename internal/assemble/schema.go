package assemble

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/glw907/drafter/internal/coverage"
	"github.com/glw907/drafter/internal/edits"
)

// answerSchema returns the JSON Schema of the answer and the contract text that closes the
// request. Every schema keeps to the structured-output limits: each object closes with
// additionalProperties false, and no length, range, or array-size keyword appears.
func answerSchema(a Answer) (json.RawMessage, string, error) {
	switch a {
	case AnswerDraft:
		return marshal(draftSchema()), "Answer with the page as Markdown in page, its sentence-to-fact brief as a JSON string in brief, and any friction you met, a list of short notes, in friction.", nil
	case AnswerRead, AnswerReader2:
		return marshal(readSchema()), "Answer with a verdict, each finding with its location, the finding, a rewrite, and whether it blocks, a summary, and any friction you met in friction.", nil
	case AnswerReader1:
		return marshal(reader1Schema()), "Answer with your paraphrase of the page, a summary, and any friction you met in friction.", nil
	case AnswerEdits:
		return edits.Schema(), edits.Instructions(), nil
	case AnswerCoverage:
		return coverage.Schema(), "Answer with one entry per listed item: its id and the exact quote, empty when the page does not carry it.", nil
	case AnswerConsistency:
		return marshal(consistencySchema()), "Answer with each conflict you find, or an empty list.", nil
	}
	return nil, "", fmt.Errorf("assemble: unknown answer %q", a)
}

func marshal(v map[string]any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("assemble: schema does not marshal: " + err.Error())
	}
	return b
}

func str(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func strList(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

// obj closes an object over props. Every property is required except the names in optional.
func obj(props map[string]any, optional ...string) map[string]any {
	var required []string
	for name := range props {
		isOptional := false
		for _, o := range optional {
			isOptional = isOptional || o == name
		}
		if !isOptional {
			required = append(required, name)
		}
	}
	slices.Sort(required)
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}
}

const frictionDesc = "Optional short notes on anything in the task that did not work or was missing."

func draftSchema() map[string]any {
	return obj(map[string]any{
		"page":     str("The page, as Markdown."),
		"brief":    str("The sentence-to-fact brief, as a string holding JSON."),
		"friction": strList(frictionDesc),
	}, "friction")
}

func readSchema() map[string]any {
	finding := obj(map[string]any{
		"location": str("Where on the page the finding applies."),
		"finding":  str("What is wrong."),
		"rewrite":  str("The rewrite that fixes it."),
		"blocking": map[string]any{"type": "boolean", "description": "Whether the page must not ship until this is fixed."},
	})
	return obj(map[string]any{
		"verdict": map[string]any{"type": "string", "enum": []string{"ok", "fix"}, "description": "ok when the page needs no change; fix otherwise."},
		"findings": map[string]any{
			"type": "array", "items": finding, "description": "Every finding, blocking or advisory.",
		},
		"summary":  str("A short summary of the read."),
		"friction": strList(frictionDesc),
	}, "friction")
}

func reader1Schema() map[string]any {
	return obj(map[string]any{
		"paraphrase": str("The reader's paraphrase of the page."),
		"summary":    str("A short summary of what the reader took from the page."),
		"friction":   strList(frictionDesc),
	}, "friction")
}

func consistencySchema() map[string]any {
	conflict := obj(map[string]any{
		"facts":    strList("The ids of the placed facts that conflict."),
		"with":     str("The fact id, or page#heading, they conflict with."),
		"conflict": str("What conflicts."),
	})
	return obj(map[string]any{
		"conflicts": map[string]any{"type": "array", "items": conflict, "description": "Every conflict found."},
	})
}

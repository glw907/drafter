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
		return marshal(readSchema(true)), reviewContract, nil
	case AnswerBlueprint:
		return marshal(readSchema(false)), "Answer with a verdict, each finding with its location, the finding, a rewrite, and whether it blocks, a summary, and any friction you met in friction.", nil
	case AnswerReader1:
		return marshal(reader1Schema()), "Answer with your paraphrase of the page, a summary, and any friction you met in friction.", nil
	case AnswerEdits:
		return edits.Schema(), edits.Instructions(), nil
	case AnswerCoverage:
		return coverage.Schema(), "Answer with one entry per listed item: its id and the exact quote, empty when the page does not carry it.", nil
	case AnswerConsistency:
		return marshal(consistencySchema()), "Answer with each conflict you find, or an empty list.", nil
	case AnswerFactTruth:
		return marshal(factTruthSchema()), "Answer with one entry for each listed fact: its id, its verdict, the excerpt lines a verdict other than holds rests on, and the claim the code supports in fix. Leave excerpt and fix empty for holds.", nil
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

// kinds are the values a review finding's kind takes. The first four are the small kinds, which
// the pipeline lets one scoped fix answer without another review.
var kinds = []string{"link", "handoff", "heading", "citation", "fact", "claim", "code", "voice", "step", "placement", "other"}

// reviewContract closes a review request. It defines each kind in one clause, since the schema
// carries only the enum.
const reviewContract = "Answer with a verdict, each finding with its location, the finding, a rewrite, whether it blocks, and its kind, a summary, and any friction you met in friction. " +
	"A finding's kind is one of these: link, a link that is missing, broken, or points to the wrong place; " +
	"handoff, a pointer to the next page or task that is missing or wrong; " +
	"heading, a section heading whose wording or level is wrong; " +
	"citation, a brief sentence that is missing from the page or cites the wrong fact; " +
	"fact, a fact the page states wrongly or leaves out; " +
	"claim, a claim the page cannot support or overstates; " +
	"code, a code sample that is wrong or missing; " +
	"voice, the tone, voice, or word choice of the prose; " +
	"step, a step that is missing, extra, or out of order; " +
	"placement, content in the wrong section, or a section that is missing; " +
	"other, anything the kinds above do not name."

// readSchema is the schema of a read's verdict. The blueprint read's findings carry no kind, so
// its request bytes stay as they were before kinds existed.
func readSchema(withKind bool) map[string]any {
	props := map[string]any{
		"location": str("Where on the page the finding applies."),
		"finding":  str("What is wrong."),
		"rewrite":  str("The rewrite that fixes it."),
		"blocking": map[string]any{"type": "boolean", "description": "Whether the page must not ship until this is fixed."},
	}
	if withKind {
		props["kind"] = map[string]any{"type": "string", "enum": kinds, "description": "What the finding is about, one of the kinds the closing instructions define."}
	}
	finding := obj(props)
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

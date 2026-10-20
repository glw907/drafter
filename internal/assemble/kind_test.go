package assemble

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/config"
)

func TestReviewFindingKind(t *testing.T) {
	want := []string{"link", "handoff", "heading", "citation", "fact", "claim", "code", "voice", "step", "placement", "other"}
	tests := []struct {
		answer   Answer
		wantKind bool
	}{
		{AnswerRead, true},
		{AnswerReader2, true},
		{AnswerBlueprint, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.answer), func(t *testing.T) {
			schema, contract, err := answerSchema(tt.answer)
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				Properties struct {
					Findings struct {
						Items struct {
							Required   []string `json:"required"`
							Properties struct {
								Kind *struct {
									Type string   `json:"type"`
									Enum []string `json:"enum"`
								} `json:"kind"`
							} `json:"properties"`
						} `json:"items"`
					} `json:"findings"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(schema, &doc); err != nil {
				t.Fatal(err)
			}
			items := doc.Properties.Findings.Items
			if !tt.wantKind {
				if items.Properties.Kind != nil || strings.Contains(contract, "kind") {
					t.Errorf("the %s answer carries a kind: schema %s, contract %q", tt.answer, schema, contract)
				}
				return
			}
			kind := items.Properties.Kind
			if kind == nil || kind.Type != "string" || !slices.Equal(kind.Enum, want) {
				t.Fatalf("kind = %+v, want a string enum of exactly %v", kind, want)
			}
			if !slices.Contains(items.Required, "kind") {
				t.Errorf("finding required = %v, want kind among them", items.Required)
			}
			for _, k := range want {
				if !strings.Contains(contract, k+", ") {
					t.Errorf("contract does not define %q:\n%s", k, contract)
				}
			}
			if strings.Contains(strings.ToLower(contract), "register") {
				t.Errorf("contract names the register:\n%s", contract)
			}
		})
	}
}

// The blueprint read's request keeps the bytes it had before review findings carried a kind. The
// pipeline's and the CLI's request goldens pin the whole request; this pins the two parts a kind
// change would move.
func TestBlueprintAnswerKeepsTheReadShape(t *testing.T) {
	schema, contract, err := answerSchema(AnswerBlueprint)
	if err != nil {
		t.Fatal(err)
	}
	const wantContract = "Answer with a verdict, each finding with its location, the finding, a rewrite, and whether it blocks, a summary, and any friction you met in friction."
	if contract != wantContract {
		t.Errorf("contract = %q, want %q", contract, wantContract)
	}
	const wantSchema = `{"additionalProperties":false,"properties":{"findings":{"description":"Every finding, blocking or advisory.","items":{"additionalProperties":false,"properties":{"blocking":{"description":"Whether the page must not ship until this is fixed.","type":"boolean"},"finding":{"description":"What is wrong.","type":"string"},"location":{"description":"Where on the page the finding applies.","type":"string"},"rewrite":{"description":"The rewrite that fixes it.","type":"string"}},"required":["blocking","finding","location","rewrite"],"type":"object"},"type":"array"},"friction":{"description":"Optional short notes on anything in the task that did not work or was missing.","items":{"type":"string"},"type":"array"},"summary":{"description":"A short summary of the read.","type":"string"},"verdict":{"description":"ok when the page needs no change; fix otherwise.","enum":["ok","fix"],"type":"string"}},"required":["findings","summary","verdict"],"type":"object"}`
	if string(schema) != wantSchema {
		t.Errorf("schema = %s\nwant %s", schema, wantSchema)
	}
}

func TestBlueprintSeatInputsCarryTheirCommandBlock(t *testing.T) {
	f := newFixture(t)
	c := f.config()
	c.Seat["blueprint"] = config.Seat{Model: "m", Inputs: []string{"echo BLUEPRINT-INPUT"}}
	in := f.input(c, "go")
	in.Seat, in.Answer = "blueprint", AnswerBlueprint
	got := requestText(t, in)
	if !strings.Contains(got, "=== command: echo BLUEPRINT-INPUT ===\nBLUEPRINT-INPUT\n=== end command") {
		t.Errorf("the blueprint read lacks its seat's command block:\n%s", got)
	}
}

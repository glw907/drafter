package coverage

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/glw907/drafter/internal/plan"
)

// listed is a plan item the coverage read answers: a claim sentence, a handoff, or a reason.
type listed struct {
	item    plan.Item
	section int
}

func (l listed) kind() string {
	if l.item.Kind == "sentence" {
		return l.item.Mode
	}
	return l.item.Kind
}

func listedItems(p *plan.Plan) []listed {
	var out []listed
	for i, s := range p.Sections {
		for _, it := range s.Items {
			if it.Kind == "handoff" || it.Kind == "reason" || (it.Kind == "sentence" && it.Mode == "claim") {
				out = append(out, listed{it, i})
			}
		}
	}
	return out
}

// answer is the coverage read's reply.
type answer struct {
	Items []struct {
		ID    string `json:"id"`
		Quote string `json:"quote"`
	} `json:"items"`
}

// Prompt returns the coverage read's request text: every claim sentence, handoff, and reason item
// of p with its id, section heading, and text, followed by page.
func Prompt(p *plan.Plan, page []byte) string {
	var b strings.Builder
	b.WriteString("Check a documentation page against its plan. Each item below must be carried by the page, in the section named.\n\n")
	b.WriteString("For each item, quote the passage of the page that carries it, copied exactly as the page has it and taken from that item's own section. ")
	b.WriteString("If the page doesn't carry the item, give an empty quote. Answer every item, by id.\n\nItems:\n")
	for _, l := range listedItems(p) {
		fmt.Fprintf(&b, "\n- id: %s\n  section: %s\n  kind: %s\n  text: %s\n",
			l.item.ID, p.Sections[l.section].Heading, l.kind(), strings.Join(strings.Fields(l.item.Text), " "))
	}
	b.WriteString("\nPage:\n\n")
	b.Write(page)
	return b.String()
}

// Schema returns the JSON Schema of the coverage read's answer, within the structured-output
// limits: every object closes with additionalProperties false and lists all its properties.
func Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "items": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "quote": {"type": "string"}
        },
        "required": ["id", "quote"],
        "additionalProperties": false
      }
    }
  },
  "required": ["items"],
  "additionalProperties": false
}`)
}

// Verify checks the coverage read's answer against p and page. A listed item the answer leaves
// out or answers with an empty quote is a miss. A quote that doesn't occur, after normalization,
// inside its item's section is a miss and a fault; the fault says where else on the page the
// quote is, when it is. The error is an answer that isn't the expected JSON.
func Verify(p *plan.Plan, page, answerJSON []byte) (misses []Miss, faults []string, err error) {
	var a answer
	if err := json.Unmarshal(answerJSON, &a); err != nil {
		return nil, nil, fmt.Errorf("coverage answer is not valid: %w", err)
	}
	quotes := map[string]string{}
	for _, it := range a.Items {
		if _, dup := quotes[it.ID]; !dup {
			quotes[it.ID] = it.Quote
		}
	}
	preamble, regions := split(p, page)

	for _, l := range listedItems(p) {
		r := regions[l.section]
		miss := Miss{Item: l.item.ID, Section: r.heading, Kind: l.kind()}
		quote := Normalize(quotes[l.item.ID])
		switch {
		case !r.found:
			miss.Reason = reasonNoHeading
		case quote == "":
			miss.Reason = "the coverage read found no passage for this item"
		case strings.Contains(r.norm, quote):
			continue
		default:
			miss.Reason = "the quoted passage is not in this section"
			fault := fmt.Sprintf("coverage quote for %s is not in section %q", l.item.ID, r.heading)
			if where, ok := elsewhere(preamble, regions, quote); ok {
				fault += " (it is in " + where + ")"
			}
			faults = append(faults, fault)
		}
		misses = append(misses, miss)
	}
	return misses, faults, nil
}

func elsewhere(preamble region, regions []region, quote string) (string, bool) {
	for _, r := range regions {
		if r.found && strings.Contains(r.norm, quote) {
			return fmt.Sprintf("section %q", r.heading), true
		}
	}
	if strings.Contains(preamble.norm, quote) {
		return "the text above the first planned section", true
	}
	return "", false
}

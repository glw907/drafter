package coverage

import (
	"encoding/json"
	"errors"
	"fmt"
)

// noClaim is the id a brief sentence carries when it cites no fact.
const noClaim = "no-claim"

// Brief is the sentence-to-fact brief the drafter writes beside a page. Fields beyond the
// sentences, such as a cairn brief's page, cuts, and local, are ignored.
type Brief struct {
	Sentences []Sentence `json:"sentences"`
}

// Sentence is one page sentence and the fact ids it cites.
type Sentence struct {
	Text string `json:"text"`
	ID   IDs    `json:"id"`
}

// IDs is a sentence's cited fact ids. It decodes from a single string or a list of strings, and
// drops "no-claim", so a sentence that cites nothing holds no ids.
type IDs []string

// UnmarshalJSON decodes a string or a list of strings.
func (ids *IDs) UnmarshalJSON(data []byte) error {
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*ids = keepFacts([]string{one})
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return errors.New("id must be a string or a list of strings")
	}
	*ids = keepFacts(many)
	return nil
}

func keepFacts(in []string) IDs {
	var out IDs
	for _, id := range in {
		if id != noClaim {
			out = append(out, id)
		}
	}
	return out
}

func parseBrief(data []byte) (*Brief, error) {
	var b Brief
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("brief is not valid JSON: %w", err)
	}
	return &b, nil
}

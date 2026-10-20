// Package edits is the edit contract: the shape of a redraft or gate-fix
// answer, the schema and instructions that ask for it, and the check that
// applies its edits or refuses it with problems the model can act on.
package edits

// Finding is one numbered item a seat or the gate raised against a page.
// N is unique within a page's exchange. Kind is the review answer's label for
// what the finding is about, empty for a gate finding and for a finding saved
// before kinds existed.
type Finding struct {
	N        int
	Seat     string
	Location string
	Text     string
	Rewrite  string
	Blocking bool
	Kind     string
}

// Edit replaces one span of a file. File is "page" or "brief".
type Edit struct {
	File string `json:"file"`
	Old  string `json:"old"`
	New  string `json:"new"`
}

// Resolution records what an answer did about one finding. Status is
// "edited" or "declined". Edits holds 0-based indexes into the same
// answer's edits.
type Resolution struct {
	Finding int    `json:"finding"`
	Status  string `json:"status"`
	Edits   []int  `json:"edits"`
	Reason  string `json:"reason"`
}

// Answer is the structured output of a redraft or gate-fix turn.
type Answer struct {
	Edits       []Edit       `json:"edits"`
	Resolutions []Resolution `json:"resolutions"`
}

// Bound limits an answer to some sections of a page. Headings are the page's section headings in
// order, which cut the page as coverage.Spans cuts it, and Allowed holds the headings of the
// sections the answer may change. A nil *Bound limits nothing.
type Bound struct {
	Headings []string
	Allowed  []string
}

// Status values a Resolution takes.
const (
	StatusEdited   = "edited"
	StatusDeclined = "declined"
)

// File values an Edit takes.
const (
	FilePage  = "page"
	FileBrief = "brief"
)

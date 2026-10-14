// Package plan decodes a page plan, a <page>.plan.yaml file, and checks it.
//
// A plan is the planner's contract with the drafter: the page's argument, its sections in order,
// and the items each section must carry. Decode turns the YAML into typed values and reports
// every schema fault with its line. The struct tags are the schema's source of truth:
// schema/plan.schema.json must list the same fields and required sets, and a test enforces it.
package plan

// Version is the plan format version this package decodes.
const Version = 1

// Plan is a decoded plan file.
//
// Fields tagged plan:"required" must appear, and a required list must hold an entry unless it
// is also tagged plan:"empty", which leaves the emptiness to a later rule. Fields
// tagged plan:"prose" are written as a sentence or more, so a plain-style value that carries a
// line comment is refused: YAML would silently cut the value at the comment.
type Plan struct {
	Version       int           `yaml:"version" plan:"required"`
	Page          Page          `yaml:"page" plan:"required"`
	Argument      string        `yaml:"argument" plan:"required,prose"`
	Introduction  Introduction  `yaml:"introduction" plan:"required"`
	OrderReason   string        `yaml:"orderReason" plan:"required,prose"`
	Preconditions []string      `yaml:"preconditions"`
	Sections      []Section     `yaml:"sections" plan:"required"`
	Dispositions  []Disposition `yaml:"dispositions"`
	Constraints   []string      `yaml:"constraints" plan:"prose"`
	Friction      []string      `yaml:"friction" plan:"prose"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// Page names the page a plan is for.
type Page struct {
	Path  string `yaml:"path" plan:"required"`
	Title string `yaml:"title" plan:"required,prose"`
	Type  string `yaml:"type" plan:"required"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// Introduction holds the three parts Google's developer documentation style gives a page's
// opening: what it covers, what the reader should already know, and what it leaves out.
type Introduction struct {
	Covers         string   `yaml:"covers" plan:"required,prose"`
	PriorKnowledge string   `yaml:"priorKnowledge" plan:"required,prose"`
	NotCovered     string   `yaml:"notCovered" plan:"required,prose"`
	Facts          []string `yaml:"facts"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// Section is one page section, in page order. Level is the heading level, 2 or 3; Decode sets
// it to 2 when the plan omits it. Anchor, when set, is the heading's slug as the plan expects the
// rendered page to carry it. Items must be present, and is empty only for a level-2 section
// that the next section's level-3 headings fill.
type Section struct {
	Heading string `yaml:"heading" plan:"required,prose"`
	Level   int    `yaml:"level"`
	Anchor  string `yaml:"anchor"`
	Opens   string `yaml:"opens" plan:"required,prose"`
	Items   []Item `yaml:"items" plan:"required,empty"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// Item is one pinned element of a section. Kind selects which of the other fields apply:
// the kinds table in decode.go lists the fields each kind requires and allows, and the
// decoder refuses any other.
type Item struct {
	ID        string   `yaml:"id" plan:"required"`
	Kind      string   `yaml:"kind" plan:"required"`
	Fact      string   `yaml:"fact"`
	Use       string   `yaml:"use" plan:"prose"`
	Mode      string   `yaml:"mode"`
	Text      string   `yaml:"text" plan:"prose"`
	Facts     []string `yaml:"facts"`
	Source    string   `yaml:"source"`
	Target    string   `yaml:"target"`
	Requires  []string `yaml:"requires"`
	Produces  []string `yaml:"produces"`
	CanFail   bool     `yaml:"canFail"`
	Optional  bool     `yaml:"optional"`
	From      string   `yaml:"from"`
	Symptom   string   `yaml:"symptom" plan:"prose"`
	Cause     string   `yaml:"cause" plan:"prose"`
	HandledBy string   `yaml:"handledBy"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// Disposition records a fact the plan leaves out of the page body: subordinated to a link
// target, or cut.
type Disposition struct {
	Fact        string `yaml:"fact" plan:"required"`
	Disposition string `yaml:"disposition" plan:"required"`
	Reference   string `yaml:"reference"`
	Reason      string `yaml:"reason" plan:"required,prose"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// Finding severities.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// Finding is one problem a decode or a check found, addressed to a line of a file.
type Finding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Check    string `json:"check"`
	Message  string `json:"message"`
	Fix      string `json:"fix"`
}

// HasError reports whether any finding is an error.
func HasError(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Package blueprint decodes the files of a documentation project's blueprint tree: a page's
// <page>.blueprint.yaml, the doc set's outline.yaml, and the drafted.lock that records what
// drafter last wrote.
//
// A blueprint is the page plan made durable. It starts from plan format v1 and adds a stable id
// and a reason to each section, a framing block, exemplars, an example item kind, and the marks
// that carry a human edit. The struct tags are the schema's source of truth: schema/blueprint.schema.json
// and schema/outline.schema.json must list the same fields and required sets, and a test enforces it.
package blueprint

import "github.com/glw907/drafter/internal/plan"

// Version is the blueprint and outline format version this package decodes.
const Version = 1

// Blueprint is a decoded blueprint file. The page's job, type, title, and path are not here: they
// live in the page's outline entry and the path rule, so no fact has two homes.
//
// The plan tags mean what they do on plan.Plan.
type Blueprint struct {
	Version       int                `yaml:"version" plan:"required"`
	Argument      string             `yaml:"argument" plan:"required,prose"`
	Introduction  plan.Introduction  `yaml:"introduction" plan:"required"`
	Framing       Framing            `yaml:"framing"`
	OrderReason   string             `yaml:"orderReason" plan:"required,prose"`
	Preconditions []string           `yaml:"preconditions"`
	Exemplars     []Exemplar         `yaml:"exemplars"`
	Sections      []Section          `yaml:"sections" plan:"required"`
	Dispositions  []plan.Disposition `yaml:"dispositions"`
	Constraints   []string           `yaml:"constraints" plan:"prose"`
	Review        string             `yaml:"review"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// Framing holds the four descriptive parts of the framing record. A part cites a fact as a
// backticked id.
type Framing struct {
	Readers       string `yaml:"readers" plan:"prose"`
	Background    string `yaml:"background" plan:"prose"`
	PlaceInDocSet string `yaml:"placeInDocSet" plan:"prose"`
	IntroPlan     string `yaml:"introPlan" plan:"prose"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// Exemplar names a published page to imitate and what to take from it, in the shape cairn's JSON
// outlines give them.
type Exemplar struct {
	Source       string `yaml:"source" plan:"required"`
	Slice        string `yaml:"slice"`
	ManifestType string `yaml:"manifestType"`
	Take         string `yaml:"take" plan:"required,prose"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// Section is one page section, in page order. ID is stable across heading changes, and Why says
// why the section is here and why it sits where it does. A check requires a Why; the decoder does
// not, so a converted blueprint decodes and the check names what is left to write.
type Section struct {
	ID         string      `yaml:"id" plan:"required"`
	Heading    string      `yaml:"heading" plan:"required,prose"`
	Level      int         `yaml:"level"`
	Anchor     string      `yaml:"anchor"`
	Why        string      `yaml:"why" plan:"prose"`
	Opens      string      `yaml:"opens" plan:"required,prose"`
	Items      []Item      `yaml:"items" plan:"required,empty"`
	StyleNotes []StyleNote `yaml:"styleNotes"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// StyleNote records a human's style-only edit: the wording drafter wrote and the wording that
// replaced it.
type StyleNote struct {
	Old string `yaml:"old" plan:"required,prose"`
	New string `yaml:"new" plan:"required,prose"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// Item is one pinned element of a section. Kind selects which of the other fields apply, as the
// kinds table in decode.go lists. Origin is "human" for an item a person's edit wrote back.
type Item struct {
	ID        string   `yaml:"id" plan:"required"`
	Kind      string   `yaml:"kind" plan:"required"`
	Origin    string   `yaml:"origin"`
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

// Outline is a decoded outline.yaml: the doc set's page types and pages.
type Outline struct {
	Version    int         `yaml:"version" plan:"required"`
	Types      []Type      `yaml:"types" plan:"required"`
	Pages      []Page      `yaml:"pages" plan:"required"`
	CrossLinks []CrossLink `yaml:"crossLinks"`
	Tracks     []Track     `yaml:"tracks"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// Track groups pages for one reader. Reader is the reader profile in prose, Standard names the
// published style standard the track follows (only "google" ships), Paths are globs over page
// paths in which a "**" segment matches any number of segments, and Register is the optional
// tracked path of a project register that adds anatomy entries beyond the stock ones.
type Track struct {
	Name     string   `yaml:"name" plan:"required"`
	Reader   string   `yaml:"reader" plan:"required,prose"`
	Standard string   `yaml:"standard" plan:"required"`
	Paths    []string `yaml:"paths" plan:"required"`
	Register string   `yaml:"register"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// Type declares a page type and the anatomy it follows. Anatomy is a stock anatomy name, such as
// "Task guide", or the name of an entry in the register of the track that holds the type's pages.
// A check requires one; the converter leaves it for a person to name.
type Type struct {
	Name    string `yaml:"name" plan:"required"`
	Anatomy string `yaml:"anatomy"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// Page is one outline entry. A page with Managed false needs only Path and has no blueprint;
// Decode sets Managed to true when the entry omits it.
type Page struct {
	Path       string   `yaml:"path" plan:"required"`
	Slug       string   `yaml:"slug"`
	Title      string   `yaml:"title" plan:"prose"`
	Type       string   `yaml:"type"`
	Job        string   `yaml:"job" plan:"prose"`
	Covers     []string `yaml:"covers" plan:"prose"`
	OutOfScope []string `yaml:"outOfScope" plan:"prose"`
	Figure     bool     `yaml:"figure"`
	Managed    bool     `yaml:"managed"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// CrossLink asks that the page From link to the page To, naming each by slug or path.
type CrossLink struct {
	From string `yaml:"from" plan:"required"`
	To   string `yaml:"to" plan:"required"`
	Why  string `yaml:"why" plan:"required,prose"`

	Line  int            `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// Lock is a decoded drafted.lock, written only by drafter. Pages is keyed on the page path.
type Lock struct {
	Version int                  `yaml:"version"`
	Pages   map[string]LockEntry `yaml:"pages,omitempty"`
}

// LockEntry is what drafter recorded for one page. Page and Blueprint are git blob ids of the
// two files at drafter's last write, Sections maps each section id to a sha256 of its text (the
// key _intro holds the text above the first section), and Accepted is the blob id of the
// blueprint the blueprint read last accepted. An entry with only Accepted set is a page not
// drafted yet.
//
// Facts maps each fact id in the blueprint's read set to the hash [FactsHashes] gives its bullets
// at the read that last judged them, and is present only when a fact-truth read judged every bullet
// in it. A read compares it with the current read set to find the facts that changed since the
// accept.
//
// Blocked is the read bar: the outcome of the read that last judged the whole blueprint and
// blocked it. A later read judges the change since Blocked.Commit against its findings, and an
// accept clears it. An entry with only Blocked set is a page whose blueprint has never been
// accepted or drafted.
type LockEntry struct {
	Page      string            `yaml:"page,omitempty"`
	Blueprint string            `yaml:"blueprint,omitempty"`
	Accepted  string            `yaml:"accepted,omitempty"`
	Facts     map[string]string `yaml:"facts,omitempty"`
	Sections  map[string]string `yaml:"sections,omitempty"`
	Blocked   *Blocked          `yaml:"blocked,omitempty"`
}

// Blocked is a blocked whole read as the lock keeps it: the full sha of the commit the read read,
// and its blocking findings in the order the read gave them.
type Blocked struct {
	Commit   string           `yaml:"commit"`
	Findings []BlockedFinding `yaml:"findings"`
}

// BlockedFinding is one blocking finding of a blocked read. Seat names the read seat that raised
// it and Kind its kind. A bar none of whose findings has a seat was written before the
// fact-truth seat existed.
type BlockedFinding struct {
	Location string `yaml:"location"`
	Text     string `yaml:"text"`
	Seat     string `yaml:"seat,omitempty"`
	Kind     string `yaml:"kind,omitempty"`
}

package blueprint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v3"

	"github.com/glw907/drafter/internal/plan"
)

// NamedData is a file's name, for messages, and its bytes.
type NamedData struct {
	Name string
	Data []byte
}

// JSONOutline is the chain's JSON outline: the pages and cross-links of one or several files,
// merged in file order. It reads only the keys the blueprint format keeps and ignores the rest.
type JSONOutline struct {
	pages      []jsonPage
	crossLinks []CrossLink
}

type jsonPage struct {
	Path       string   `json:"path"`
	Slug       string   `json:"slug"`
	Title      string   `json:"title"`
	PageType   string   `json:"pageType"`
	Job        string   `json:"job"`
	Covers     []string `json:"covers"`
	OutOfScope []string `json:"outOfScope"`
	Figure     bool     `json:"figure"`
	Exemplars  []struct {
		Source       string `json:"source"`
		Slice        string `json:"slice"`
		ManifestType string `json:"manifestType"`
		Take         string `json:"take"`
	} `json:"exemplars"`
}

// ParseJSONOutlines reads each JSON outline and merges them. A file that is not JSON, or that has
// no pages array, is an error naming the file.
func ParseJSONOutlines(files []NamedData) (*JSONOutline, error) {
	jo := new(JSONOutline)
	for _, f := range files {
		var doc struct {
			Pages      *[]jsonPage `json:"pages"`
			CrossLinks []struct {
				From string `json:"from"`
				To   string `json:"to"`
				Why  string `json:"why"`
			} `json:"crossLinks"`
		}
		if err := json.Unmarshal(f.Data, &doc); err != nil {
			return nil, fmt.Errorf("outline %s: %w", f.Name, err)
		}
		if doc.Pages == nil {
			return nil, fmt.Errorf("outline %s has no pages array", f.Name)
		}
		jo.pages = append(jo.pages, *doc.Pages...)
		for _, c := range doc.CrossLinks {
			jo.crossLinks = append(jo.crossLinks, CrossLink{From: c.From, To: c.To, Why: c.Why})
		}
	}
	return jo, nil
}

// Outline returns the merged outline. It declares each distinct page type once, in order of first
// use, with no anatomy: naming the register entry a type follows is a person's call, and the check
// reports the gap.
func (jo *JSONOutline) Outline() *Outline {
	o := &Outline{Version: Version, CrossLinks: jo.crossLinks}
	seen := map[string]bool{}
	for _, p := range jo.pages {
		if p.PageType != "" && !seen[p.PageType] {
			seen[p.PageType] = true
			o.Types = append(o.Types, Type{Name: p.PageType})
		}
		// Managed stays false only to be left out of the file: an omitted managed means true.
		var covers []Cover
		for _, text := range p.Covers {
			covers = append(covers, Cover{Text: text})
		}
		o.Pages = append(o.Pages, Page{Path: p.Path, Slug: p.Slug, Title: p.Title, Type: p.PageType,
			Job: p.Job, Covers: covers, OutOfScope: p.OutOfScope, Figure: p.Figure})
	}
	return o
}

// Exemplars returns the exemplars of the page at pagePath, and whether the outline lists the page.
func (jo *JSONOutline) Exemplars(pagePath string) ([]Exemplar, bool) {
	for _, p := range jo.pages {
		if p.Path != pagePath {
			continue
		}
		var out []Exemplar
		for _, e := range p.Exemplars {
			out = append(out, Exemplar{Source: e.Source, Slice: e.Slice, ManifestType: e.ManifestType, Take: e.Take})
		}
		return out, true
	}
	return nil, false
}

// ConvertOutline returns the outline.yaml for the JSON outlines in files. The result decodes with
// DecodeOutline; an input the decoder would refuse, such as a page listed twice, is an error here.
func ConvertOutline(files []NamedData) ([]byte, error) {
	jo, err := ParseJSONOutlines(files)
	if err != nil {
		return nil, err
	}
	data, err := marshalNode(jo.Outline())
	if err != nil {
		return nil, err
	}
	if _, findings := DecodeOutline("outline.yaml", data); plan.HasError(findings) {
		return nil, fmt.Errorf("the converted outline does not decode: %s", firstError(findings))
	}
	return data, nil
}

// PlanInput is what ConvertPlan maps: a decoded v1 plan, the framing record's text (nil for none),
// and the exemplars from the page's entry in the JSON outline.
type PlanInput struct {
	Plan      *plan.Plan
	Framing   []byte
	Exemplars []Exemplar
}

// Converted is a blueprint ConvertPlan wrote, and what the conversion left for a person.
type Converted struct {
	// Data is the blueprint file.
	Data []byte
	// Friction holds the plan's friction lines, which belong in the project's friction log.
	Friction []string
	// Departures holds the framing record's departures section, which belongs in the commit message.
	Departures string
	// Preamble holds the framing record's text between its title and its first section: the
	// planning session's note on what it read and wrote, which is history for the commit message.
	Preamble string
	// Judgment lists what the mapping could not decide: one line per item, and the lines that
	// begin "hint:" are guesses a person may discard.
	Judgment []string
}

type framingField struct {
	heading string
	set     func(*Framing, string)
}

// framingFields maps the framing record's headings to the framing field each fills.
var framingFields = []framingField{
	{"Who arrives, from where, and why", func(f *Framing, s string) { f.Readers = s }},
	{"Background the page rests on", func(f *Framing, s string) { f.Background = s }},
	{"Place in the doc set", func(f *Framing, s string) { f.PlaceInDocSet = s }},
	{"Intro plan", func(f *Framing, s string) { f.IntroPlan = s }},
}

const departuresHeading = "Departures from the plan's introduction"

// ConvertPlan maps a v1 plan to a blueprint. It keeps every field and item the plan sets, gives each
// section an id, copies the framing record's four descriptive sections, and writes no section
// reason. The page block, the friction, and the departures are not blueprint fields; they come
// back in the result. The blueprint it returns decodes without a finding, or ConvertPlan fails.
func ConvertPlan(in PlanInput) (*Converted, error) {
	p := in.Plan
	b := &Blueprint{
		Version:       Version,
		Argument:      p.Argument,
		Introduction:  p.Introduction,
		OrderReason:   p.OrderReason,
		Preconditions: p.Preconditions,
		Exemplars:     in.Exemplars,
		Dispositions:  p.Dispositions,
		Constraints:   p.Constraints,
	}
	res := &Converted{Friction: p.Friction}
	if in.Framing != nil {
		var err error
		if b.Framing, res.Preamble, res.Departures, err = parseFraming(in.Framing); err != nil {
			return nil, err
		}
	}

	taken := map[string]bool{}
	for _, s := range p.Sections {
		for _, it := range s.Items {
			taken[it.ID] = true
		}
	}
	for _, s := range p.Sections {
		id, note := sectionID(s, taken)
		if note != "" {
			res.Judgment = append(res.Judgment, note)
		}
		out := Section{ID: id, Heading: s.Heading, Level: s.Level, Anchor: s.Anchor, Opens: s.Opens}
		if out.Level == 2 {
			out.Level = 0 // 2 is the default, so the file leaves it out
		}
		for _, it := range s.Items {
			out.Items = append(out.Items, Item{ID: it.ID, Kind: it.Kind, Fact: it.Fact, Use: it.Use, Mode: it.Mode,
				Text: it.Text, Facts: it.Facts, Source: it.Source, Target: it.Target, Requires: it.Requires,
				Produces: it.Produces, CanFail: it.CanFail, Optional: it.Optional, From: it.From,
				Symptom: it.Symptom, Cause: it.Cause, HandledBy: it.HandledBy})
		}
		b.Sections = append(b.Sections, out)
	}

	data, err := marshalNode(b)
	if err != nil {
		return nil, err
	}
	decoded, findings := Decode("blueprint.yaml", data)
	if len(findings) > 0 {
		return nil, fmt.Errorf("the converted blueprint does not decode: %s", firstError(findings))
	}
	res.Data = data
	res.Judgment = append(res.Judgment, judgment(decoded)...)
	return res, nil
}

// parseFraming splits a framing record into the four framing fields, the preamble under its title,
// and the departures text. A
// level-2 heading outside the known five is refused by name, since a section copied wrongly would
// leave the blueprint quietly short of something the planner wrote.
func parseFraming(data []byte) (f Framing, preamble, departures string, err error) {
	type part struct {
		heading string
		lines   []string
	}
	var parts []*part
	var cur *part
	var pre []string
	for line := range strings.SplitSeq(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			cur = &part{heading: strings.TrimSpace(strings.TrimPrefix(line, "## "))}
			parts = append(parts, cur)
		case cur != nil:
			cur.lines = append(cur.lines, line)
		case strings.HasPrefix(line, "# Framing record:"):
		default:
			pre = append(pre, line)
		}
	}
	preamble = trimBlank(pre)
	seen := map[string]bool{}
	for _, p := range parts {
		if seen[p.heading] {
			return f, "", "", fmt.Errorf("the framing record has the heading %q twice", p.heading)
		}
		seen[p.heading] = true
		body := trimBlank(p.lines)
		if p.heading == departuresHeading {
			departures = body
			continue
		}
		i := slices.IndexFunc(framingFields, func(x framingField) bool { return x.heading == p.heading })
		if i < 0 {
			return f, "", "", fmt.Errorf("the framing record has the heading %q, which the blueprint has no field for", p.heading)
		}
		if body != "" {
			framingFields[i].set(&f, body+"\n")
		}
	}
	return f, preamble, departures, nil
}

// trimBlank joins lines without the blank lines at either end. It keeps indentation and inner blank
// lines, which prose and code samples depend on.
func trimBlank(lines []string) string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimRight(strings.Join(lines, "\n"), " \t")
}

// sectionID returns the section's id and, when the natural id was taken, a judgment line. The id
// is the anchor or the heading, lowercased with each run of other characters made one hyphen. A
// repeat takes -2, -3 and so on, so section ids and item ids stay in one namespace.
func sectionID(s plan.Section, taken map[string]bool) (id, note string) {
	base := idSlug(s.Anchor)
	if base == "" {
		base = idSlug(s.Heading)
	}
	if base == "" {
		base = "section"
	}
	id = base
	for n := 2; taken[id]; n++ {
		id = base + "-" + strconv.Itoa(n)
	}
	taken[id] = true
	if id != base {
		note = fmt.Sprintf("section %q: the id %q was taken, so it is %q; rename it if the order matters", s.Heading, base, id)
	}
	return id, note
}

func idSlug(s string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(s) {
		if r < unicode.MaxASCII && (unicode.IsLower(r) || unicode.IsDigit(r)) {
			if hyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			hyphen = false
			b.WriteRune(r)
			continue
		}
		hyphen = true
	}
	return b.String()
}

// judgment lists what a person decides after the mechanical mapping, over the decoded blueprint.
func judgment(b *Blueprint) []string {
	var out []string
	if b.Framing.IntroPlan == "" {
		out = append(out, "framing.introPlan: write it; the check requires a plan for the page's introduction")
	}
	for _, s := range b.Sections {
		out = append(out, fmt.Sprintf("section %q (%s): write its why", s.Heading, s.ID))
		for _, it := range s.Items {
			if it.Use != "" && !strings.HasPrefix(it.Use, "Only") && !strings.HasPrefix(it.Use, "Omit") {
				out = append(out, fmt.Sprintf("item %q: its use does not start with Only or Omit", it.ID))
			}
		}
	}
	out = append(out, headingHints(b)...)
	return out
}

// headingHints reports prose that names a section's heading, where a [[section-id]] reference
// would stay true when the heading changes. It is a guess: a case-sensitive substring of a heading
// at least four bytes long, so a person decides each.
func headingHints(b *Blueprint) []string {
	type field struct{ label, text, own string }
	var fields []field
	add := func(label, text, own string) {
		if text != "" {
			fields = append(fields, field{label, text, own})
		}
	}
	add("argument", b.Argument, "")
	add("introduction.covers", b.Introduction.Covers, "")
	add("introduction.priorKnowledge", b.Introduction.PriorKnowledge, "")
	add("introduction.notCovered", b.Introduction.NotCovered, "")
	add("framing.readers", b.Framing.Readers, "")
	add("framing.background", b.Framing.Background, "")
	add("framing.placeInDocSet", b.Framing.PlaceInDocSet, "")
	add("framing.introPlan", b.Framing.IntroPlan, "")
	add("orderReason", b.OrderReason, "")
	for i, c := range b.Constraints {
		add(fmt.Sprintf("constraints[%d]", i), c, "")
	}
	for _, s := range b.Sections {
		add("section "+s.ID+" opens", s.Opens, s.Heading)
		for _, it := range s.Items {
			for _, f := range [][2]string{{"use", it.Use}, {"text", it.Text}, {"symptom", it.Symptom}, {"cause", it.Cause}} {
				add("item "+it.ID+" "+f[0], f[1], s.Heading)
			}
		}
	}
	var out []string
	for _, f := range fields {
		for _, s := range b.Sections {
			if len(s.Heading) >= 4 && s.Heading != f.own && strings.Contains(f.text, s.Heading) {
				out = append(out, fmt.Sprintf("hint: %s names the heading %q; [[%s]] would survive a rename", f.label, s.Heading, s.ID))
			}
		}
	}
	return out
}

func firstError(findings []plan.Finding) string {
	i := slices.IndexFunc(findings, func(f plan.Finding) bool { return f.Severity == plan.SeverityError })
	f := findings[max(i, 0)]
	return fmt.Sprintf("line %d: %s", f.Line, f.Message)
}

// marshalNode writes v, a decoded-file struct, as YAML. It walks the struct itself instead of
// calling the encoder on it, for three reasons the encoder cannot meet: a field a value does not
// set is left out (the schema tags say which are required), a required empty list is written as
// [] where an absent one would be refused, and a prose field is a | block when it spans lines.
// The encoder still picks quoting for a one-line value, so a leading [[ , a colon, and a # comment
// survive a round trip.
func marshalNode(v any) ([]byte, error) {
	root := structNode(reflect.ValueOf(v).Elem())
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}); err != nil {
		return nil, errors.New("encode: " + err.Error())
	}
	if err := enc.Close(); err != nil {
		return nil, errors.New("encode: " + err.Error())
	}
	return buf.Bytes(), nil
}

func structNode(v reflect.Value) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	for i := range v.NumField() {
		f := v.Type().Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "" || name == "-" {
			continue
		}
		tags := strings.Split(f.Tag.Get("plan"), ",")
		val := valueNode(v.Field(i), slices.Contains(tags, "prose"), slices.Contains(tags, "required"), slices.Contains(tags, "empty"))
		if val != nil {
			n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: name}, val)
		}
	}
	return n
}

// valueNode returns the node for v, or nil when v is unset and the field is not required.
func valueNode(v reflect.Value, prose, required, empty bool) *yaml.Node {
	switch v.Kind() {
	case reflect.String:
		if v.String() == "" {
			return nil
		}
		return stringNode(v.String(), prose)
	case reflect.Int:
		if v.Int() == 0 {
			return nil
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(v.Int(), 10)}
	case reflect.Bool:
		if !v.Bool() {
			return nil
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}
	case reflect.Struct:
		n := structNode(v)
		if len(n.Content) == 0 && !required {
			return nil
		}
		return n
	case reflect.Slice:
		if v.Len() == 0 {
			if required && empty {
				return &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
			}
			return nil
		}
		n := &yaml.Node{Kind: yaml.SequenceNode}
		for i := range v.Len() {
			e := v.Index(i)
			if c, ok := reflect.TypeAssert[Cover](e); ok && c.ID == "" {
				n.Content = append(n.Content, stringNode(c.Text, prose))
				continue
			}
			if e.Kind() == reflect.String {
				n.Content = append(n.Content, stringNode(e.String(), prose))
				continue
			}
			n.Content = append(n.Content, structNode(e))
		}
		return n
	}
	return nil
}

func stringNode(s string, prose bool) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
	if prose && strings.Contains(s, "\n") {
		n.Style = yaml.LiteralStyle
	}
	return n
}

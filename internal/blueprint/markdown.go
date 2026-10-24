package blueprint

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/glw907/drafter/internal/plan"
)

// markdownSuffix names a blueprint in the markdown format. A page may have a .blueprint.md or a
// .blueprint.yaml; where both exist the markdown file is the page's blueprint.
const markdownSuffix = ".blueprint.md"

const checkMarkdown = "markdown"

// Markdown is a decoded markdown blueprint: YAML front matter, a level-1 title, a :::reader block,
// then the page's sections as headings. Text is the file as written, which is what a draft
// request carries. The other fields are the parts the checks, refs, the lock, and section scoping
// read; Go decodes no further than they need. ReaderLines holds the line of every :::reader
// block's opening fence, Reader being the first. Constraints and ConstraintLines are the same for
// the :::constraints block. BareFences holds the line of every code fence that has no Code: or
// Figure: bullet directly above it.
type Markdown struct {
	Front           Front
	Text            string
	Reader          Reader
	ReaderLines     []int
	Constraints     Reader
	ConstraintLines []int
	BareFences      []int
	Sections        []MDSection
}

// Front is the front matter. Page is the page's title, Type the outline's type name for it,
// Track the track that holds it, and Exemplar the source of the published page to imitate.
type Front struct {
	Page     string `yaml:"page"`
	Type     string `yaml:"type"`
	Track    string `yaml:"track"`
	Exemplar string `yaml:"exemplar"`

	Line int `yaml:"-"`
}

// Cite is one :fact tag: the id as written and the line it sits on.
type Cite struct {
	Line int
	ID   string
}

// Reader is the :::reader block, the page's statement of who reads it and why. Line is its
// opening fence's line, zero when the file has none. Text omits the :fact tags; Facts lists their
// ids in order and Cites the same with their lines.
type Reader struct {
	Line  int
	Text  string
	Facts []string
	Cites []Cite
}

// MDSection is one heading and what sits under it up to the next heading. ID is the heading's
// slug, unique in the file, which keys the section in the lock as a YAML section's id does. Job
// is the italic line under the heading that says what the section does for the reader. Raw is the
// section's text as written, heading line included, trimmed of surrounding blank lines.
type MDSection struct {
	ID, Heading string
	Level, Line int
	Job         string
	Raw         string
	Lines       []MDLine
	Rows        []Row
}

// MDLine is one bullet or numbered step. Text omits the list marker, the :fact and :source tags,
// and the Code: or Figure: label; Facts lists the tags' ids in order. Kind is "code" for a Code:
// bullet and "figure" for a Figure: bullet, empty otherwise, and Source is the bullet's :source
// pointer. Fence is the body of the code fence attached to the bullet, empty when it has none.
type MDLine struct {
	Line   int
	Kind   string
	Text   string
	Source string
	Facts  []string
	Fence  string
}

// Row is one :::row block of a symptom page: what the reader sees, why, and what to do. Line is
// its opening fence's line. Facts are the ids its parts cite, and Cites the same with their lines.
type Row struct {
	Line                int
	Symptom, Cause, Fix string
	Facts               []string
	Cites               []Cite
}

var (
	factTag     = regexp.MustCompile(`\s*:fact\[([^\]\s]+)\]`)
	sourceTag   = regexp.MustCompile(`\s*:source\[((?:[^\[\]\s]|\[[^\[\]\s]*\])+)\]`)
	formLabel   = regexp.MustCompile(`^(Code|Figure):\s*`)
	listMarker  = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+`)
	headingLine = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	rowPart     = regexp.MustCompile(`^\*\*(Symptom|Cause|Fix)\.\*\*\s*`)
	seeLink     = regexp.MustCompile(`^See \[(.+)\]\(([^()\s]+)\)\.$`)
)

// DecodeMarkdown parses data as a markdown blueprint. The front matter is YAML between two ---
// lines and takes the keys page, type, track, and exemplar. After it a line scanner reads the
// body, since goldmark has no directive extension, and skips fenced code:
//
//   - A heading of level 2 to 6 opens a section, and a level-1 heading is the page title and ends
//     the section above it. A section's ID is its heading's slug, taken in page order under the
//     front matter's page title.
//   - The first non-blank line under a heading, when it is one italic span, is the section's job.
//   - A bullet or numbered line is an MDLine, with its :fact[id] tags lifted out of its text. A
//     bullet that opens "Code:" or "Figure:" records that kind, and its :source[path] tag becomes
//     Source, and a second :source tag on it is a warning. A code fence below a Code: or Figure:
//     bullet, blank lines and a comment line allowed between, attaches to it as its Fence; any
//     other fence is recorded in BareFences.
//   - A line that opens with ":::" opens a directive block that runs to the next line holding
//     only ":::". :::reader fills Reader; a second one adds only its line to ReaderLines.
//     :::constraints does the same for Constraints and ConstraintLines. :::row
//     adds a Row to the section above it, reading its **Symptom.**, **Cause.**, and **Fix.**
//     lines, each of which may wrap onto the lines after it.
//
// A fault in the markup itself is a finding at its line: a block that never closes, a ":::" that
// closes nothing, a directive of another name, and a :::row above every heading. A section with no
// job and a row missing a part are not faults here; the blueprint check reports them. The result
// is nil only when the front matter is missing or is not a YAML mapping.
func DecodeMarkdown(file string, data []byte) (*Markdown, []plan.Finding) {
	var findings []plan.Finding
	add := func(line int, msg, fix string) {
		findings = append(findings, plan.Finding{File: file, Line: line, Severity: plan.SeverityError, Check: checkMarkdown, Message: msg, Fix: fix})
	}
	warn := func(line int, msg, fix string) {
		findings = append(findings, plan.Finding{File: file, Line: line, Severity: plan.SeverityWarning, Check: checkMarkdown, Message: msg, Fix: fix})
	}
	text := string(data)
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		add(1, "the blueprint has no front matter", "open the file with a --- line, the page, type, track, and exemplar keys, and a closing --- line")
		return nil, findings
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		add(1, "the front matter has no closing --- line", "close the front matter with a --- line")
		return nil, findings
	}
	m := &Markdown{Text: text, Front: Front{Line: 1}}
	dec := yaml.NewDecoder(strings.NewReader(strings.Join(lines[1:end], "\n")))
	dec.KnownFields(true)
	if err := dec.Decode(&m.Front); err != nil && !errors.Is(err, io.EOF) {
		add(1, "the front matter does not decode: "+err.Error(), "write the front matter as page, type, track, and exemplar keys")
		return nil, findings
	}
	m.Front.Line = 1

	var headings []string
	cur, rawStart := -1, 0
	closeSection := func(at int) {
		if cur >= 0 {
			m.Sections[cur].Raw = strings.TrimSpace(strings.Join(lines[rawStart:at], "\n"))
		}
	}
	var open *block
	fence := ""
	// formAt is the index in the current section's Lines of a Code: or Figure: bullet that a fence
	// may still attach to, or -1. Blank lines and a comment line leave it; any other line clears it.
	// fenceTo is the bullet whose fence is being read, and fenceBody that fence's lines so far.
	formAt, fenceTo := -1, -1
	var fenceBody []string
	for i := end + 1; i < len(lines); i++ {
		line, n := lines[i], i+1
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
				if fenceTo >= 0 {
					m.Sections[cur].Lines[fenceTo].Fence = strings.Join(fenceBody, "\n")
					fenceTo = -1
				}
				continue
			}
			fenceBody = append(fenceBody, line)
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			fenceBody = nil
			if open == nil {
				if formAt < 0 {
					m.BareFences = append(m.BareFences, n)
				} else {
					fenceTo = formAt
				}
				formAt = -1
			}
			continue
		}
		if open != nil {
			if trimmed != ":::" {
				if sourceTag.MatchString(line) && (open.name != "reader" || !isForm(line, "figure")) {
					add(n, "this :source tag is not on a Code: or Figure: bullet, so drafter does not read it", "put the tag on a Code: bullet in a section, or on a Figure: bullet")
				}
				open.add(n, line)
				continue
			}
			switch open.name {
			case "reader":
				m.ReaderLines = append(m.ReaderLines, open.line)
				if len(m.ReaderLines) == 1 {
					m.Reader = open.reader()
				}
			case "constraints":
				m.ConstraintLines = append(m.ConstraintLines, open.line)
				if len(m.ConstraintLines) == 1 {
					m.Constraints = open.reader()
				}
			case "row":
				if cur < 0 {
					add(open.line, "a :::row block sits above the first heading, so it belongs to no section", "move the row under the section it explains")
					break
				}
				m.Sections[cur].Rows = append(m.Sections[cur].Rows, open.row())
			}
			open = nil
			continue
		}
		loc := listMarker.FindStringIndex(line)
		heading := headingLine.FindStringSubmatch(line)
		// A tag drafter never reads would drop out of the check, refs, and staleness alike. Only a
		// bullet or step inside a section, and the lines of an open block, are read for tags.
		if factTag.MatchString(line) && (loc == nil || cur < 0 || heading != nil) {
			add(n, "this :fact tag is not on a bullet, step, row, or reader line, so drafter does not read it", "put the tag at the end of the bullet or step's first line, or inside the row")
			if heading == nil && !strings.HasPrefix(trimmed, ":::") {
				continue
			}
		}
		if sourceTag.MatchString(line) && (loc == nil || cur < 0 || heading != nil || !isForm(line, "")) {
			add(n, "this :source tag is not on a Code: or Figure: bullet, so drafter does not read it", "put the tag at the end of a Code: or Figure: bullet's first line")
			if heading == nil && !strings.HasPrefix(trimmed, ":::") {
				continue
			}
		}
		// A comment line and an indented continuation of the bullet leave the pending Code: bullet.
		wrapped := loc == nil && heading == nil && !strings.HasPrefix(trimmed, ":::") && line != strings.TrimLeft(line, " \t")
		if trimmed != "" && !strings.HasPrefix(trimmed, "<!--") && !wrapped {
			formAt = -1
		}
		if strings.HasPrefix(trimmed, ":::") {
			name, _, _ := strings.Cut(strings.TrimSpace(trimmed[3:]), " ")
			switch name {
			case "":
				add(n, "a ::: line closes no block", "delete the line, or open a block above it with :::reader or :::row")
				continue
			case "reader", "row", "constraints":
			default:
				add(n, "the directive :::"+name+" is not one the format has", "use :::reader, :::constraints, or :::row")
			}
			open = &block{name: name, line: n}
			continue
		}
		if h := heading; h != nil {
			closeSection(i)
			if len(h[1]) == 1 {
				cur = -1
				continue
			}
			m.Sections = append(m.Sections, MDSection{Heading: h[2], Level: len(h[1]), Line: n})
			headings = append(headings, h[2])
			cur, rawStart = len(m.Sections)-1, i
			continue
		}
		if cur < 0 || trimmed == "" {
			continue
		}
		sec := &m.Sections[cur]
		if sec.Job == "" && len(sec.Lines) == 0 && len(sec.Rows) == 0 && isItalicLine(trimmed) {
			sec.Job = trimmed[1 : len(trimmed)-1]
			continue
		}
		if loc != nil {
			body := line[loc[1]:]
			var ids []string
			for _, t := range factTag.FindAllStringSubmatch(body, -1) {
				ids = append(ids, t[1])
			}
			ml := MDLine{Line: n, Facts: ids}
			if lab := formLabel.FindStringSubmatch(body); lab != nil {
				ml.Kind = strings.ToLower(lab[1])
				body = body[len(lab[0]):]
				if srcs := sourceTag.FindAllStringSubmatch(body, -1); srcs != nil {
					ml.Source = srcs[0][1]
					for _, extra := range srcs[1:] {
						warn(n, "this bullet has a second :source tag, "+extra[1]+", which drafter does not read", "keep one :source tag per bullet and put the other on its own Code: or Figure: bullet")
					}
				}
			}
			ml.Text = strings.TrimSpace(sourceTag.ReplaceAllString(factTag.ReplaceAllString(body, ""), ""))
			sec.Lines = append(sec.Lines, ml)
			if ml.Kind != "" {
				formAt = len(sec.Lines) - 1
			}
		}
	}
	closeSection(len(lines))
	if open != nil {
		add(open.line, "the :::"+open.name+" block has no closing ::: line", "close the block with a line holding only :::")
	}
	for i, slug := range plan.HeadingSlugs(m.Front.Page, headings) {
		m.Sections[i].ID = slug
	}
	return m, findings
}

// block is a directive block being read: the lines between its fences and where each sits.
type block struct {
	name string
	line int
	text []string
	nums []int
}

func (b *block) add(n int, line string) {
	b.text = append(b.text, line)
	b.nums = append(b.nums, n)
}

func (b *block) tags() (ids []string, cites []Cite) {
	for i, l := range b.text {
		for _, t := range factTag.FindAllStringSubmatch(l, -1) {
			ids = append(ids, t[1])
			cites = append(cites, Cite{Line: b.nums[i], ID: t[1]})
		}
	}
	return ids, cites
}

func (b *block) reader() Reader {
	r := Reader{Line: b.line}
	r.Facts, r.Cites = b.tags()
	stripped := make([]string, len(b.text))
	for i, l := range b.text {
		stripped[i] = sourceTag.ReplaceAllString(factTag.ReplaceAllString(l, ""), "")
	}
	r.Text = strings.TrimSpace(strings.Join(stripped, "\n"))
	return r
}

func (b *block) row() Row {
	r := Row{Line: b.line}
	r.Facts, r.Cites = b.tags()
	var part *string
	for _, l := range b.text {
		l = strings.TrimSpace(factTag.ReplaceAllString(l, ""))
		if m := rowPart.FindStringSubmatch(l); m != nil {
			switch m[1] {
			case "Symptom":
				part = &r.Symptom
			case "Cause":
				part = &r.Cause
			default:
				part = &r.Fix
			}
			l = l[len(m[0]):]
		}
		if part != nil && l != "" {
			*part = strings.TrimSpace(*part + " " + l)
		}
	}
	return r
}

// isForm reports whether line is a list item that opens with the Code: or Figure: label of kind,
// or with either label when kind is empty.
func isForm(line, kind string) bool {
	loc := listMarker.FindStringIndex(line)
	if loc == nil {
		return false
	}
	lab := formLabel.FindStringSubmatch(line[loc[1]:])
	return lab != nil && (kind == "" || strings.ToLower(lab[1]) == kind)
}

// isItalicLine reports whether s is one emphasized span, *like this* or _like this_.
func isItalicLine(s string) bool {
	if len(s) < 3 {
		return false
	}
	for _, d := range []byte{'*', '_'} {
		if s[0] == d && s[len(s)-1] == d && s[1] != d && s[len(s)-2] != d {
			return true
		}
	}
	return false
}

// resolveFact maps a :fact tag to the id the facts store holds. A tag omits the store's "f:"
// prefix, as in :fact[rkj7tn] for f:rkj7tn, but a tag written whole matches too. An id the store
// does not hold comes back as written, so the facts check can name it.
func resolveFact(facts *plan.Facts, tag string) string {
	if facts == nil || facts.Count(tag) > 0 {
		return tag
	}
	if facts.Count("f:"+tag) > 0 {
		return "f:" + tag
	}
	return tag
}

func resolveFacts(facts *plan.Facts, tags []string) []string {
	var out []string
	for _, t := range tags {
		out = append(out, resolveFact(facts, t))
	}
	return out
}

// FactIDs lists the ids of every fact the section cites, in page order, resolved against facts.
func (s *MDSection) FactIDs(facts *plan.Facts) []string {
	var ids []string
	for _, l := range s.Lines {
		ids = append(ids, resolveFacts(facts, l.Facts)...)
	}
	for _, r := range s.Rows {
		ids = append(ids, resolveFacts(facts, r.Facts)...)
	}
	return ids
}

// FactIDs lists the sorted, distinct ids of every fact the blueprint cites, resolved against facts.
func (m *Markdown) FactIDs(facts *plan.Facts) []string {
	ids := resolveFacts(facts, slices.Concat(m.Reader.Facts, m.Constraints.Facts))
	for i := range m.Sections {
		ids = append(ids, m.Sections[i].FactIDs(facts)...)
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// Source is a blueprint in either format, as the draft path reads it. [Load] returns one.
type Source interface {
	// View projects the blueprint onto a plan, as the package function View does for YAML.
	View(entry Page, facts *plan.Facts) *plan.Plan
	// ExemplarSources lists the sources of the published pages the page imitates.
	ExemplarSources() []string
	// SectionHeads lists each section's id and heading in page order.
	SectionHeads() []SectionHead
	// CodeSources lists the source file of each code item, in order.
	CodeSources() []string
}

// SectionHead is a section's id and heading.
type SectionHead struct {
	ID, Heading string
}

// Load decodes a blueprint of either format, chosen by file's suffix. The Source is nil when the
// file does not decode at all; the findings say why.
func Load(file string, data []byte) (Source, []plan.Finding) {
	if strings.HasSuffix(file, markdownSuffix) {
		m, findings := DecodeMarkdown(file, data)
		if m == nil {
			return nil, findings
		}
		return m, findings
	}
	b, findings := Decode(file, data)
	if b == nil {
		return nil, findings
	}
	return b, findings
}

// View calls the package function View on b.
func (b *Blueprint) View(entry Page, facts *plan.Facts) *plan.Plan { return View(b, entry, facts) }

// ExemplarSources lists each exemplar's source.
func (b *Blueprint) ExemplarSources() []string {
	var out []string
	for _, ex := range b.Exemplars {
		out = append(out, ex.Source)
	}
	return out
}

// SectionHeads lists each section's id and heading.
func (b *Blueprint) SectionHeads() []SectionHead {
	out := make([]SectionHead, len(b.Sections))
	for i, s := range b.Sections {
		out[i] = SectionHead{ID: s.ID, Heading: s.Heading}
	}
	return out
}

// CodeSources lists the source of each code item.
func (b *Blueprint) CodeSources() []string {
	var out []string
	for _, s := range b.Sections {
		for _, it := range s.Items {
			if it.Kind == "code" && it.Source != "" {
				out = append(out, it.Source)
			}
		}
	}
	return out
}

// CodeSources lists the :source of each Code: bullet, in page order.
func (m *Markdown) CodeSources() []string {
	var out []string
	for _, s := range m.Sections {
		for _, l := range s.Lines {
			if l.Kind == "code" && l.Source != "" {
				out = append(out, l.Source)
			}
		}
	}
	return out
}

// View projects m onto a plan so the code that reads a YAML blueprint's view reads a markdown one.
// The page's path, title, and type come from entry. The reader block is the introduction: its
// text the covers, its tags the facts. Each bullet becomes an example item whose id is
// <section id>.p<n>, the anchor form a review note uses, except a "See [text](target)." bullet,
// which becomes a link item so the links check reads its target, a Code: bullet a code item, and
// a Figure: bullet a figure item, each carrying its :source; each row becomes an example item
// holding its three parts, with id <section id>.row<n>. Every :fact tag resolves against facts,
// so the plan checks look up the id the store holds; a nil facts leaves the tags as written.
func (m *Markdown) View(entry Page, facts *plan.Facts) *plan.Plan {
	p := &plan.Plan{
		Version: plan.Version,
		Page:    plan.Page{Path: entry.Path, Title: entry.Title, Type: entry.Type, Line: 1, Lines: map[string]int{"path": 1, "title": 1, "type": 1}},
		Introduction: plan.Introduction{
			Covers: m.Reader.Text, Facts: resolveFacts(facts, m.Reader.Facts), Line: m.Reader.Line,
			Lines: map[string]int{"facts": citeLine(m.Reader.Cites, m.Reader.Line)},
		},
		Line: 1,
	}
	for _, s := range m.Sections {
		ps := plan.Section{Heading: s.Heading, Level: s.Level, Anchor: s.ID, Opens: s.Job, Line: s.Line, Lines: map[string]int{"heading": s.Line}}
		for i, l := range s.Lines {
			it := plan.Item{
				ID: fmt.Sprintf("%s.p%d", s.ID, i+1), Kind: "example", Text: l.Text, Facts: resolveFacts(facts, l.Facts), Line: l.Line,
				Lines: map[string]int{"text": l.Line, "facts": l.Line},
			}
			if l.Kind != "" {
				it.Kind, it.Source = l.Kind, l.Source
				if l.Kind == "figure" && l.Fence != "" {
					it.Text += "\n" + l.Fence
				}
				if l.Source != "" {
					it.Lines["source"] = l.Line
				}
			} else if see := seeLink.FindStringSubmatch(l.Text); see != nil {
				it.Kind, it.Text, it.Target, it.Facts = "link", see[1], see[2], nil
				it.Lines = map[string]int{"text": l.Line, "target": l.Line}
			}
			ps.Items = append(ps.Items, it)
		}
		for i, r := range s.Rows {
			ps.Items = append(ps.Items, plan.Item{
				ID: fmt.Sprintf("%s.row%d", s.ID, i+1), Kind: "example", Facts: resolveFacts(facts, r.Facts), Line: r.Line,
				Text:  "Symptom. " + r.Symptom + "\nCause. " + r.Cause + "\nFix. " + r.Fix,
				Lines: map[string]int{"text": r.Line, "facts": citeLine(r.Cites, r.Line)},
			})
		}
		p.Sections = append(p.Sections, ps)
	}
	return p
}

// citeLine is the line of the first cite, or def when there are none.
func citeLine(cites []Cite, def int) int {
	if len(cites) > 0 {
		return cites[0].Line
	}
	return def
}

// ExemplarSources lists the front matter's exemplar, when it names one.
func (m *Markdown) ExemplarSources() []string {
	if m.Front.Exemplar == "" {
		return nil
	}
	return []string{m.Front.Exemplar}
}

// SectionHeads lists each section's id and heading.
func (m *Markdown) SectionHeads() []SectionHead {
	out := make([]SectionHead, len(m.Sections))
	for i, s := range m.Sections {
		out[i] = SectionHead{ID: s.ID, Heading: s.Heading}
	}
	return out
}

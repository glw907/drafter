package blueprint

import (
	"errors"
	"fmt"
	"io"
	"regexp"
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
// read; Go decodes no further than they need.
type Markdown struct {
	Front    Front
	Text     string
	Reader   Reader
	Sections []MDSection
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

// Reader is the :::reader block, the page's statement of who reads it and why. Line is its
// opening fence's line, zero when the file has none.
type Reader struct {
	Line int
	Text string
}

// MDSection is one heading and what sits under it up to the next heading. ID is the heading's
// slug, unique in the file, which keys the section in the lock as a YAML section's id does. Job
// is the italic line under the heading that says what the section does for the reader.
type MDSection struct {
	ID, Heading string
	Level, Line int
	Job         string
	Lines       []MDLine
	Rows        []Row
}

// MDLine is one bullet or numbered step. Text omits the list marker and the :fact tags; Facts
// lists the tags' ids in order.
type MDLine struct {
	Line  int
	Text  string
	Facts []string
}

// Row is one :::row block of a symptom page: what the reader sees, why, and what to do. Facts are
// the ids its parts cite.
type Row struct {
	Line                int
	Symptom, Cause, Fix string
	Facts               []string
}

var (
	factTag     = regexp.MustCompile(`\s*:fact\[([^\]\s]+)\]`)
	listMarker  = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+`)
	headingLine = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
)

// DecodeMarkdown parses data as a markdown blueprint. It reads the front matter, the headings,
// each section's job line, and each bullet's :fact tags, skipping fenced code. A directive block
// (a line opening with ":::" up to the line that is ":::" alone) is skipped whole, so Reader and
// Rows stay empty; reading :::reader and :::row into them completes the format. The result is nil only when the front matter is
// missing or is not a YAML mapping; every other fault is a finding at its line.
func DecodeMarkdown(file string, data []byte) (*Markdown, []plan.Finding) {
	var findings []plan.Finding
	add := func(line int, msg, fix string) {
		findings = append(findings, plan.Finding{File: file, Line: line, Severity: plan.SeverityError, Check: checkMarkdown, Message: msg, Fix: fix})
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
	var sec *MDSection
	fence, directive := "", false
	for i := end + 1; i < len(lines); i++ {
		line, n := lines[i], i+1
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			continue
		}
		if directive {
			if trimmed == ":::" {
				directive = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, ":::") {
			directive = true
			continue
		}
		if h := headingLine.FindStringSubmatch(line); h != nil {
			level := len(h[1])
			if level == 1 {
				sec = nil
				continue
			}
			m.Sections = append(m.Sections, MDSection{Heading: h[2], Level: level, Line: n})
			headings = append(headings, h[2])
			sec = &m.Sections[len(m.Sections)-1]
			continue
		}
		if sec == nil || trimmed == "" {
			continue
		}
		if sec.Job == "" && len(sec.Lines) == 0 && isItalicLine(trimmed) {
			sec.Job = trimmed[1 : len(trimmed)-1]
			continue
		}
		if loc := listMarker.FindStringIndex(line); loc != nil {
			body := line[loc[1]:]
			var ids []string
			for _, t := range factTag.FindAllStringSubmatch(body, -1) {
				ids = append(ids, t[1])
			}
			sec.Lines = append(sec.Lines, MDLine{Line: n, Text: strings.TrimSpace(factTag.ReplaceAllString(body, "")), Facts: ids})
		}
	}
	for i, slug := range plan.HeadingSlugs(m.Front.Page, headings) {
		m.Sections[i].ID = slug
	}
	return m, findings
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

// CodeSources is empty: a markdown blueprint has no code items, and its facts' excerpts carry
// the code a draft reads.
func (m *Markdown) CodeSources() []string { return nil }

// View projects m onto a plan so the code that reads a YAML blueprint's view reads a markdown one.
// The page's path, title, and type come from entry. The reader block is the introduction's
// covers. Each bullet becomes an example item whose id is <section id>.p<n>, the anchor form a
// review note uses, and each row an example item holding its three parts. facts is unused: a
// markdown blueprint cites facts only through :fact tags.
func (m *Markdown) View(entry Page, _ *plan.Facts) *plan.Plan {
	p := &plan.Plan{
		Version:      plan.Version,
		Page:         plan.Page{Path: entry.Path, Title: entry.Title, Type: entry.Type, Line: 1, Lines: map[string]int{"path": 1, "title": 1, "type": 1}},
		Introduction: plan.Introduction{Covers: m.Reader.Text, Line: m.Reader.Line},
		Line:         1,
	}
	for _, s := range m.Sections {
		ps := plan.Section{Heading: s.Heading, Level: s.Level, Anchor: s.ID, Opens: s.Job, Line: s.Line, Lines: map[string]int{"heading": s.Line}}
		for i, l := range s.Lines {
			ps.Items = append(ps.Items, plan.Item{
				ID: fmt.Sprintf("%s.p%d", s.ID, i+1), Kind: "example", Text: l.Text, Facts: l.Facts, Line: l.Line,
				Lines: map[string]int{"text": l.Line, "facts": l.Line},
			})
		}
		for i, r := range s.Rows {
			ps.Items = append(ps.Items, plan.Item{
				ID: fmt.Sprintf("%s.row%d", s.ID, i+1), Kind: "example", Facts: r.Facts, Line: r.Line,
				Text:  "Symptom. " + r.Symptom + "\nCause. " + r.Cause + "\nFix. " + r.Fix,
				Lines: map[string]int{"text": r.Line, "facts": r.Line},
			})
		}
		p.Sections = append(p.Sections, ps)
	}
	return p
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

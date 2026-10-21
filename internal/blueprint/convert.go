package blueprint

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v3"

	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/internal/run"
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
		o.Pages = append(o.Pages, Page{Path: p.Path, Slug: p.Slug, Title: p.Title, Type: p.PageType,
			Job: p.Job, Covers: p.Covers, OutOfScope: p.OutOfScope, Figure: p.Figure})
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

// MarkdownConverted reports one page ConvertToMarkdown converted.
type MarkdownConverted struct {
	// Page is the page's path, From the YAML blueprint that was deleted, and To the markdown
	// blueprint that replaced it, both relative to the repository root.
	Page, From, To string
	// Judgment lists what the mapping could not carry over or decide, one line each.
	Judgment []string
}

// ConvertToMarkdown converts each YAML blueprint named, a repo-relative slash path, to a markdown
// blueprint beside it, deletes the YAML, and rewrites the page's drafted.lock entry so a page
// whose file has not changed stays up to date: the entry's blueprint becomes the new file's blob
// and its sections are hashed again under the markdown headings' slugs. The entry's accepted id
// and read bar are dropped, since no read has judged the new file.
//
// Every blueprint is checked before anything is written, and the first refusal names the page and
// the fix. A blueprint is refused when it does not decode, its page is not a managed one the
// outline lists, the page already has a markdown blueprint, or the page's lock entry records a
// draft and no longer matches it: the scope code would call the page changed, because the
// blueprint, the outline entry, or a fact it cites differs from what drafter last drafted from.
// Convert such a page after drafter page has brought it up to date.
func (p Project) ConvertToMarkdown(blueprints []string) ([]MarkdownConverted, error) {
	gitDir, err := gitRaw(p.Root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, err
	}
	// The worktree lock keeps a running drafter from reading a lock this call is about to change.
	wl, err := run.TryLock(filepath.Join(strings.TrimSpace(string(gitDir)), "drafter.lock"))
	if err != nil {
		return nil, fmt.Errorf("another drafter run holds this worktree: %w", err)
	}
	defer func() { _ = wl.Release() }()

	l, err := p.load()
	if err != nil {
		return nil, err
	}
	type job struct {
		MarkdownConverted
		data []byte
	}
	var jobs []job
	for _, rel := range blueprints {
		page, ok := PagePath(p.BlueprintRoot, rel)
		if !ok || !strings.HasSuffix(rel, blueprintSuffix) {
			return nil, fmt.Errorf("%s is not a blueprint to convert; name a %s file under %s", rel, blueprintSuffix, p.BlueprintRoot)
		}
		data, err := os.ReadFile(p.abs(rel))
		if err != nil {
			return nil, fmt.Errorf("read blueprint: %w", err)
		}
		b, findings := Decode(rel, data)
		if b == nil || plan.HasError(findings) {
			return nil, fmt.Errorf("%s does not decode; run drafter check", rel)
		}
		entry, listed := l.pages[page]
		if !listed || !entry.Managed {
			return nil, fmt.Errorf("%s is not a managed page the outline lists; fix %s first", page, p.Outline)
		}
		to := MarkdownPath(p.BlueprintRoot, page)
		if _, err := os.Lstat(p.abs(to)); err == nil {
			return nil, fmt.Errorf("%s already exists; convert never overwrites a blueprint", to)
		}
		if e, ok := l.lock.Pages[page]; ok && e.Page != "" {
			if why, err := p.stale(l, rel, entry, e, data); err != nil {
				return nil, err
			} else if why != "" {
				return nil, fmt.Errorf("%s is stale: %s; run drafter page to bring it up to date, then convert", page, why)
			}
		}
		track := ""
		if t, ok := l.outline.TrackFor(page); ok {
			track = t.Name
		}
		md, judgment := RenderMarkdown(b, entry, track, l.facts)
		if track == "" {
			judgment = append(judgment, "the outline gives the page no single track, so the front matter's track is empty")
		}
		jobs = append(jobs, job{MarkdownConverted{Page: page, From: rel, To: to, Judgment: judgment}, md})
	}

	lock := l.lock
	for _, j := range jobs {
		if err := writeExclusive(p.abs(j.To), j.data); err != nil {
			return nil, err
		}
		if err := os.Remove(p.abs(j.From)); err != nil {
			return nil, fmt.Errorf("delete the converted blueprint: %w", err)
		}
		e, ok := lock.Pages[j.Page]
		if !ok {
			continue
		}
		e.Accepted, e.Blocked = "", nil
		if e.Page != "" {
			md, _ := DecodeMarkdown(j.To, j.data)
			pageBytes := BlobBytes(p.Root, e.Page)
			if pageBytes == nil {
				return nil, fmt.Errorf("the page recorded for %s is not in the object store, so its sections cannot be hashed; the blueprint %s is converted and the lock is not", j.Page, j.To)
			}
			blob, err := FileBlobID(p.Root, j.To)
			if err != nil {
				return nil, err
			}
			e.Blueprint, e.Sections = blob, SectionHashes(md, pageBytes)
		}
		if e.Page == "" && e.Blueprint == "" && e.Accepted == "" {
			delete(lock.Pages, j.Page)
			continue
		}
		lock.Pages[j.Page] = e
	}
	if err := WriteLock(p.abs(l.lockRel), lock); err != nil {
		return nil, err
	}
	out := make([]MarkdownConverted, len(jobs))
	for i, j := range jobs {
		out[i] = j.MarkdownConverted
	}
	return out, nil
}

// stale says why the lock entry e of a drafted page no longer matches its YAML blueprint, or ""
// when it does. It asks Compare what the scope code asks: the blueprint the entry recorded, with
// the outline entry and facts as they stood just before the commit that introduced the entry,
// against the blueprint on disk with the outline entry and facts as they are now.
func (p Project) stale(l *loaded, rel string, entry Page, e LockEntry, current []byte) (string, error) {
	recorded := BlobBytes(p.Root, e.Blueprint)
	if recorded == nil {
		return "the blueprint the lock recorded is not in the object store", nil
	}
	introduced, err := LockIntroducedAt(p.Root, l.lockRel, entry.Path, e)
	if err != nil {
		return "", err
	}
	if introduced == "" {
		return "no commit introduced its entry in " + LockFile, nil
	}
	old := p.revisionAt(ParentCommit(p.Root, introduced), entry.Path)
	old.Blueprint = recorded
	cur := Revision{Blueprint: current, Entry: &entry, Facts: l.facts}
	if sc := Compare(rel, old, cur); sc.Kind != ScopeUnchanged {
		if sc.Why != "" {
			return sc.Why, nil
		}
		return "its blueprint, outline entry, or a fact it cites changed since the page was drafted", nil
	}
	return "", nil
}

// revisionAt gathers a page's outline entry and facts store as committed at rev. What rev lacks
// or cannot decode comes back empty, which Compare reads as a reason to treat the page whole.
func (p Project) revisionAt(rev, pagePath string) Revision {
	var r Revision
	if out, err := gitRaw(p.Root, "show", rev+":"+p.Outline); err == nil {
		if o, fs := DecodeOutline(p.Outline, out); o != nil && !plan.HasError(fs) {
			for i := range o.Pages {
				if path.Clean(o.Pages[i].Path) == pagePath {
					r.Entry = &o.Pages[i]
				}
			}
		}
	}
	if p.FactsDir == "" {
		return r
	}
	dir, err := filepath.Rel(p.Root, p.FactsDir)
	if err != nil {
		return r
	}
	dir = filepath.ToSlash(dir)
	names, err := gitRaw(p.Root, "ls-tree", "--name-only", rev+":"+dir)
	if err != nil {
		return Revision{}
	}
	files := map[string]string{}
	for name := range strings.FieldsSeq(string(names)) {
		if !strings.HasSuffix(name, ".md") || name == "README.md" {
			continue
		}
		if out, err := gitRaw(p.Root, "show", rev+":"+dir+"/"+name); err == nil {
			files[name] = string(out)
		}
	}
	r.Facts = plan.ParseFacts(files, p.Root)
	return r
}

// RenderMarkdown writes b, the blueprint of the page entry describes, as a markdown blueprint.
// track names the track that holds the page. facts is the project's facts store, or nil, and
// supplies the claim a fact item places. The second result lists what the mapping dropped or could
// not decide.
//
// The reader block carries the framing record and the introduction. A section keeps its heading
// level, its why as the job line, its opens text as the first bullet, and each item as a bullet,
// numbered for a step, with its facts as :fact tags that omit the store's "f:" prefix. A link is a
// "See [text](target)." bullet. A failure is a :::row block whose Fix is its handler's text, and
// a handler that is not a step is not repeated as a bullet. A [[section-id]] reference becomes
// the section's heading.
func RenderMarkdown(b *Blueprint, entry Page, track string, facts *plan.Facts) ([]byte, []string) {
	var judgment []string
	note := func(format string, args ...any) { judgment = append(judgment, fmt.Sprintf(format, args...)) }
	heads := map[string]string{}
	items := map[string]Item{}
	for _, s := range b.Sections {
		heads[s.ID] = s.Heading
		for _, it := range s.Items {
			items[it.ID] = it
		}
	}
	text := func(s string) string {
		return sectionRef.ReplaceAllStringFunc(strings.TrimSpace(s), func(m string) string {
			if h, ok := heads[m[2:len(m)-2]]; ok {
				return h
			}
			return m
		})
	}
	line := func(s string) string { return strings.Join(strings.Fields(text(s)), " ") }
	tags := func(ids ...string) string {
		var sb strings.Builder
		var seen []string
		for _, id := range ids {
			if id != "" && !slices.Contains(seen, id) {
				seen = append(seen, id)
				sb.WriteString(" :fact[" + strings.TrimPrefix(id, "f:") + "]")
			}
		}
		return sb.String()
	}
	link := func(it Item) string {
		label := cmp.Or(line(it.Text), it.Target)
		return "See [" + label + "](" + it.Target + ")."
	}
	claim := func(it Item) string {
		if facts != nil {
			if f, ok := facts.Get(it.Fact); ok && f.Text != "" {
				return line(f.Text)
			}
		}
		note("fact item %s: the facts store gave no claim to place, so the bullet holds the item's use text", it.ID)
		return cmp.Or(line(it.Use), strings.TrimPrefix(it.Fact, "f:"))
	}

	handlers := map[string]bool{}
	for _, s := range b.Sections {
		for _, it := range s.Items {
			if h, ok := items[it.HandledBy]; ok && it.Kind == "failure" && h.Kind != "step" && h.Kind != "code" && h.Kind != "figure" {
				handlers[h.ID] = true
			}
		}
	}

	front := Front{Page: entry.Title, Type: entry.Type, Track: track}
	if len(b.Exemplars) > 0 {
		front.Exemplar = strings.TrimSuffix(b.Exemplars[0].Source, "/")
	}
	if len(b.Exemplars) > 1 {
		note("exemplars after the first, with their take, are dropped; the front matter names one")
	}
	fm, _ := yaml.Marshal(front) // a struct of strings always marshals

	var out strings.Builder
	out.WriteString("---\n" + string(fm) + "---\n\n# " + entry.Title + "\n\n:::reader\n")
	in, fr := b.Introduction, b.Framing
	for _, part := range []struct{ label, body, facts string }{
		{"Who reads it.", fr.Readers, ""},
		{"Background.", fr.Background, ""},
		{"Place in the doc set.", fr.PlaceInDocSet, ""},
		{"What the page does for them.", in.Covers, tags(in.Facts...)},
		{"What they already know.", in.PriorKnowledge, ""},
		{"What it leaves to other pages.", in.NotCovered, ""},
		{"How the introduction runs.", fr.IntroPlan, ""},
	} {
		if strings.TrimSpace(part.body) != "" {
			out.WriteString("**" + part.label + "** " + text(part.body) + part.facts + "\n\n")
		}
	}
	out.WriteString(":::\n\n")

	for _, s := range b.Sections {
		level := cmp.Or(s.Level, 2)
		out.WriteString(strings.Repeat("#", level) + " " + s.Heading + "\n\n")
		if why := line(s.Why); why != "" {
			out.WriteString("*" + why + "*\n\n")
		} else {
			note("section %s has no why, so it has no job line", s.ID)
		}
		var bullets []string
		flush := func() {
			if len(bullets) > 0 {
				out.WriteString(strings.Join(bullets, "\n") + "\n\n")
				bullets = nil
			}
		}
		if opens := line(s.Opens); opens != "" {
			bullets = append(bullets, "- "+opens)
		}
		step := 0
		for _, it := range s.Items {
			if handlers[it.ID] {
				continue
			}
			if it.Kind == "step" {
				step++
			} else {
				step = 0
			}
			var body string
			switch it.Kind {
			case "fact":
				body = claim(it) + tags(it.Fact)
			case "sentence", "example", "reason", "code", "figure":
				body = line(it.Text) + tags(it.Facts...)
			case "handoff", "step":
				body = line(it.Text)
			case "link":
				body = link(it)
			case "failure":
				flush()
				cause := line(it.Cause)
				if cause == "" {
					note("failure %s has no cause; the row's Cause part is empty", it.ID)
				}
				fix, ids := "", append([]string(nil), it.Facts...)
				if h, ok := items[it.HandledBy]; ok {
					switch h.Kind {
					case "fact":
						fix, ids = claim(h), append(ids, h.Fact)
					case "link":
						fix = link(h)
					default:
						fix, ids = line(h.Text), append(ids, h.Facts...)
					}
				}
				out.WriteString(":::row\n**Symptom.** " + line(it.Symptom) + "\n**Cause.** " + cause + "\n**Fix.** " + fix + tags(ids...) + "\n:::\n\n")
				continue
			}
			if it.Kind == "code" || it.Kind == "figure" {
				note("%s item %s becomes a bullet of its text; its source is not carried", it.Kind, it.ID)
			}
			if it.Kind == "step" {
				bullets = append(bullets, fmt.Sprintf("%d. %s", step, body))
			} else {
				bullets = append(bullets, "- "+body)
			}
		}
		flush()
	}

	for _, f := range []struct{ name, set string }{
		{"argument", b.Argument}, {"orderReason", b.OrderReason},
		{"preconditions", strings.Join(b.Preconditions, "")}, {"constraints", strings.Join(b.Constraints, "")},
	} {
		if strings.TrimSpace(f.set) != "" {
			note("%s is dropped; the markdown format has no field for it", f.name)
		}
	}
	if len(b.Dispositions) > 0 {
		note("dispositions are dropped; the markdown format records no fact a page leaves out")
	}
	return []byte(strings.TrimRight(out.String(), "\n") + "\n"), judgment
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

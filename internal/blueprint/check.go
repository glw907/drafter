package blueprint

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/stock"
)

// Names of the checks this package adds to plan's, carried in Finding.Check.
const (
	checkBlueprints = "blueprints"
	checkRefs       = "refs"
	checkWhy        = "why"
	checkUse        = "use"
	checkHandler    = "handler"
	checkTypes      = "types"
	checkLock       = "lock"
	checkConvention = "convention"
	checkFraming    = "framing"
)

// sectionRef matches a [[section-id]] reference in prose.
var sectionRef = regexp.MustCompile(`\[\[([^\[\]]*)\]\]`)

// Check validates the blueprints of the project offline. With no named files it checks every
// blueprint under the blueprint root, the pairing of blueprints with the outline, the lock, and
// the project's departures from the layout convention. With named files, each a repo-relative
// slash path, it checks only those blueprints and the outline and lock entries of their pages.
//
// The shared plan checks run on each blueprint's view. A blueprint that fails to decode reports
// its decode findings and the walk goes on to the next. The error means the command could not
// run: the blueprint root, the outline, the lock, or the facts directory is missing or
// malformed, or a named file is not a blueprint at its path-rule location. Findings come back
// grouped by file, in the order checked.
func (p Project) Check(named []string) ([]plan.Finding, error) {
	l, err := p.load()
	if err != nil {
		return nil, err
	}
	files := l.files
	if len(named) > 0 {
		files = nil
		for _, rel := range named {
			if _, ok := PagePath(p.BlueprintRoot, rel); !ok {
				return nil, fmt.Errorf("%s is not a %s file under the blueprint root %s", rel, strings.TrimPrefix(blueprintSuffix, "."), p.BlueprintRoot)
			}
			if _, err := os.Stat(p.abs(rel)); err != nil {
				return nil, fmt.Errorf("blueprint %s: %w", rel, err)
			}
			files = append(files, rel)
		}
	}
	c := &projectCheck{Project: p, l: l, usedTypes: map[string]bool{}}
	c.findings = append(c.findings, l.findings...)
	for _, rel := range files {
		c.blueprint(rel)
	}
	c.outlineChecks()
	if len(named) == 0 {
		c.pairing()
		c.convention()
	}
	c.lockEntries(files, len(named) > 0)
	return c.findings, nil
}

type projectCheck struct {
	Project
	l         *loaded
	findings  []plan.Finding
	usedTypes map[string]bool
}

func (c *projectCheck) add(file string, line int, severity, check, msg, fix string) {
	c.findings = append(c.findings, plan.Finding{File: file, Line: max(line, 1), Severity: severity, Check: check, Message: msg, Fix: fix})
}

// blueprint checks the blueprint at rel: its decode, the plan checks on its view, and the
// checks only a blueprint has.
func (c *projectCheck) blueprint(rel string) {
	data, err := os.ReadFile(c.abs(rel))
	if err != nil {
		c.add(rel, 1, plan.SeverityError, checkBlueprints, "the blueprint cannot be read: "+err.Error(), "fix the file's permissions")
		return
	}
	if page, _ := PagePath(c.BlueprintRoot, rel); strings.HasSuffix(rel, blueprintSuffix) && c.Locate(page) != rel {
		c.shadowed(rel)
		return
	}
	c.blueprintData(rel, data)
}

// CheckSource checks one blueprint, in either format, from its bytes against the outline's bytes,
// which a caller reads from a commit instead of the disk. rel is the blueprint's repo-relative
// slash path, whose suffix names the format. It returns the decoded blueprint and its outline
// entry when the checks ran far enough to find them, nil and a zero Page otherwise, and every
// finding. The error means the outline, the lock, or the facts directory could not be read.
// Pairing, the lock's entries, and the convention are the project walk's, not this check's.
func (p Project) CheckSource(rel string, blueprint, outline []byte) (Source, Page, []plan.Finding, error) {
	l, err := p.loadOutline(outline)
	if err != nil {
		return nil, Page{}, nil, err
	}
	c := &projectCheck{Project: p, l: l, usedTypes: map[string]bool{}}
	c.findings = append(c.findings, l.findings...)
	b, entry := c.blueprintData(rel, blueprint)
	return b, entry, c.findings, nil
}

// blueprintData is blueprint on bytes already read. It returns the blueprint and its entry when
// the file decoded cleanly and its page is a managed one the outline lists, and a nil Source
// otherwise.
func (c *projectCheck) blueprintData(rel string, data []byte) (Source, Page) {
	src, findings := Load(rel, data)
	c.findings = append(c.findings, findings...)
	pagePath, _ := PagePath(c.BlueprintRoot, rel)
	entry, listed := c.l.pages[pagePath]
	switch {
	case !listed:
		c.add(rel, 1, plan.SeverityError, checkBlueprints,
			fmt.Sprintf("the blueprint is for %s, which the outline lists no page for", pagePath),
			"add the page to "+c.Outline+", or move the blueprint to the path of a listed page")
		return nil, Page{}
	case !entry.Managed:
		c.add(rel, 1, plan.SeverityError, checkBlueprints,
			fmt.Sprintf("the blueprint is for %s, which the outline marks managed: false", pagePath),
			"delete the blueprint, or remove managed: false from the outline entry")
		return nil, Page{}
	}
	if src == nil || plan.HasError(findings) {
		return nil, Page{}
	}
	opts := plan.CheckOptions{Root: c.Root, FactsDir: c.FactsDir, Outline: c.l.plan}
	c.findings = append(c.findings, plan.Check(rel, src.View(entry, c.l.facts), opts)...)
	switch b := src.(type) {
	case *Markdown:
		c.markdown(rel, b, entry)
		c.exhaustive(rel, b)
	case *Blueprint:
		c.references(rel, b)
		c.sections(rel, b)
		c.framing(rel, b)
		c.handlers(rel, b, entry.Title)
	}
	c.pageType(entry)
	return src, entry
}

// markdown runs the checks only a markdown blueprint has, each finding at its line: the front
// matter's keys and their agreement with the outline, one reader block, a job line on every
// section, three parts on every row, and a heading slug used once.
func (c *projectCheck) markdown(rel string, m *Markdown, entry Page) {
	fault := func(line int, msg, fix string) { c.add(rel, line, plan.SeverityError, checkMarkdown, msg, fix) }
	f := m.Front
	for _, key := range [...]struct{ name, val string }{{"page", f.Page}, {"type", f.Type}} {
		if strings.TrimSpace(key.val) == "" {
			fault(f.Line, fmt.Sprintf("the front matter has no %s", key.name), "add "+key.name+": to the front matter")
		}
	}
	if f.Page != "" && entry.Title != "" && f.Page != entry.Title {
		fault(f.Line, fmt.Sprintf("the front matter's page is %q, but the outline titles %s %q", f.Page, entry.Path, entry.Title),
			"set page to the outline's title, or change the title in the outline")
	}
	if f.Type != "" && entry.Type != f.Type {
		fault(f.Line, fmt.Sprintf("the front matter's type is %q, but the outline gives %s the type %q", f.Type, entry.Path, entry.Type),
			"set type to the outline's type for the page, or change the entry in the outline")
	}
	c.track(rel, f, entry)

	if len(m.ReaderLines) == 0 {
		fault(1, "the blueprint has no :::reader block", "add a :::reader block under the title that says who reads the page and why")
	} else {
		for _, line := range m.ReaderLines[1:] {
			fault(line, fmt.Sprintf("the page has a second :::reader block (the first is at line %d)", m.ReaderLines[0]), "keep one :::reader block and merge this one into it")
		}
	}

	for i, line := range m.ConstraintLines {
		switch {
		case i > 0:
			fault(line, fmt.Sprintf("the page has a second :::constraints block (the first is at line %d)", m.ConstraintLines[0]), "keep one :::constraints block and merge this one into it")
		case len(m.ReaderLines) == 0 || line < m.ReaderLines[0] || (len(m.Sections) > 0 && line > m.Sections[0].Line):
			fault(line, "the :::constraints block is not between the :::reader block and the first heading", "move the block under the :::reader block and above the first section")
		}
	}
	for _, line := range m.BareFences {
		fault(line, "this code fence has no Code: or Figure: bullet directly above it", "put a Code: or Figure: bullet above the fence that says what the block shows")
	}

	seen := map[string]int{}
	for _, s := range m.Sections {
		if s.Job == "" {
			fault(s.Line, fmt.Sprintf("the section %q has no job line", s.Heading), "add an italic line under the heading that says what the section does for the reader")
		}
		slug := plan.HeadingSlugs("", []string{s.Heading})[0]
		if at, dup := seen[slug]; dup {
			fault(s.Line, fmt.Sprintf("the heading %q has the same slug as the heading at line %d", s.Heading, at), "reword one of the two headings")
		} else {
			seen[slug] = s.Line
		}
		for _, r := range s.Rows {
			for _, part := range [...]struct{ name, val string }{{"Symptom", r.Symptom}, {"Cause", r.Cause}, {"Fix", r.Fix}} {
				if part.val == "" {
					fault(r.Line, fmt.Sprintf("the row has no **%s.** part", part.name), "add a **"+part.name+".** line to the row")
				}
			}
		}
	}
}

// track checks the front matter's track against the outline's. An outline that declares no tracks
// leaves a track optional and any name an error; one that declares tracks requires a track the
// outline declares and whose paths hold the page.
func (c *projectCheck) track(rel string, f Front, entry Page) {
	tracks := c.l.outline.Tracks
	if len(tracks) == 0 {
		if f.Track != "" {
			c.add(rel, f.Line, plan.SeverityError, checkMarkdown,
				fmt.Sprintf("the front matter names the track %q, but the outline declares no tracks", f.Track),
				"remove track from the front matter, or declare tracks in "+c.Outline)
		}
		return
	}
	if f.Track == "" {
		c.add(rel, f.Line, plan.SeverityError, checkMarkdown, "the front matter has no track", "add track: to the front matter")
		return
	}
	i := slices.IndexFunc(tracks, func(t Track) bool { return t.Name == f.Track })
	switch {
	case i < 0:
		c.add(rel, f.Line, plan.SeverityError, checkMarkdown,
			fmt.Sprintf("the front matter's track %q is not a track the outline declares", f.Track), "name a track from "+c.Outline)
	case !slices.Contains(matches(tracks, path.Clean(entry.Path)), i):
		c.add(rel, f.Line, plan.SeverityError, checkMarkdown,
			fmt.Sprintf("the paths of the track %q do not hold %s", f.Track, entry.Path),
			"name the track whose paths hold the page, or add the page to this track's paths")
	}
}

// exhaustive warns on each line of a markdown blueprint that claims a complete set: bullets and
// steps, the three parts of a row, and the lines of the reader and constraints blocks.
func (c *projectCheck) exhaustive(rel string, m *Markdown) {
	warn := func(line int, text string) {
		if claim := plan.ExhaustiveClaim(text); claim != "" {
			c.findings = append(c.findings, plan.ExhaustiveFinding(rel, line, claim))
		}
	}
	lines := strings.Split(strings.ReplaceAll(m.Text, "\r\n", "\n"), "\n")
	for _, open := range []int{m.Reader.Line, m.Constraints.Line} {
		for n := open + 1; open > 0 && n <= len(lines) && strings.TrimSpace(lines[n-1]) != ":::"; n++ {
			warn(n, sourceTag.ReplaceAllString(factTag.ReplaceAllString(lines[n-1], ""), ""))
		}
	}
	for _, s := range m.Sections {
		for _, l := range s.Lines {
			warn(l.Line, l.Text)
		}
		for _, r := range s.Rows {
			warn(r.Line, strings.Join([]string{r.Symptom, r.Cause, r.Fix}, " "))
		}
	}
}

// framing requires the framing record to plan the page's introduction. The decoder does not, so a
// converted blueprint decodes and this check names what is left to write.
func (c *projectCheck) framing(file string, b *Blueprint) {
	switch {
	case b.Lines["framing"] == 0:
		c.add(file, 1, plan.SeverityError, checkFraming,
			"the blueprint has no framing, so it plans no introduction", "add framing with an introPlan")
	case b.Framing.IntroPlan == "":
		c.add(file, b.Lines["framing"], plan.SeverityError, checkFraming,
			"framing has no introPlan, so the blueprint plans no introduction",
			"add framing.introPlan: who arrives, what they know, why they read, and what the page covers and leaves out")
	}
}

// sections requires a why on every section and an Only or Omit opening on every use.
func (c *projectCheck) sections(file string, b *Blueprint) {
	for _, s := range b.Sections {
		if strings.TrimSpace(s.Why) == "" {
			c.add(file, cmp.Or(s.Lines["why"], s.Line), plan.SeverityError, checkWhy,
				fmt.Sprintf("section %q has no why", s.ID), "say why the section is on the page and why it sits here")
		}
		for _, it := range s.Items {
			if use := strings.TrimSpace(it.Use); use != "" && !opensWith(use, "Only", "Omit") {
				c.add(file, cmp.Or(it.Lines["use"], it.Line), plan.SeverityError, checkUse,
					fmt.Sprintf("item %q has a use that does not open with Only or Omit", it.ID),
					"start the use with Only (the part to place) or Omit (the part to leave out)")
			}
		}
	}
}

// handlers warns on a failure whose handler gives the reader no next action: a fact, a verbatim
// sentence, or a link or handoff that sends the reader back to the section holding the step the
// failure comes from. Only a step, or a link or handoff elsewhere, moves the reader on.
func (c *projectCheck) handlers(file string, b *Blueprint, title string) {
	headings := make([]string, len(b.Sections))
	for i, s := range b.Sections {
		headings[i] = s.Heading
	}
	slugs := plan.HeadingSlugs(title, headings)
	anchors := map[string]string{} // section id to anchor
	home := map[string]string{}    // item id to the id of its section
	items := map[string]Item{}
	for i, s := range b.Sections {
		anchors[s.ID] = cmp.Or(s.Anchor, slugs[i])
		for _, it := range s.Items {
			home[it.ID] = s.ID
			items[it.ID] = it
		}
	}
	for _, s := range b.Sections {
		for _, it := range s.Items {
			if it.Kind != "failure" {
				continue
			}
			h, ok := items[it.HandledBy]
			if !ok {
				continue
			}
			var what string
			switch {
			case h.Kind == "fact":
				what = "a fact"
			case h.Kind == "sentence" && h.Mode == "verbatim":
				what = "a verbatim sentence"
			case (h.Kind == "link" || h.Kind == "handoff") && it.From != "" && sendsBack(h, home[it.From], anchors):
				what = "a " + h.Kind + " back to the section of " + it.From
			default:
				continue
			}
			c.add(file, cmp.Or(it.Lines["handledBy"], it.Line), plan.SeverityWarning, checkHandler,
				fmt.Sprintf("failure %q is handled by %s (%s), which gives the reader no next action; hand off to the step or section that does", it.ID, what, h.ID),
				"make the handler a step, or point it at the section that gives the reader a next action")
		}
	}
}

// anchorRef matches a #anchor reference in prose or a link target.
var anchorRef = regexp.MustCompile(`#([^\s)\]]+)`)

// sendsBack reports whether a link or handoff refers to the section with the given id and to no
// other section, by [[id]] or #anchor in its target or text. A [[id]] in a link's target is no
// path the links check resolves, so it belongs in the text.
func sendsBack(h Item, section string, anchors map[string]string) bool {
	text := h.Target + " " + h.Text
	named := map[string]bool{}
	for _, m := range sectionRef.FindAllStringSubmatch(text, -1) {
		named[m[1]] = true
	}
	for _, m := range anchorRef.FindAllStringSubmatch(text, -1) {
		for id, anchor := range anchors {
			if anchor == m[1] {
				named[id] = true
			}
		}
	}
	return len(named) == 1 && named[section]
}

// opensWith reports whether s starts with one of the words, as a whole word.
func opensWith(s string, words ...string) bool {
	for _, w := range words {
		if rest, ok := strings.CutPrefix(s, w); ok && (rest == "" || !unicode.IsLetter([]rune(rest)[0])) {
			return true
		}
	}
	return false
}

// references resolves every [[id]] in a prose field against the blueprint's section ids.
func (c *projectCheck) references(file string, b *Blueprint) {
	ids := map[string]bool{}
	for _, s := range b.Sections {
		ids[s.ID] = true
	}
	scan := func(line int, text string) {
		for _, m := range sectionRef.FindAllStringSubmatch(text, -1) {
			if !ids[m[1]] {
				c.add(file, line, plan.SeverityError, checkRefs,
					fmt.Sprintf("the reference [[%s]] names no section id", m[1]), "name a section id from this blueprint")
			}
		}
	}
	scan(b.Lines["argument"], b.Argument)
	scan(b.Lines["orderReason"], b.OrderReason)
	in := b.Introduction
	scan(in.Lines["covers"], in.Covers)
	scan(in.Lines["priorKnowledge"], in.PriorKnowledge)
	scan(in.Lines["notCovered"], in.NotCovered)
	for _, part := range framingParts(&b.Framing) {
		scan(b.Framing.Lines[part.name], part.text)
	}
	for _, text := range b.Constraints {
		scan(b.Lines["constraints"], text)
	}
	for _, s := range b.Sections {
		scan(cmp.Or(s.Lines["why"], s.Line), s.Why)
		scan(cmp.Or(s.Lines["opens"], s.Line), s.Opens)
		for _, it := range s.Items {
			for _, f := range [...]struct{ field, text string }{{"text", it.Text}, {"use", it.Use}, {"symptom", it.Symptom}, {"cause", it.Cause}} {
				scan(cmp.Or(it.Lines[f.field], it.Line), f.text)
			}
		}
	}
}

// pageType checks, once per page, that its type is declared and that the type names an anatomy.
func (c *projectCheck) pageType(entry Page) {
	var decl *Type
	for i := range c.l.outline.Types {
		if c.l.outline.Types[i].Name == entry.Type {
			decl = &c.l.outline.Types[i]
		}
	}
	line := cmp.Or(entry.Lines["type"], entry.Line)
	switch {
	case decl == nil:
		c.add(c.Outline, line, plan.SeverityError, checkTypes,
			fmt.Sprintf("page %s has the type %q, which types declares no entry for", entry.Path, entry.Type),
			"declare the type under types, or use a declared one")
	case decl.Anatomy == "" && !c.usedTypes[decl.Name]:
		c.add(c.Outline, cmp.Or(decl.Line, 1), plan.SeverityError, checkTypes,
			fmt.Sprintf("the type %q names no anatomy", decl.Name),
			"set anatomy to a stock anatomy, one of "+strings.Join(stock.Anatomies, ", ")+", or to an entry in the register of the track that holds the type's pages")
	}
	c.usedTypes[entry.Type] = true
}

// shadowed reports the .blueprint.yaml at rel, whose page also has a .blueprint.md that wins.
func (c *projectCheck) shadowed(rel string) {
	c.add(rel, 1, plan.SeverityError, checkBlueprints,
		"the page has both a .blueprint.md and this .blueprint.yaml, and the markdown file is the blueprint",
		"delete this file, or run drafter convert markdown to replace it")
}

// pairing reports blueprint-like files at no path-rule location and managed pages without a
// blueprint. The second is a warning until blueprints.required is set.
func (c *projectCheck) pairing() {
	for _, rel := range c.l.strays {
		c.add(rel, 1, plan.SeverityError, checkBlueprints,
			"the file is not at a path-rule location, so no page maps to it",
			"name it <page path without .md>.blueprint.md (or .blueprint.yaml) under "+c.BlueprintRoot)
	}
	for _, rel := range c.l.shadowed {
		c.shadowed(rel)
	}
	have := map[string]bool{}
	for _, rel := range c.l.files {
		pg, _ := PagePath(c.BlueprintRoot, rel)
		have[pg] = true
	}
	severity := plan.SeverityWarning
	if c.Required {
		severity = plan.SeverityError
	}
	for _, pg := range c.l.outline.Pages {
		if pg.Managed && !have[pg.Path] {
			c.add(c.Outline, cmp.Or(pg.Lines["path"], pg.Line), severity, checkBlueprints,
				fmt.Sprintf("the managed page %s has no blueprint at %s", pg.Path, BlueprintPath(c.BlueprintRoot, pg.Path)),
				"write the blueprint, run drafter convert, or mark the page managed: false")
		}
	}
}

// convention warns while the project's paths depart from the layout the format describes.
func (c *projectCheck) convention() {
	if c.BlueprintRoot != DefaultRoot {
		c.add("drafter.toml", 1, plan.SeverityWarning, checkConvention,
			fmt.Sprintf("blueprints.root is %q, not the conventional %q", c.BlueprintRoot, DefaultRoot),
			"move the blueprint tree to "+DefaultRoot+" and drop the setting once the migration ends")
	}
	if c.FactsDir == "" {
		return
	}
	if want := c.abs(path.Join(c.BlueprintRoot, "facts")); c.FactsDir != want {
		c.add("drafter.toml", 1, plan.SeverityWarning, checkConvention,
			fmt.Sprintf("facts.dir is %s, not the conventional %s/facts", c.FactsDir, c.BlueprintRoot),
			"move the facts store to "+c.BlueprintRoot+"/facts and drop the setting once the migration ends")
	}
}

// lockEntries checks drafted.lock against the disk and the outline. With scoped true only the
// entries for the pages of files are checked.
func (c *projectCheck) lockEntries(files []string, scoped bool) {
	keep := map[string]bool{}
	for _, rel := range files {
		pg, _ := PagePath(c.BlueprintRoot, rel)
		keep[pg] = true
	}
	for _, pg := range slices.Sorted(maps.Keys(c.l.lock.Pages)) {
		if scoped && !keep[pg] {
			continue
		}
		e := c.l.lock.Pages[pg]
		switch {
		case e.Page != "":
			if _, err := os.Stat(c.abs(pg)); err != nil {
				c.add(c.l.lockRel, 1, plan.SeverityError, checkLock,
					fmt.Sprintf("the entry for %s records a drafted page, but the page is not on disk", pg),
					"restore the page, or remove the entry")
			}
		case e.Accepted != "" || e.Blocked != nil:
			if _, ok := c.l.pages[pg]; !ok {
				what := "an accepted blueprint"
				if e.Accepted == "" {
					what = "a blocked read"
				}
				c.add(c.l.lockRel, 1, plan.SeverityError, checkLock,
					fmt.Sprintf("the entry for %s records %s, but the outline lists no such page", pg, what),
					"add the page to "+c.Outline+", or remove the entry")
			}
		}
	}
}

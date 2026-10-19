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
	c.blueprintData(rel, data)
}

// CheckSource checks one blueprint from its bytes against the outline's bytes, which a caller
// reads from a commit instead of the disk. rel is the blueprint's repo-relative slash path. It
// returns the decoded blueprint and its outline entry when the checks ran far enough to find
// them, nil and a zero Page otherwise, and every finding. The error means the outline, the lock,
// or the facts directory could not be read. Pairing, the lock's entries, and the convention are
// the project walk's, not this check's.
func (p Project) CheckSource(rel string, blueprint, outline []byte) (*Blueprint, Page, []plan.Finding, error) {
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
// the file decoded cleanly and its page is a managed one the outline lists.
func (c *projectCheck) blueprintData(rel string, data []byte) (*Blueprint, Page) {
	b, findings := Decode(rel, data)
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
	if b == nil || plan.HasError(findings) {
		return nil, Page{}
	}
	opts := plan.CheckOptions{Root: c.Root, FactsDir: c.FactsDir, Outline: c.l.plan}
	c.findings = append(c.findings, plan.Check(rel, View(b, entry, c.l.facts), opts)...)
	c.references(rel, b)
	c.sections(rel, b)
	c.framing(rel, b)
	c.handlers(rel, b, entry.Title)
	c.pageType(entry)
	return b, entry
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

// pairing reports blueprint-like files at no path-rule location and managed pages without a
// blueprint. The second is a warning until blueprints.required is set.
func (c *projectCheck) pairing() {
	for _, rel := range c.l.strays {
		c.add(rel, 1, plan.SeverityError, checkBlueprints,
			"the file is not at a path-rule location, so no page maps to it",
			"name it <page path without .md>.blueprint.yaml under "+c.BlueprintRoot)
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

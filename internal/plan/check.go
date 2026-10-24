package plan

import (
	"cmp"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Names of the checks, carried in Finding.Check.
const (
	checkIDs         = "ids"
	checkReaderState = "reader-state"
	checkRecovery    = "recovery"
	checkLinks       = "links"
	checkFacts       = "facts"
)

// CheckOptions says where the checks look. Root is the repository root that link targets and
// the page path resolve against. FactsDir is the facts directory, empty when the project has no
// facts store; callers confirm a set directory with CheckFactsDir first. Outline is optional.
type CheckOptions struct {
	Root     string
	FactsDir string
	Outline  *Outline
}

type checker struct {
	file     string
	p        *Plan
	opts     CheckOptions
	findings []Finding
	items    map[string]*Item
	anchors  map[string]map[string]bool
}

func (c *checker) add(line int, severity, check, msg, fix string) {
	c.findings = append(c.findings, Finding{
		File: c.file, Line: line, Severity: severity, Check: check, Message: msg, Fix: fix,
	})
}

// Check runs the plan checks over a plan that decoded without error: id integrity, the
// reader-state walk, the recovery matrix, links and anchors, and, when opts.FactsDir is set,
// fact citability. Findings come back sorted by line, then message.
func Check(file string, p *Plan, opts CheckOptions) []Finding {
	c := &checker{file: file, p: p, opts: opts, items: map[string]*Item{}, anchors: map[string]map[string]bool{}}
	c.ids()
	c.readerState()
	c.recovery()
	c.links()
	c.crossLinks()
	c.facts()
	slices.SortStableFunc(c.findings, func(a, b Finding) int {
		return cmp.Or(cmp.Compare(a.Line, b.Line), cmp.Compare(a.Message, b.Message))
	})
	return c.findings
}

// each calls fn for every item in page order.
func (c *checker) each(fn func(it *Item)) {
	for i := range c.p.Sections {
		for j := range c.p.Sections[i].Items {
			fn(&c.p.Sections[i].Items[j])
		}
	}
}

func (c *checker) ids() {
	c.each(func(it *Item) {
		if first, dup := c.items[it.ID]; dup {
			c.add(it.Lines["id"], SeverityError, checkIDs,
				fmt.Sprintf("id %q repeats the id of the item at line %d", it.ID, first.Line), "give each item its own id")
			return
		}
		c.items[it.ID] = it
	})
	placed := map[string]*Item{}
	c.each(func(it *Item) {
		if it.Kind == "fact" && placed[it.Fact] == nil {
			placed[it.Fact] = it
		}
		if it.Kind != "failure" {
			return
		}
		switch from := c.items[it.From]; {
		case it.From == "":
		case from == nil:
			c.add(it.Lines["from"], SeverityError, checkIDs,
				fmt.Sprintf("from names %q, which is no item's id", it.From), "name the step this failure belongs to")
		case from.Kind != "step":
			c.add(it.Lines["from"], SeverityError, checkIDs,
				fmt.Sprintf("from names %q, a %s item, not a step", it.From, from.Kind), "name the step this failure belongs to")
		}
		if c.items[it.HandledBy] == nil {
			c.add(it.Lines["handledBy"], SeverityError, checkIDs,
				fmt.Sprintf("handledBy names %q, which is no item's id", it.HandledBy), "name the item that handles this failure")
		}
	})
	disposed := map[string]int{}
	for i := range c.p.Dispositions {
		d := &c.p.Dispositions[i]
		disposed[d.Fact] = d.Lines["fact"]
		if it := placed[d.Fact]; it != nil {
			c.add(d.Lines["fact"], SeverityError, checkIDs,
				fmt.Sprintf("fact %q is placed in the page by the item at line %d and also disposed here", d.Fact, it.Line),
				"keep the fact as an item or as a disposition, not both")
		}
	}
	c.eachFactsList(func(line int, id string) {
		if at, ok := disposed[id]; ok {
			c.add(line, SeverityError, checkIDs,
				fmt.Sprintf("fact %q is cited here but disposed at line %d", id, at),
				"cite a placed fact, or place this one and drop its disposition")
		}
	})
}

// eachFactsList calls fn for every id in the introduction's facts list and in each item's, with
// the line of the list.
func (c *checker) eachFactsList(fn func(line int, id string)) {
	for _, id := range c.p.Introduction.Facts {
		fn(c.p.Introduction.Lines["facts"], id)
	}
	c.each(func(it *Item) {
		for _, id := range it.Facts {
			fn(it.Lines["facts"], id)
		}
	})
}

func (c *checker) readerState() {
	produced := map[string]bool{}
	optional := map[string]bool{}
	for _, s := range c.p.Preconditions {
		produced[s] = true
	}
	firstProducer := map[string]int{}
	c.each(func(it *Item) {
		for _, s := range it.Produces {
			if _, ok := firstProducer[s]; !ok {
				firstProducer[s] = it.Line
			}
		}
	})
	c.each(func(it *Item) {
		if it.Kind != "step" {
			return
		}
		for _, r := range it.Requires {
			if produced[r] {
				continue
			}
			if optional[r] {
				if !it.Optional {
					c.add(it.Lines["requires"], SeverityWarning, checkReaderState,
						fmt.Sprintf("step %q requires %q, which only optional steps produce before it, so a reader who skips them lacks it", it.ID, r),
						"make the producing step required, or list the state under preconditions")
				}
				continue
			}
			msg := fmt.Sprintf("step %q requires %q, which no earlier step produces and no precondition lists", it.ID, r)
			if line, ok := firstProducer[r]; ok {
				msg += fmt.Sprintf(" (the step at line %d produces it, but not before this one)", line)
			}
			c.add(it.Lines["requires"], SeverityError, checkReaderState, msg,
				"reorder the steps, or list the state under preconditions")
		}
		for _, s := range it.Produces {
			if it.Optional {
				optional[s] = true
			} else {
				produced[s] = true
			}
		}
	})
}

func (c *checker) recovery() {
	handled := map[string]bool{}
	c.each(func(it *Item) {
		if it.Kind != "failure" {
			return
		}
		handled[it.From] = true
		if from := c.items[it.From]; from != nil && from.Kind == "step" && !from.CanFail {
			c.add(it.Lines["from"], SeverityWarning, checkRecovery,
				fmt.Sprintf("failure %q follows step %q, which is not marked canFail", it.ID, it.From),
				"add canFail: true to the step, or remove from to make the failure free-standing")
		}
	})
	c.each(func(it *Item) {
		if it.Kind == "step" && it.CanFail && !handled[it.ID] {
			c.add(it.Lines["canFail"], SeverityError, checkRecovery,
				fmt.Sprintf("step %q can fail, but no failure item names it in from", it.ID),
				"add a failure item with from: "+it.ID+", or remove canFail")
		}
	})
}

func (c *checker) links() {
	own := c.pageAnchors()
	c.each(func(it *Item) {
		switch it.Kind {
		case "link":
			c.link(it.Lines["target"], it.Target, own)
		case "code", "figure":
			if it.Source != "" && !isURL(it.Source) {
				c.codeSource(it.Lines["source"], it.Source)
			}
		}
	})
	c.anchorPins()
	for i := range c.p.Dispositions {
		if d := &c.p.Dispositions[i]; d.Disposition == "subordinated" {
			c.link(d.Lines["reference"], d.Reference, own)
		}
	}
}

// pageAnchors returns the slugs of the page's title and section headings, in page order through
// one slugger, so a repeated heading takes -1 as the rendered page will.
func (c *checker) pageAnchors() map[string]bool {
	own := map[string]bool{}
	for _, s := range c.slugs() {
		own[s] = true
	}
	return own
}

// slugs returns the slug of the page title followed by one per section heading, in page order.
func (c *checker) slugs() []string {
	sl := newSlugger()
	out := []string{sl.slug(c.p.Page.Title)}
	for i := range c.p.Sections {
		out = append(out, sl.slug(c.p.Sections[i].Heading))
	}
	return out
}

// anchorPins checks that each section's anchor, when the plan pins one, is the slug its heading
// renders with.
func (c *checker) anchorPins() {
	slugs := c.slugs()[1:]
	for i := range c.p.Sections {
		s := &c.p.Sections[i]
		if s.Anchor != "" && s.Anchor != slugs[i] {
			c.add(s.Lines["anchor"], SeverityError, checkLinks,
				fmt.Sprintf("anchor %q does not match %q, the slug of the heading %q", s.Anchor, slugs[i], s.Heading),
				"write the heading's slug, or correct the heading")
		}
	}
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// resolveFile returns the repo-relative path a link or source path names, resolved from the page's
// directory, or from the repository root when it starts with a slash.
func (c *checker) resolveFile(file string) string {
	if rest, ok := strings.CutPrefix(file, "/"); ok {
		return path.Clean(rest)
	}
	return path.Join(path.Dir(c.p.Page.Path), file)
}

// crossLinks warns about each outline cross-link from this page that no link item follows, and
// about a cross-link whose target the outline does not list. The outline's own pages and slugs
// resolve both ends, and a cross-link from another page is that page's to check.
func (c *checker) crossLinks() {
	o := c.opts.Outline
	if o == nil || !o.lists(path.Clean(c.p.Page.Path)) {
		return
	}
	linked := map[string]bool{}
	c.each(func(it *Item) {
		if file, _, _ := strings.Cut(it.Target, "#"); it.Kind == "link" && file != "" && !isURL(it.Target) {
			linked[c.resolveFile(file)] = true
		}
	})
	line := c.p.Page.Lines["path"]
	for _, x := range o.crossLinks {
		if from, ok := o.resolve(x.From); !ok || from != path.Clean(c.p.Page.Path) {
			continue
		}
		to, ok := o.resolve(x.To)
		switch {
		case !ok:
			c.add(line, SeverityWarning, checkLinks,
				fmt.Sprintf("the outline cross-links this page to %q (%s), which names no page the outline lists", x.To, x.Why),
				"correct the cross-link in the outline, or add the page to it")
		case !linked[to]:
			c.add(line, SeverityWarning, checkLinks,
				fmt.Sprintf("the outline cross-links this page to %q (%s), and no link item targets %s", x.To, x.Why, to),
				"add a link item to "+to+", or drop the cross-link from the outline")
		}
	}
}

// repoPath resolves a link or source path from the page's directory, or from the repository root
// when it starts with a slash. It reports a finding and returns false when the path leaves the
// repository.
func (c *checker) repoPath(line int, file string) (string, bool) {
	rel := c.resolveFile(file)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		c.add(line, SeverityError, checkLinks,
			fmt.Sprintf("target %s leaves the repository", file), "point the link at a file under the repository root")
		return "", false
	}
	return rel, true
}

// codeSource checks that the path part of a code or figure item's source exists. The #Symbol part is not
// checked.
func (c *checker) codeSource(line int, source string) {
	file, _, _ := strings.Cut(source, "#")
	rel, ok := c.repoPath(line, file)
	if !ok {
		return
	}
	if _, err := os.Stat(filepath.Join(c.opts.Root, filepath.FromSlash(rel))); err != nil {
		c.add(line, SeverityError, checkLinks,
			fmt.Sprintf("source %s does not exist", rel), "correct the path to the file the code block shows")
	}
}

func (c *checker) link(line int, target string, own map[string]bool) {
	if isURL(target) {
		return
	}
	file, anchor, _ := strings.Cut(target, "#")
	if file == "" {
		if anchor == "" {
			c.add(line, SeverityError, checkLinks,
				fmt.Sprintf("target %q names neither an anchor nor a path", target), "name an anchor or a path")
			return
		}
		if !own[anchor] {
			c.add(line, SeverityError, checkLinks,
				fmt.Sprintf("anchor #%s matches no heading on this page", anchor),
				"use the slug of the page title or of a section heading")
		}
		return
	}
	rel, ok := c.repoPath(line, file)
	if !ok {
		return
	}
	info, err := os.Stat(filepath.Join(c.opts.Root, filepath.FromSlash(rel)))
	switch {
	case os.IsNotExist(err):
		if c.opts.Outline.lists(rel) {
			c.add(line, SeverityWarning, checkLinks,
				fmt.Sprintf("target %s is in the outline but not on disk yet, so its anchor is not checked", rel),
				"write the page before the link ships")
			return
		}
		c.add(line, SeverityError, checkLinks,
			fmt.Sprintf("target %s does not exist", rel), "correct the path, or add the page to the outline if it is still to be written")
		return
	case err != nil:
		c.add(line, SeverityError, checkLinks, fmt.Sprintf("target %s cannot be read: %v", rel, err), "fix the file's permissions")
		return
	case anchor == "":
		return
	case info.IsDir():
		c.add(line, SeverityError, checkLinks,
			fmt.Sprintf("target %s is a directory, which has no headings for #%s to match", rel, anchor),
			"link to a file in the directory, or drop the anchor")
		return
	}
	anchors, ok := c.anchors[rel]
	if !ok {
		data, err := os.ReadFile(filepath.Join(c.opts.Root, filepath.FromSlash(rel)))
		if err != nil {
			c.add(line, SeverityError, checkLinks, fmt.Sprintf("target %s cannot be read: %v", rel, err), "fix the file's permissions")
			return
		}
		anchors = anchorsOf(data)
		c.anchors[rel] = anchors
	}
	if !anchors[anchor] {
		c.add(line, SeverityError, checkLinks,
			fmt.Sprintf("anchor #%s matches no heading in %s", anchor, rel),
			"use the slug of a heading in that file, or add the heading")
	}
}

func (c *checker) facts() {
	if c.opts.FactsDir == "" {
		if line := c.p.Introduction.Lines["facts"]; len(c.p.Introduction.Facts) > 0 {
			c.add(line, SeverityWarning, checkFacts,
				fmt.Sprintf("the facts list at line %d is not checked because no facts directory is set", line),
				"pass --facts <dir> to check that each fact exists and is citable")
		}
		c.each(func(it *Item) {
			switch {
			case it.Kind == "fact":
				c.add(it.Lines["fact"], SeverityWarning, checkFacts,
					fmt.Sprintf("the fact item at line %d is not checked because no facts directory is set", it.Lines["fact"]),
					"pass --facts <dir> to check that the fact exists and is citable")
			case len(it.Facts) > 0:
				c.add(it.Lines["facts"], SeverityWarning, checkFacts,
					fmt.Sprintf("the facts list at line %d is not checked because no facts directory is set", it.Lines["facts"]),
					"pass --facts <dir> to check that each fact exists and is citable")
			}
		})
		return
	}
	facts, err := LoadFacts(c.opts.FactsDir)
	if err != nil {
		c.add(1, SeverityError, checkFacts, "the facts directory cannot be read: "+err.Error(), "fix the facts directory path or its permissions")
		return
	}
	resolve := func(line int, id string) (Fact, bool) {
		found := facts.byID[id]
		switch len(found) {
		case 0:
			c.add(line, SeverityError, checkFacts,
				fmt.Sprintf("fact %q is in no facts file", id), "correct the id, or file the fact in the facts directory")
			return Fact{}, false
		case 1:
			return found[0], true
		}
		places := make([]string, len(found))
		for i, f := range found {
			places[i] = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		c.add(line, SeverityError, checkFacts,
			fmt.Sprintf("fact %q is on %d bullets (%s)", id, len(found), strings.Join(places, ", ")),
			"keep one bullet per id")
		return Fact{}, false
	}
	cite := func(line int, id string) {
		f, ok := resolve(line, id)
		switch {
		case !ok || f.Citable():
		case f.Problem != "":
			c.add(line, SeverityError, checkFacts,
				fmt.Sprintf("fact %q cannot be cited: %s", id, f.Problem), "correct the bullet's status tag")
		default:
			c.add(line, SeverityError, checkFacts,
				fmt.Sprintf("fact %q is tagged [%s], which a page cannot cite", id, f.Tag),
				"place a verified, external, or vendor fact, or retag the bullet once it is checked")
		}
	}
	c.eachFactsList(cite)
	c.each(func(it *Item) {
		if it.Kind == "fact" {
			cite(it.Lines["fact"], it.Fact)
		}
	})
	for i := range c.p.Dispositions {
		resolve(c.p.Dispositions[i].Lines["fact"], c.p.Dispositions[i].Fact)
	}
}

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
	for i := range c.p.Dispositions {
		d := &c.p.Dispositions[i]
		if it := placed[d.Fact]; it != nil {
			c.add(d.Lines["fact"], SeverityError, checkIDs,
				fmt.Sprintf("fact %q is placed in the page by the item at line %d and also disposed here", d.Fact, it.Line),
				"keep the fact as an item or as a disposition, not both")
		}
	}
}

func (c *checker) readerState() {
	produced := map[string]bool{}
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
			msg := fmt.Sprintf("step %q requires %q, which no earlier step produces and no precondition lists", it.ID, r)
			if line, ok := firstProducer[r]; ok {
				msg += fmt.Sprintf(" (the step at line %d produces it, but not before this one)", line)
			}
			c.add(it.Lines["requires"], SeverityError, checkReaderState, msg,
				"reorder the steps, or list the state under preconditions")
		}
		for _, s := range it.Produces {
			produced[s] = true
		}
	})
}

func (c *checker) recovery() {
	handled := map[string]bool{}
	c.each(func(it *Item) {
		if it.Kind == "failure" {
			handled[it.From] = true
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
		if it.Kind == "link" {
			c.link(it.Lines["target"], it.Target, own)
		}
	})
	for i := range c.p.Dispositions {
		if d := &c.p.Dispositions[i]; d.Disposition == "subordinated" {
			c.link(d.Lines["reference"], d.Reference, own)
		}
	}
}

// pageAnchors returns the slugs of the page's title and section headings, in page order through
// one slugger, so a repeated heading takes -1 as the rendered page will.
func (c *checker) pageAnchors() map[string]bool {
	sl := newSlugger()
	own := map[string]bool{sl.slug(c.p.Page.Title): true}
	for i := range c.p.Sections {
		own[sl.slug(c.p.Sections[i].Heading)] = true
	}
	return own
}

func (c *checker) link(line int, target string, own map[string]bool) {
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return
	}
	file, anchor, _ := strings.Cut(target, "#")
	if file == "" {
		if anchor != "" && !own[anchor] {
			c.add(line, SeverityError, checkLinks,
				fmt.Sprintf("anchor #%s matches no heading on this page", anchor),
				"use the slug of the page title or of a section heading")
		}
		return
	}
	rel := path.Join(path.Dir(c.p.Page.Path), file)
	if rest, ok := strings.CutPrefix(file, "/"); ok {
		rel = path.Clean(rest)
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		c.add(line, SeverityError, checkLinks,
			fmt.Sprintf("target %s leaves the repository", file), "point the link at a file under the repository root")
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
		c.each(func(it *Item) {
			if it.Kind == "fact" {
				c.add(it.Lines["fact"], SeverityWarning, checkFacts,
					fmt.Sprintf("the fact item at line %d is not checked because no facts directory is set", it.Lines["fact"]),
					"set facts.dir to check that the fact exists and is citable")
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
	c.each(func(it *Item) {
		if it.Kind != "fact" {
			return
		}
		f, ok := resolve(it.Lines["fact"], it.Fact)
		switch {
		case !ok || f.Citable():
		case f.Problem != "":
			c.add(it.Lines["fact"], SeverityError, checkFacts,
				fmt.Sprintf("fact %q cannot be cited: %s", it.Fact, f.Problem), "correct the bullet's status tag")
		default:
			c.add(it.Lines["fact"], SeverityError, checkFacts,
				fmt.Sprintf("fact %q is tagged [%s], which a page cannot cite", it.Fact, f.Tag),
				"place a verified, external, or vendor fact, or retag the bullet once it is checked")
		}
	})
	for i := range c.p.Dispositions {
		resolve(c.p.Dispositions[i].Lines["fact"], c.p.Dispositions[i].Fact)
	}
}

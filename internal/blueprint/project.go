package blueprint

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/glw907/drafter/internal/plan"
)

// Project says where a documentation project keeps its blueprint tree. Root is the project root
// on disk. BlueprintRoot and Outline are slash paths relative to it, FactsDir is an absolute
// directory or empty when the project has no facts store, and Required makes a managed page
// with no blueprint an error instead of a warning. BriefPath is the brief path template that
// [BriefPath] takes.
type Project struct {
	Root          string
	BlueprintRoot string
	Outline       string
	FactsDir      string
	Required      bool
	BriefPath     string
}

// loaded is what a command reads before it looks at any blueprint: the outline, the lock, and the
// facts store, each already confirmed to be usable.
type loaded struct {
	outline  *Outline
	pages    map[string]Page
	plan     *plan.Outline
	lock     *Lock
	lockRel  string
	facts    *plan.Facts
	files    []string // repo-relative blueprint paths in walk order
	strays   []string // blueprint-like files that no page path maps to
	outRel   string
	findings []plan.Finding // outline decode warnings
}

func (p Project) abs(rel string) string { return filepath.Join(p.Root, filepath.FromSlash(rel)) }

// load reads the shared inputs. Every error it returns means the command could not run: the
// blueprint root or the outline is missing or malformed, the lock does not parse, or the facts
// directory is unusable. An absent lock is valid.
func (p Project) load() (*loaded, error) {
	info, err := os.Stat(p.abs(p.BlueprintRoot))
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("no blueprint root at %s; create it, or set blueprints.root", p.BlueprintRoot)
	}
	data, err := os.ReadFile(p.abs(p.Outline))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no outline at %s; list the doc set's pages there, or set blueprints.outline", p.Outline)
	}
	if err != nil {
		return nil, fmt.Errorf("read outline: %w", err)
	}
	l, err := p.loadOutline(data)
	if err != nil {
		return nil, err
	}
	return l, p.discover(l)
}

// loadOutline decodes the outline and reads the lock and the facts store. It does not walk the
// blueprint root.
func (p Project) loadOutline(data []byte) (*loaded, error) {
	var err error
	o, outFindings := DecodeOutline(p.Outline, data)
	if o == nil || plan.HasError(outFindings) {
		first := outFindings[0]
		for _, f := range outFindings {
			if f.Severity == plan.SeverityError {
				first = f
				break
			}
		}
		return nil, fmt.Errorf("outline %s is malformed: %s:%d: %s", p.Outline, first.File, first.Line, first.Message)
	}
	l := &loaded{outline: o, pages: map[string]Page{}, outRel: p.Outline, findings: outFindings}
	var paths []string
	slugs := map[string]string{}
	for _, pg := range o.Pages {
		pg.Path = path.Clean(pg.Path)
		l.pages[pg.Path] = pg
		paths = append(paths, pg.Path)
		if pg.Slug != "" {
			slugs[pg.Slug] = pg.Path
		}
	}
	var links []plan.CrossLink
	for _, c := range o.CrossLinks {
		links = append(links, plan.CrossLink{From: c.From, To: c.To, Why: c.Why})
	}
	l.plan = plan.NewOutline(paths, slugs, links)

	l.lockRel = path.Join(path.Dir(p.Outline), "drafted.lock")
	if l.lock, err = ReadLock(p.abs(l.lockRel)); err != nil {
		return nil, err
	}
	if p.FactsDir != "" {
		if err := plan.CheckFactsDir(p.FactsDir); err != nil {
			return nil, err
		}
		if l.facts, err = plan.LoadFacts(p.FactsDir); err != nil {
			return nil, err
		}
	}
	return l, nil
}

// discover lists the blueprint files under the blueprint root. A file that ends in
// .blueprint.yaml but maps to no page path, such as one named only that, goes to strays, as does
// a .blueprint.yml file, since the suffix is the path rule's and the extension is not negotiable.
func (p Project) discover(l *loaded) error {
	err := filepath.WalkDir(p.abs(p.BlueprintRoot), func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(p.Root, file)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case strings.HasSuffix(rel, blueprintSuffix):
			if _, ok := PagePath(p.BlueprintRoot, rel); ok {
				l.files = append(l.files, rel)
			} else {
				l.strays = append(l.strays, rel)
			}
		case strings.HasSuffix(rel, ".blueprint.yml"):
			l.strays = append(l.strays, rel)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("read blueprint root: %w", err)
	}
	return nil
}

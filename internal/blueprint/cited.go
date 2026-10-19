package blueprint

import (
	"fmt"
	"os"
	"path"
	"slices"
)

// Cited returns the sorted fact ids the blueprint at rel, a repo-relative slash path, places or
// cites: the introduction's facts, each fact item, each item's facts list, and the facts the
// framing parts cite in backticks, as the blueprint's view counts them. A disposition names a fact
// the page leaves out, so it does not count. The result is empty, with no error, when the
// blueprint does not decode or its page is not a managed one the outline lists, since Check
// reports those. The error means the blueprint, the outline, the lock, or the facts directory
// could not be read.
func (p Project) Cited(rel string) ([]string, error) {
	data, err := os.ReadFile(p.abs(rel))
	if err != nil {
		return nil, fmt.Errorf("read blueprint: %w", err)
	}
	outline, err := os.ReadFile(p.abs(p.Outline))
	if err != nil {
		return nil, fmt.Errorf("read outline: %w", err)
	}
	return p.CitedIn(rel, data, outline)
}

// CitedIn is Cited over the given blueprint and outline bytes, so a caller can read both as
// committed at a revision. The lock and the facts store still come from the working tree.
func (p Project) CitedIn(rel string, data, outline []byte) ([]string, error) {
	l, err := p.loadOutline(outline)
	if err != nil {
		return nil, err
	}
	b, _ := Decode(rel, data)
	pagePath, _ := PagePath(p.BlueprintRoot, rel)
	entry, listed := l.pages[path.Clean(pagePath)]
	if b == nil || !listed || !entry.Managed {
		return nil, nil
	}
	v := View(b, entry, l.facts)
	ids := slices.Clone(v.Introduction.Facts)
	for _, s := range v.Sections {
		for _, it := range s.Items {
			if it.Fact != "" {
				ids = append(ids, it.Fact)
			}
			ids = append(ids, it.Facts...)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

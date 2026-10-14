package plan

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
)

// Outline is a docs outline file, a JSON object whose pages array lists the pages a project
// plans. drafter reads each page's repo-relative path, so a link to a page that is planned but not
// written yet reads as a warning, not an error. It also reads each page's optional slug and the
// optional top-level crossLinks list, which the cross-link check uses.
type Outline struct {
	paths      map[string]bool
	slugs      map[string]string // slug to repo-relative path
	crossLinks []crossLink
}

// crossLink is one crossLinks entry. From and To each name a page by slug or by repo-relative path.
type crossLink struct {
	From, To, Why string
}

// LoadOutline reads the outline at file. It fails when the file is missing, is not JSON, or has
// no pages array.
func LoadOutline(file string) (*Outline, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read outline: %w", err)
	}
	var doc struct {
		Pages *[]struct {
			Path string `json:"path"`
			Slug string `json:"slug"`
		} `json:"pages"`
		CrossLinks []struct {
			From string `json:"from"`
			To   string `json:"to"`
			Why  string `json:"why"`
		} `json:"crossLinks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("outline %s: %w", file, err)
	}
	if doc.Pages == nil {
		return nil, fmt.Errorf("outline %s has no pages array", file)
	}
	o := &Outline{paths: map[string]bool{}, slugs: map[string]string{}}
	for _, p := range *doc.Pages {
		if p.Path == "" {
			continue
		}
		clean := path.Clean(filepath.ToSlash(p.Path))
		o.paths[clean] = true
		if p.Slug != "" {
			o.slugs[p.Slug] = clean
		}
	}
	for _, c := range doc.CrossLinks {
		o.crossLinks = append(o.crossLinks, crossLink(c))
	}
	return o, nil
}

// LoadOutlines reads each outline file and returns one outline that lists every page any of them
// lists, resolves a slug any of them gives, and carries every cross-link. A load error names the
// file that failed.
func LoadOutlines(files []string) (*Outline, error) {
	merged := &Outline{paths: map[string]bool{}, slugs: map[string]string{}}
	for _, file := range files {
		o, err := LoadOutline(file)
		if err != nil {
			return nil, err
		}
		maps.Copy(merged.paths, o.paths)
		maps.Copy(merged.slugs, o.slugs)
		merged.crossLinks = append(merged.crossLinks, o.crossLinks...)
	}
	return merged, nil
}

// lists reports whether the outline lists the repo-relative slash path p.
func (o *Outline) lists(p string) bool {
	return o != nil && o.paths[p]
}

// resolve returns the repo-relative path of the page that ref names, either a path the outline
// lists or a slug, and reports whether there is one.
func (o *Outline) resolve(ref string) (string, bool) {
	if o == nil {
		return "", false
	}
	if p := path.Clean(filepath.ToSlash(ref)); o.paths[p] {
		return p, true
	}
	p, ok := o.slugs[ref]
	return p, ok
}

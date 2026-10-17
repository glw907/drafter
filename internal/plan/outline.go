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

// CrossLink is one cross-link of an outline built by NewOutline: the page From should link to
// the page To, each named by slug or by repo-relative path, for the reason Why.
type CrossLink struct {
	From, To, Why string
}

// NewOutline returns an outline that lists the repo-relative slash paths in paths, resolves each
// key of slugs to the path it maps to, and carries crossLinks. It serves outlines that are not
// JSON, such as a blueprint project's outline.yaml, which LoadOutline cannot read.
func NewOutline(paths []string, slugs map[string]string, crossLinks []CrossLink) *Outline {
	o := &Outline{paths: map[string]bool{}, slugs: map[string]string{}}
	for _, p := range paths {
		o.paths[path.Clean(filepath.ToSlash(p))] = true
	}
	for slug, p := range slugs {
		o.slugs[slug] = path.Clean(filepath.ToSlash(p))
	}
	for _, c := range crossLinks {
		o.crossLinks = append(o.crossLinks, crossLink(c))
	}
	return o
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

// OutlineWithPages returns an outline that lists every page the outline files list and every
// repo-relative path in pages. With no files it lists pages alone. A pipeline run passes the
// pages it is about to write, so a link to one of them is not a link to a missing page.
func OutlineWithPages(files, pages []string) (*Outline, error) {
	o := &Outline{paths: map[string]bool{}, slugs: map[string]string{}}
	if len(files) > 0 {
		var err error
		if o, err = LoadOutlines(files); err != nil {
			return nil, err
		}
	}
	for _, p := range pages {
		o.paths[path.Clean(filepath.ToSlash(p))] = true
	}
	return o, nil
}

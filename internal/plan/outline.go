package plan

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

// Outline is a docs outline file, a JSON object whose pages array lists the pages a project
// plans. drafter reads only each page's repo-relative path, so a link to a page that is planned
// but not written yet reads as a warning, not an error.
type Outline struct {
	paths map[string]bool
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
		} `json:"pages"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("outline %s: %w", filepath.Base(file), err)
	}
	if doc.Pages == nil {
		return nil, errors.New("outline has no pages array")
	}
	o := &Outline{paths: map[string]bool{}}
	for _, p := range *doc.Pages {
		if p.Path != "" {
			o.paths[path.Clean(filepath.ToSlash(p.Path))] = true
		}
	}
	return o, nil
}

// lists reports whether the outline lists the repo-relative slash path p.
func (o *Outline) lists(p string) bool {
	return o != nil && o.paths[p]
}

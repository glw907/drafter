package assemble

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

// ConsistencyPrompt builds the consistency read's prompt for p: the facts the plan places, each
// with its text, and the repo pages it links, each with its headings. The placed facts are the
// plan's fact items and every facts list an item or the introduction carries; facts may be nil,
// which lists ids alone. A link to a URL or to the page itself is skipped, and a linked file that
// does not exist is listed as missing. root is the project root.
func ConsistencyPrompt(p *plan.Plan, facts *plan.Facts, root string) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Check a documentation page plan for conflicts. The page is %s (%s).\n", p.Page.Path, p.Page.Title)
	b.WriteString("Find each place where two of the placed facts below conflict, or where a placed fact conflicts with a page the plan links. ")
	b.WriteString("Report a conflict only when both sides cannot be true together; a difference of scope is not one.\n\nPlaced facts:\n")
	for _, id := range placedFacts(p) {
		fmt.Fprintf(&b, "\n- %s", id)
		if facts == nil {
			continue
		}
		if f, ok := facts.Get(id); ok {
			fmt.Fprintf(&b, ": %s", strings.Join(strings.Fields(f.Text), " "))
		} else {
			b.WriteString(": (not held by exactly one bullet of the facts store)")
		}
	}
	b.WriteString("\n\nLinked pages:\n")

	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("assemble: resolve project root: %w", err)
	}
	dir := path.Dir(p.Page.Path)
	seen := map[string]bool{}
	for _, s := range p.Sections {
		for _, it := range s.Items {
			if it.Kind != "link" || strings.Contains(it.Target, "://") || strings.HasPrefix(it.Target, "#") {
				continue
			}
			file, _, _ := strings.Cut(it.Target, "#")
			rel := path.Join(dir, file)
			if seen[rel] {
				continue
			}
			seen[rel] = true
			heads, err := headings(realRoot, rel)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				fmt.Fprintf(&b, "\n- missing: %s\n", rel)
			case err != nil:
				return "", err
			default:
				fmt.Fprintf(&b, "\n- %s\n", rel)
				for _, h := range heads {
					fmt.Fprintf(&b, "  %s\n", h)
				}
			}
		}
	}
	return b.String(), nil
}

// placedFacts returns the ids of the plan's fact items and every facts list, in order of first
// appearance.
func placedFacts(p *plan.Plan) []string {
	var ids []string
	seen := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, id := range p.Introduction.Facts {
		add(id)
	}
	for _, s := range p.Sections {
		for _, it := range s.Items {
			add(it.Fact)
			for _, id := range it.Facts {
				add(id)
			}
		}
	}
	return ids
}

// headings returns the Markdown heading lines of the repo-relative file rel, outside code fences.
// A path that leaves the root reads as missing.
func headings(realRoot, rel string) ([]string, error) {
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return nil, fs.ErrNotExist
	}
	real, err := filepath.EvalSymlinks(filepath.Join(realRoot, filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	if !within(real, realRoot) {
		return nil, fs.ErrNotExist
	}
	data, err := os.ReadFile(real)
	if err != nil {
		return nil, fmt.Errorf("assemble: read linked page %s: %w", rel, err)
	}
	var out []string
	fenced := false
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~"):
			fenced = !fenced
		case !fenced && strings.HasPrefix(line, "#"):
			out = append(out, strings.TrimRight(line, " \t\r"))
		}
	}
	return out, nil
}

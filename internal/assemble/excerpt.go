package assemble

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/plan"
)

// Lines around a pointer an excerpt takes.
const (
	pointBefore, pointAfter = 10, 10 // path:N
	spanBefore, spanAfter   = 5, 5   // path:A-B
	declBefore, declAfter   = 5, 40  // path#name
)

var proseExt = map[string]bool{".md": true, ".mdx": true, ".markdown": true, ".txt": true, ".rst": true, ".adoc": true}

var tsExt = map[string]bool{".ts": true, ".tsx": true, ".mts": true, ".cts": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true, ".svelte": true}

// Excerpter renders fact excerpts against one project's tracked files. Its blocks are the ones a
// request carries, so a command that shows a fact's excerpts and a read that sends them agree.
type Excerpter struct {
	r *repo
}

// NewExcerpter lists the project's tracked files once, for every excerpt the Excerpter renders.
func NewExcerpter(ctx context.Context, c *config.Config) (*Excerpter, error) {
	r, err := openRepo(ctx, c)
	if err != nil {
		return nil, err
	}
	return &Excerpter{r: r}, nil
}

// Fact returns the excerpt blocks of the fact with the given id, one per source pointer or line
// range, and reports whether any of them shows source lines rather than a "no excerpt:" reason.
// A fact with no source pointer has no blocks and reports false. An id the store does not hold
// once is an error.
func (e *Excerpter) Fact(facts *plan.Facts, id string) ([]string, bool, error) {
	return e.r.factBlocks(facts, id)
}

// excerptBlocks returns the source excerpts of the facts with the given ids: one block per
// source pointer or line range. An id the store does not hold once is an error.
func (r *repo) excerptBlocks(facts *plan.Facts, ids []string) ([]string, error) {
	var blocks []string
	for _, id := range ids {
		b, _, err := r.factBlocks(facts, id)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, b...)
	}
	return blocks, nil
}

func (r *repo) factBlocks(facts *plan.Facts, id string) ([]string, bool, error) {
	f, ok := facts.Get(id)
	if !ok {
		return nil, false, fmt.Errorf("assemble: fact %q is not held by exactly one bullet", id)
	}
	var blocks []string
	real := false
	for _, s := range f.Sources {
		for _, b := range r.excerpt(id, s) {
			_, body, _ := strings.Cut(b, "\n")
			real = real || !strings.HasPrefix(body, "no excerpt:")
			blocks = append(blocks, b)
		}
	}
	return blocks, real, nil
}

// excerpt returns the blocks for one source pointer of fact id.
func (r *repo) excerpt(id, source string) []string {
	file, suffix := source, ""
	if i := strings.IndexAny(source, ":#"); i >= 0 {
		file, suffix = source[:i], source[i:]
	}
	label := source + " (fact " + id + ")"
	none := func(form string) []string { return []string{block("excerpt", label, "no excerpt: "+form)} }

	rel := filepath.ToSlash(filepath.Clean(file))
	abs := filepath.Join(r.root, rel)
	real, err := filepath.EvalSymlinks(abs)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return none("missing")
	case err != nil || !within(real, r.real):
		return none("untracked")
	}
	info, err := os.Stat(real)
	switch {
	case err != nil:
		return none("missing")
	case info.IsDir():
		return none("directory")
	case !r.tracked[rel]:
		return none("untracked")
	case proseExt[strings.ToLower(filepath.Ext(rel))]:
		return none("prose")
	}

	if suffix == "" {
		b, ok := fileBlock(target{rel, real})
		if !ok {
			return none("not a regular file")
		}
		return []string{b}
	}
	data, err := os.ReadFile(real)
	if err != nil {
		return none("missing")
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")

	if suffix[0] == '#' {
		name := suffix[1:]
		name = name[strings.LastIndex(name, ".")+1:]
		at := declaration(lines, filepath.Ext(rel), name)
		if at < 0 {
			return none("declaration not found")
		}
		return []string{window(label, lines, at+1-declBefore, at+1+declAfter)}
	}
	var out []string
	for rng := range strings.SplitSeq(suffix[1:], ",") {
		a, b, isSpan := strings.Cut(rng, "-")
		lo, _ := strconv.Atoi(a)
		hi := lo
		before, after := pointBefore, pointAfter
		if isSpan {
			hi, _ = strconv.Atoi(b)
			before, after = spanBefore, spanAfter
		}
		out = append(out, window(label, lines, lo-before, hi+after))
	}
	return out
}

// window returns the block for lines lo through hi, 1-based and clamped to the file.
func window(label string, lines []string, lo, hi int) string {
	lo, hi = max(lo, 1), min(hi, len(lines))
	if lo > hi {
		return block("excerpt", label, "no excerpt: range beyond the end of the file")
	}
	return block("excerpt", fmt.Sprintf("%s lines %d-%d", label, lo, hi), strings.Join(lines[lo-1:hi], "\n"))
}

// goGroupOpen matches the line that opens a grouped Go const, var, or type block.
var goGroupOpen = regexp.MustCompile(`^(const|var|type)\s*\(\s*$`)

// declaration returns the 0-based index of the first line declaring name in a file of extension
// ext, or -1. Only a declaration counts, never a mere mention; in Go that includes a name at the
// top level of a grouped const, var, or type block.
func declaration(lines []string, ext, name string) int {
	var res []*regexp.Regexp
	q := regexp.QuoteMeta(name)
	switch {
	case ext == ".go":
		res = []*regexp.Regexp{
			regexp.MustCompile(`^func\s+(\([^)]*\)\s*)?` + q + `\b`),
			regexp.MustCompile(`^type\s+` + q + `\b`),
			regexp.MustCompile(`^(var|const)\s+` + q + `\b`),
		}
	case tsExt[strings.ToLower(ext)]:
		res = []*regexp.Regexp{regexp.MustCompile(
			`^(export\s+(default\s+)?)?(declare\s+)?(abstract\s+)?(async\s+)?(function\s*\*?|class|interface|type|enum|const|let)\s+` + q + `\b`)}
	}
	var member *regexp.Regexp
	if ext == ".go" {
		member = regexp.MustCompile(`^\t` + q + `(\s|=|$)`)
	}
	inGroup := false
	for i, line := range lines {
		for _, re := range res {
			if re.MatchString(line) {
				return i
			}
		}
		switch {
		case goGroupOpen.MatchString(line):
			inGroup = member != nil
		case line == ")":
			inGroup = false
		case inGroup && member.MatchString(line):
			return i
		}
	}
	return -1
}

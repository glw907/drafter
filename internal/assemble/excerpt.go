package assemble

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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

// excerptBlocks returns the source excerpts of the facts with the given ids: one block per
// source pointer or line range. An id the store does not hold once is an error.
func (r *repo) excerptBlocks(facts *plan.Facts, ids []string) ([]string, error) {
	var blocks []string
	for _, id := range ids {
		f, ok := facts.Get(id)
		if !ok {
			return nil, fmt.Errorf("assemble: fact %q is not held by exactly one bullet", id)
		}
		for _, s := range f.Sources {
			blocks = append(blocks, r.excerpt(id, s)...)
		}
	}
	return blocks, nil
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
		b, _ := fileBlock(target{rel, real})
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

// declaration returns the 0-based index of the first line declaring name in a file of extension
// ext, or -1. Only a declaration counts, never a mere mention.
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
	for i, line := range lines {
		for _, re := range res {
			if re.MatchString(line) {
				return i
			}
		}
	}
	return -1
}

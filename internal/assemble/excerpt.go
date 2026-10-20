package assemble

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/plan"
)

// Lines around a pointer an excerpt takes, and the sizes that change the view.
const (
	pointBefore, pointAfter = 10, 10 // path:N
	spanBefore, spanAfter   = 5, 5   // path:A-B

	// wholeFileLines is the longest tracked code file a pointer shows whole.
	wholeFileLines = 150
	// declMaxLines caps a path#name excerpt, which otherwise spans the whole declaration.
	declMaxLines = 200
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

// Pointer returns the blocks for one source pointer outside any fact, such as "src/a.ts#Panel.open",
// and reports whether any shows source lines. It takes the same path through the excerpt code as
// a fact's pointer, so it shows what a read sends for a fact that cites it.
func (e *Excerpter) Pointer(source string) ([]string, bool) {
	blocks := e.r.excerpt("", source, map[string]bool{})
	return blocks, slices.ContainsFunc(blocks, func(b string) bool { return !noExcerpt(b) })
}

func noExcerpt(b string) bool {
	_, body, _ := strings.Cut(b, "\n")
	return strings.HasPrefix(body, "no excerpt:")
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
	whole := map[string]bool{}
	for _, s := range f.Sources {
		blocks = append(blocks, r.excerpt(id, s, whole)...)
	}
	return blocks, slices.ContainsFunc(blocks, func(b string) bool { return !noExcerpt(b) }), nil
}

// excerpt returns the blocks for one source pointer of fact id; id is empty for a pointer
// outside any fact. whole records the small files an earlier pointer of the same fact showed
// whole, so each shows once per fact.
func (r *repo) excerpt(id, source string, whole map[string]bool) []string {
	file, suffix := source, ""
	if i := strings.IndexAny(source, ":#"); i >= 0 {
		file, suffix = source[:i], source[i:]
	}
	factTag := ""
	if id != "" {
		factTag = " (fact " + id + ")"
	}
	label := source + factTag
	none := func(form string) []string { return []string{block("excerpt", label, "no excerpt: "+form)} }

	rel := filepath.ToSlash(filepath.Clean(file))
	prose := proseExt[strings.ToLower(filepath.Ext(rel))]
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
	case prose && (!r.prose[rel] || !strings.HasPrefix(suffix, ":")):
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

	var out []string
	if suffix[0] == '#' {
		lo, hi := anchor(lines, filepath.Ext(rel), suffix[1:])
		if lo < 0 {
			return none("declaration not found")
		}
		out = []string{declBlock(label, lines, lo, hi)}
	} else {
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
	}
	if prose || len(lines) > wholeFileLines || slices.ContainsFunc(out, noExcerpt) {
		return out
	}
	// A pointer that resolves into a small code file shows the file, so a claim is read with the
	// code around the lines it cites. A pointer that does not resolve keeps its reason.
	if whole[rel] {
		return nil
	}
	whole[rel] = true
	return []string{window(rel+factTag, lines, 1, len(lines))}
}

// declBlock returns the block for the 0-based lines lo through hi, cut to declMaxLines with a
// closing line counting the rest.
func declBlock(label string, lines []string, lo, hi int) string {
	n := hi - lo + 1
	if n <= declMaxLines {
		return window(label, lines, lo+1, hi+1)
	}
	body := strings.Join(lines[lo:lo+declMaxLines], "\n") + fmt.Sprintf("\n… (%d more lines)", n-declMaxLines)
	return block("excerpt", fmt.Sprintf("%s lines %d-%d", label, lo+1, lo+declMaxLines), body)
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

// anchor resolves a "#name" anchor, which may be dotted, to the 0-based first and last line of
// the innermost declaration, or -1 when a link of the chain is not found. The first line is the
// declaration's leading doc comment when it has one.
func anchor(lines []string, ext, name string) (lo, hi int) {
	chain := strings.Split(name, ".")
	at := declaration(lines, ext, chain[0])
	if at < 0 {
		return -1, -1
	}
	first, last := at, declEnd(lines, at)
	for _, inner := range chain[1:] {
		next := innerDeclaration(lines, ext, chain[0], inner, first, last)
		if next < 0 {
			return -1, -1
		}
		first, last = next, declEnd(lines, next)
	}
	return docStart(lines, first), last
}

var (
	indentRe = regexp.MustCompile(`^[ \t]*`)
	closerRe = regexp.MustCompile(`^[ \t]*[})\]]`)
	opensRe  = regexp.MustCompile(`[{(\[]\s*$|=>\s*$`)
)

func indent(line string) int { return len(indentRe.FindString(line)) }

// declEnd returns the 0-based last line of the declaration starting at line at: the line closing
// it at its own indentation, or else the last line before the next one at that indentation or
// less. A closing line that opens again, such as ") {" or "} else {", continues the declaration.
func declEnd(lines []string, at int) int {
	base := indent(lines[at])
	end := at
	for i := at + 1; i < len(lines); i++ {
		l := lines[i]
		if strings.TrimSpace(l) == "" {
			continue
		}
		ind := indent(l)
		switch {
		case ind > base:
			end = i
		case ind == base && closerRe.MatchString(l) && opensRe.MatchString(l):
			end = i
		case ind == base && closerRe.MatchString(l):
			return i
		default:
			return end
		}
	}
	return end
}

// docStart returns the first line of the comment and decorator lines directly above line at.
func docStart(lines []string, at int) int {
	for at > 0 {
		t := strings.TrimSpace(lines[at-1])
		if !strings.HasPrefix(t, "//") && !strings.HasPrefix(t, "/*") && !strings.HasPrefix(t, "*") && !strings.HasPrefix(t, "@") {
			break
		}
		at--
	}
	return at
}

// innerDeclaration returns the 0-based line, within lines first through last, that declares
// inner: a member, property, or method, or a const, let, var, or function declared inside. The
// match at the shallowest indentation wins, so a call of the same name deeper in a body does not.
// In Go it also finds a method of the receiver type outer, which sits at the top level.
func innerDeclaration(lines []string, ext, outer, inner string, first, last int) int {
	q := regexp.QuoteMeta(inner)
	var res []*regexp.Regexp
	switch {
	case ext == ".go":
		method := regexp.MustCompile(`^func\s+\(\s*\w*\s*\*?` + regexp.QuoteMeta(outer) + `(?:\[[^\]]*\])?\s*\)\s*` + q + `\b`)
		for i, l := range lines {
			if method.MatchString(l) {
				return i
			}
		}
		res = []*regexp.Regexp{regexp.MustCompile(`^\s+` + q + `\b`)}
	case tsExt[strings.ToLower(ext)]:
		res = []*regexp.Regexp{
			regexp.MustCompile(`^\s*(?:(?:export|public|private|protected|static|readonly|abstract|override|async|get|set|declare)\s+)*\*?\s*["']?` + q + `["']?\s*[(:=<?!]`),
			regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?(?:function\s*\*?|const|let|var|class)\s+` + q + `\b`),
		}
	}
	best, bestIndent := -1, 0
	for i := first + 1; i <= last && i < len(lines); i++ {
		for _, re := range res {
			if re.MatchString(lines[i]) && (best < 0 || indent(lines[i]) < bestIndent) {
				best, bestIndent = i, indent(lines[i])
			}
		}
	}
	return best
}

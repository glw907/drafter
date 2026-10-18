package plan

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// The facts reader follows cairn's scripts/checks/check-facts.mjs and its facts README: a facts
// directory holds Markdown files of bullets, each leading with its id as a code span and ending
// with one status tag. The reader takes only what the plan checks need, an id and a tag per
// bullet, and leaves the source pointers and owner-tier rules to the project's own gate.

// factsReadme is the one file in a facts directory that is not a facts file.
const factsReadme = "README.md"

var (
	factIDRe   = regexp.MustCompile("^`([^`\\s]+)`")
	maskSpanRe = regexp.MustCompile("`[^`]*`|\"[^\"]*\"")
	tagRe      = regexp.MustCompile(`\[(verified|docs-drift|external|vendor|candidate|rejected)\b[^\]]*\]`)
	anyTagRe   = regexp.MustCompile(`\[([a-zA-Z-]+)(:[^\]]*)?\]`)
)

// skippedSections names the sections whose bullets are not facts.
var skippedSections = []string{"harvest record", "provenance"}

// citableTags lists the status tags a page may cite.
var citableTags = []string{"verified", "external", "vendor"}

// Fact is one bullet of a facts file.
type Fact struct {
	ID   string
	File string // base name within the facts directory
	Line int
	// Tag is the bullet's status tag name, empty when Problem is set.
	Tag string
	// Problem says why the bullet's tag is unusable, empty when Tag is set.
	Problem string
	// Text is the bullet's claim: what follows the id up to its Source: part or its status tag.
	Text string
	// Sources lists the bullet's source pointers as written: "path", "path:100-110,163", or
	// "path#name". It holds the backticked spans of the Source: part that sit outside
	// parentheses and brackets and name a path inside the facts store's repository. A span
	// naming a command, a URL, or commentary, and an [external: ...] part, are left out.
	Sources []string
}

// Citable reports whether a page may cite the fact: its tag is well formed and one of verified,
// external, or vendor.
func (f Fact) Citable() bool {
	return f.Problem == "" && slices.Contains(citableTags, f.Tag)
}

// Facts is the set of fact bullets in a facts directory, keyed by id.
type Facts struct {
	byID map[string][]Fact
}

// Count returns the number of bullets that hold id, which Get hides when it is not exactly one.
func (fs *Facts) Count(id string) int { return len(fs.byID[id]) }

// Cites reports whether a backticked token in prose is a fact citation: the store holds a bullet
// with that id, or the token starts with the prefix, through its first colon, that every id in
// the store shares. The second test lets a mistyped id through to the facts check as a missing
// fact instead of passing as ordinary code.
func (fs *Facts) Cites(token string) bool {
	if fs.Count(token) > 0 {
		return true
	}
	prefix := ""
	for id := range fs.byID {
		i := strings.IndexByte(id, ':')
		if i < 0 || (prefix != "" && id[:i+1] != prefix) {
			return false
		}
		prefix = id[:i+1]
	}
	return prefix != "" && strings.HasPrefix(token, prefix) && len(token) > len(prefix)
}

// Get returns the fact with the given id. It reports false unless exactly one bullet holds the id.
func (fs *Facts) Get(id string) (Fact, bool) {
	got := fs.byID[id]
	if len(got) != 1 {
		return Fact{}, false
	}
	return got[0], true
}

// factsFiles lists the names of the facts files in dir: every Markdown file but README.md.
func factsFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && e.Name() != factsReadme {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}

// CheckFactsDir reports why dir cannot serve as a facts directory: it is missing, is not a
// directory, or holds no facts file.
func CheckFactsDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("facts directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("facts directory %s is not a directory", dir)
	}
	names, err := factsFiles(dir)
	if err != nil {
		return fmt.Errorf("facts directory: %w", err)
	}
	if len(names) == 0 {
		return fmt.Errorf("facts directory %s holds no facts file (a .md file other than %s)", dir, factsReadme)
	}
	return nil
}

// LoadFacts reads every facts file in dir. A bullet under a Harvest record or Provenance heading
// is not a fact, and a bullet without a leading id code span carries nothing a plan can cite.
func LoadFacts(dir string) (*Facts, error) {
	names, err := factsFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("read facts directory: %w", err)
	}
	files := map[string]string{}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read facts file: %w", err)
		}
		files[name] = string(data)
	}
	return ParseFacts(files, repoRoot(dir)), nil
}

// ParseFacts reads facts files given as content by base name, as LoadFacts reads a directory.
// Source pointers are judged against root, the repository the facts store lives in.
func ParseFacts(files map[string]string, root string) *Facts {
	fs := &Facts{byID: map[string][]Fact{}}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		for _, b := range extractBullets(files[name]) {
			if slices.Contains(skippedSections, strings.ToLower(strings.TrimSpace(b.section))) {
				continue
			}
			m := factIDRe.FindStringSubmatch(b.text)
			if m == nil {
				continue
			}
			f := Fact{ID: m[1], File: name, Line: b.line}
			f.Tag, f.Problem = checkTag(b.text)
			f.Text, f.Sources = parseBody(b.text[len(m[0]):], root)
			fs.byID[f.ID] = append(fs.byID[f.ID], f)
		}
	}
	return fs
}

type bullet struct {
	section string
	text    string
	line    int
}

// extractBullets splits a facts file into bullets. A bullet is a line starting "- " plus the
// non-blank lines after it, up to the next bullet, blank line, or "## " heading, joined by single
// spaces; the section is the last "## " heading before it.
func extractBullets(markdown string) []bullet {
	var out []bullet
	var cur *bullet
	section := ""
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for i, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSuffix(line, "\r")
		switch {
		case strings.HasPrefix(line, "## "):
			flush()
			section = strings.TrimSpace(line[3:])
		case strings.HasPrefix(line, "- "):
			flush()
			cur = &bullet{section: section, text: line[2:], line: i + 1}
		case strings.TrimSpace(line) == "":
			flush()
		case cur != nil:
			cur.text += " " + strings.TrimSpace(line)
		}
	}
	flush()
	return out
}

// checkTag returns the bullet's status tag name, or the reason the bullet has no valid one. It
// masks code spans and quoted spans first, since a bullet routinely carries brackets there that
// are not tags, and then wants exactly one vocabulary tag, last on the bullet, in colon form.
func checkTag(text string) (tag, problem string) {
	masked := maskSpanRe.ReplaceAllStringFunc(text, func(m string) string { return strings.Repeat("x", len(m)) })
	matches := tagRe.FindAllStringSubmatchIndex(masked, -1)
	if len(matches) == 0 {
		loose := anyTagRe.FindAllStringSubmatchIndex(masked, -1)
		if len(loose) > 0 {
			last := loose[len(loose)-1]
			if last[1] >= len(strings.TrimRight(masked, " \t")) {
				return "", fmt.Sprintf("tag outside the vocabulary: got %q", masked[last[0]:last[1]])
			}
		}
		return "", "it has no status tag from the vocabulary"
	}
	if len(matches) > 1 {
		return "", fmt.Sprintf("it has %d status tags, want exactly one", len(matches))
	}
	m := matches[0]
	if strings.TrimSpace(masked[m[1]:]) != "" {
		return "", "the status tag is not the last thing on the bullet"
	}
	name := masked[m[2]:m[3]]
	qualifier := masked[m[3] : m[1]-1]
	if qualifier != "" && !strings.HasPrefix(qualifier, ":") {
		return "", fmt.Sprintf("tag [%s...] uses a space qualifier; the colon form is [%s: ...]", name, name)
	}
	if strings.Contains(masked[:m[0]], "[") {
		return "", "a second bracket appears before the trailing status tag"
	}
	return name, ""
}

// repoRoot returns the nearest ancestor of dir, dir included, that holds a .git entry, or dir
// itself when none does. Source pointers in a facts file are relative to it.
func repoRoot(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	for d := abs; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return abs
		}
	}
}

var (
	sourceMarkerRe = regexp.MustCompile(`(^|\s)Source:`)
	lineListRe     = regexp.MustCompile(`^\d+(-\d+)?(,\d+(-\d+)?)*$`)
	pathCharsRe    = regexp.MustCompile(`^[\w@.+/()\[\]-]+$`)
)

// parseBody splits a bullet's text after its id into the claim and the source pointers. The claim
// ends at the Source: marker, or at the status tag when the bullet has no marker.
func parseBody(rest, root string) (text string, sources []string) {
	masked := maskSpanRe.ReplaceAllStringFunc(rest, func(m string) string { return strings.Repeat("x", len(m)) })
	if loc := tagRe.FindAllStringIndex(masked, -1); len(loc) > 0 {
		cut := loc[len(loc)-1][0]
		rest, masked = rest[:cut], masked[:cut]
	}
	loc := sourceMarkerRe.FindStringIndex(masked)
	if loc == nil {
		return strings.TrimSpace(rest), nil
	}
	text = strings.TrimSpace(rest[:loc[0]])
	part := rest[loc[1]:]
	depth := 0
	for i := 0; i < len(part); i++ {
		switch part[i] {
		case '(', '[':
			depth++
		case ')', ']':
			depth = max(depth-1, 0)
		case '`':
			end := strings.IndexByte(part[i+1:], '`')
			if end < 0 {
				return text, sources
			}
			span := part[i+1 : i+1+end]
			i += end + 1
			if depth == 0 && isSource(span, root) {
				sources = append(sources, span)
			}
		}
	}
	return text, sources
}

// isSource reports whether a backticked span points at a file or directory of the repository. A
// span with a slash counts as a path whether or not the file exists, so a stale pointer reaches
// the reader as a missing source; a bare name counts only when it exists.
func isSource(span, root string) bool {
	if strings.Contains(span, "://") {
		return false
	}
	file, suffix := span, ""
	if i := strings.IndexAny(span, ":#"); i >= 0 {
		file, suffix = span[:i], span[i:]
	}
	if file == "" || !pathCharsRe.MatchString(file) || filepath.IsAbs(file) {
		return false
	}
	if clean := filepath.Clean(file); clean == ".." || strings.HasPrefix(clean, "../") {
		return false
	}
	switch {
	case suffix == "":
	case suffix[0] == ':' && lineListRe.MatchString(suffix[1:]):
	case suffix[0] == '#' && len(suffix) > 1:
	default:
		return false
	}
	if strings.Contains(file, "/") {
		return true
	}
	_, err := os.Stat(filepath.Join(root, file))
	return err == nil
}

package plan

import (
	"fmt"
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
	fs := &Facts{byID: map[string][]Fact{}}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read facts file: %w", err)
		}
		for _, b := range extractBullets(string(data)) {
			if slices.Contains(skippedSections, strings.ToLower(strings.TrimSpace(b.section))) {
				continue
			}
			m := factIDRe.FindStringSubmatch(b.text)
			if m == nil {
				continue
			}
			f := Fact{ID: m[1], File: name, Line: b.line}
			f.Tag, f.Problem = checkTag(b.text)
			fs.byID[f.ID] = append(fs.byID[f.ID], f)
		}
	}
	return fs, nil
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

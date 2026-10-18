package plan

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
)

// This file verifies a facts store against its repository. It ports the contract of cairn's
// scripts/checks/check-facts.mjs, so a store that gate passes raises no error here: the anchor
// window, the line count, and the basename fallback are the same. It walks each bullet's Source:
// part itself instead of reading Fact.Sources, because isSource drops a slashless span that does
// not exist at the root, and the basename fallback could then never fire.

// checkStore names the check that verification findings carry.
const checkStore = "store"

// anchorWindow is how many lines on each side of a cited range an anchor word may fall within.
const anchorWindow = 10

var (
	// pointerAnchorRe matches the anchor that follows a pointer: a code span in parentheses.
	pointerAnchorRe = regexp.MustCompile("^\\s*\\(`([^`]*)`\\)")
	anchorWordRe    = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)
	// strictPathRe is the path alphabet verification checks; a pointer outside it is skipped.
	strictPathRe = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)
	identifierRe = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

	goColumn0Re = regexp.MustCompile(`^(func|type|var|const)\b`)
	tsColumn0Re = regexp.MustCompile(tsPrefix + `[A-Za-z_$]|^declare\s+global\b`)
	goGroupRe   = regexp.MustCompile(`^(const|var|type)\s*\(\s*$`)
)

// tsPrefix matches the start of a column-0 TypeScript or JavaScript declaration, up to its name.
const tsPrefix = `^(?:export\s+(?:default\s+)?)?(?:declare\s+)?(?:abstract\s+)?(?:async\s+)?` +
	`(?:function(?:\s*\*\s*|\s+)|(?:class|interface|type|const\s+enum|enum|const|let|var|namespace|module)\s+)`

// symbolLangs maps the extensions a path#name pointer resolves to the declaration rules to use.
var symbolLangs = map[string]string{
	".go": "go", ".ts": "ts", ".tsx": "ts", ".mts": "ts", ".cts": "ts",
	".js": "ts", ".jsx": "ts", ".mjs": "ts", ".cjs": "ts",
}

type pointerKind int

const (
	pointerBare pointerKind = iota
	pointerLines
	pointerSymbol
)

// pointer is one backticked span of a Source: part that names a repository path.
type pointer struct {
	span, path string
	kind       pointerKind
	lo, hi     int  // the lowest and highest cited line, for pointerLines
	reversed   bool // a cited range runs backward
	name       string
	anchor     string
}

// storeBullet is one bullet of a facts file with the place it was read from.
type storeBullet struct {
	file    string // repo-relative slash path of the facts file
	section string
	id      string // the leading id code span, empty when there is none
	text    string // the whole bullet
	rest    string // the bullet after its id span
	line    int
	skipped bool // the bullet sits under a Harvest record or Provenance heading
}

// sourceFile is a cited file read once.
type sourceFile struct {
	lines []string
	err   error
}

type verifier struct {
	root     string
	files    map[string]*sourceFile
	basename map[string][]string
}

// VerifyFacts checks the facts store in dir against the repository at root and returns the
// findings, the number of pointers it checked, and the number it skipped. A nil ids verifies the
// whole store; otherwise only the bullets whose id is listed. The error means the store or the
// repository's file list could not be read; every fault in the store comes back as a finding.
//
// A bullet outside a Harvest record or Provenance section needs a leading id, a Source:, and one
// valid tag, and its id must be unique in the store. Each pointer in its Source: part is
// checked: path:lines for the file, the lines, and an anchor within 10 lines of them; path#name
// for the declaration of a Go or TypeScript name; a bare path for existence, as a warning when
// missing. A pointer with a dotted symbol, a symbol in another kind of file, or a character
// outside [A-Za-z0-9_./-] is skipped and counted.
func VerifyFacts(dir, root string, ids []string) (findings []Finding, checked, skipped int, err error) {
	if dir, err = filepath.Abs(dir); err != nil {
		return nil, 0, 0, err
	}
	if root, err = filepath.Abs(root); err != nil {
		return nil, 0, 0, err
	}
	names, err := factsFiles(dir)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read facts directory: %w", err)
	}
	var bullets []storeBullet
	for _, name := range names {
		full := filepath.Join(dir, name)
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("read facts file: %w", err)
		}
		rel := name
		if r, err := filepath.Rel(root, full); err == nil {
			rel = filepath.ToSlash(r)
		}
		for _, b := range extractBullets(string(data)) {
			sb := storeBullet{file: rel, section: b.section, text: b.text, rest: b.text, line: b.line}
			sb.skipped = slices.Contains(skippedSections, strings.ToLower(strings.TrimSpace(b.section)))
			if m := factIDRe.FindStringSubmatch(b.text); m != nil {
				sb.id, sb.rest = m[1], b.text[len(m[0]):]
			}
			bullets = append(bullets, sb)
		}
	}
	places := map[string][]string{}
	for _, b := range bullets {
		if b.id != "" && !b.skipped {
			places[b.id] = append(places[b.id], fmt.Sprintf("%s:%d", b.file, b.line))
		}
	}
	// factIDRe takes any leading code span, so a Harvest record bullet that opens with a file name
	// would read as carrying an id. Only a span shaped like the store's ids, or equal to one, counts.
	prefix := sharedIDPrefix(places)
	for i, b := range bullets {
		if b.skipped && b.id != "" && len(places[b.id]) == 0 && (prefix == "" || !strings.HasPrefix(b.id, prefix)) {
			bullets[i].id = ""
		}
	}

	v := &verifier{root: root, files: map[string]*sourceFile{}}
	for _, b := range bullets {
		if ids != nil && (b.id == "" || !slices.Contains(ids, b.id)) {
			continue
		}
		add := func(severity, message, fix string) {
			findings = append(findings, Finding{File: b.file, Line: b.line, Severity: severity, Check: checkStore, Message: message, Fix: fix})
		}
		if b.skipped {
			if b.id != "" {
				add(SeverityError, fmt.Sprintf("a %s bullet carries the fact id %q; only a fact bullet takes one", strings.TrimSpace(b.section), b.id),
					"remove the id, or move the bullet out of that section")
			}
			continue
		}
		if b.id == "" {
			add(SeverityError, "the bullet has no leading fact id", "start the bullet with its id as a code span")
		} else if len(places[b.id]) > 1 {
			add(SeverityError, fmt.Sprintf("fact %q is on %d bullets (%s)", b.id, len(places[b.id]), strings.Join(places[b.id], ", ")), "keep one bullet per id")
		}
		if !strings.Contains(b.text, "Source:") {
			add(SeverityError, `the bullet has no "Source:"`, "name where the claim comes from after Source:")
		}
		if _, problem := checkTag(b.text); problem != "" {
			add(SeverityError, "the bullet's status tag is unusable: "+problem, "end the bullet with one tag from the vocabulary, in colon form")
		}
		for _, p := range sourcePointers(b.rest) {
			switch {
			case p.kind == pointerBare && !strings.Contains(p.path, "/"):
				// A bare word is a path only when it names something at the root.
				if _, err := os.Stat(filepath.Join(root, p.path)); err != nil {
					continue
				}
			case p.kind != pointerBare && !strings.Contains(p.path, "/") && !strings.Contains(p.path, "."):
				continue
			}
			if !strictPathRe.MatchString(p.path) {
				skipped++
				continue
			}
			if p.kind == pointerSymbol {
				if _, ok := symbolLangs[strings.ToLower(path.Ext(p.path))]; !ok || !identifierRe.MatchString(p.name) {
					skipped++
					continue
				}
			}
			checked++
			sev, msg, fix, err := v.verify(p)
			if err != nil {
				return nil, 0, 0, err
			}
			if msg != "" {
				add(sev, msg, fix)
			}
		}
	}
	return findings, checked, skipped, nil
}

// sharedIDPrefix returns the prefix, through the first colon, that every id in the store shares,
// or "" when some id has no colon or two prefixes differ.
func sharedIDPrefix(ids map[string][]string) string {
	prefix := ""
	for id := range ids {
		i := strings.IndexByte(id, ':')
		if i < 0 || (prefix != "" && id[:i+1] != prefix) {
			return ""
		}
		prefix = id[:i+1]
	}
	return prefix
}

// sourcePointers returns the pointers of a bullet's Source: part: the backticked spans at
// bracket depth 0, between the Source: marker and the status tag, that have a pointer's shape.
// The anchor is the code span in parentheses right after a pointer.
func sourcePointers(rest string) []pointer {
	masked := maskSpanRe.ReplaceAllStringFunc(rest, func(m string) string { return strings.Repeat("x", len(m)) })
	if loc := tagRe.FindAllStringIndex(masked, -1); len(loc) > 0 {
		cut := loc[len(loc)-1][0]
		rest, masked = rest[:cut], masked[:cut]
	}
	var part string
	if loc := sourceMarkerRe.FindStringIndex(masked); loc != nil {
		part = rest[loc[1]:]
	} else if i := strings.Index(rest, "Source:"); i >= 0 {
		part = rest[i+len("Source:"):]
	} else {
		return nil
	}
	var out []pointer
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
				return out
			}
			span := part[i+1 : i+1+end]
			i += end + 1
			if depth != 0 {
				continue
			}
			p, ok := parsePointer(span)
			if !ok {
				continue
			}
			if m := pointerAnchorRe.FindStringSubmatch(part[i+1:]); m != nil {
				p.anchor = m[1]
				i += len(m[0])
			}
			out = append(out, p)
		}
	}
	return out
}

// parsePointer reads a backticked span as a pointer: <path>, <path>:<lines>, or <path>#<name>.
// It does not look at the file system, so a slashless bare name is a pointer here; the caller
// keeps it only when the name exists.
func parsePointer(span string) (pointer, bool) {
	if strings.Contains(span, "://") {
		return pointer{}, false
	}
	file, suffix := span, ""
	if i := strings.IndexAny(span, ":#"); i >= 0 {
		file, suffix = span[:i], span[i:]
	}
	if file == "" || !pathCharsRe.MatchString(file) || filepath.IsAbs(file) {
		return pointer{}, false
	}
	if clean := filepath.Clean(file); clean == ".." || strings.HasPrefix(clean, "../") {
		return pointer{}, false
	}
	p := pointer{span: span, path: file}
	switch {
	case suffix == "":
	case suffix[0] == ':' && lineListRe.MatchString(suffix[1:]):
		p.kind = pointerLines
		p.lo, p.hi = math.MaxInt, 0
		for part := range strings.SplitSeq(suffix[1:], ",") {
			a, b, isRange := strings.Cut(part, "-")
			lo, hi := atoiSaturated(a), atoiSaturated(a)
			if isRange {
				hi = atoiSaturated(b)
			}
			p.reversed = p.reversed || lo > hi
			p.lo, p.hi = min(p.lo, lo, hi), max(p.hi, lo, hi)
		}
	case suffix[0] == '#' && len(suffix) > 1:
		p.kind, p.name = pointerSymbol, suffix[1:]
	default:
		return pointer{}, false
	}
	return p, true
}

func atoiSaturated(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return math.MaxInt32
	}
	return n
}

// verify checks one pointer. It returns an empty message when the pointer is sound. The error
// means the repository's file list could not be read.
func (v *verifier) verify(p pointer) (severity, message, fix string, err error) {
	abs := filepath.Join(v.root, filepath.FromSlash(p.path))
	info, statErr := os.Stat(abs)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) && !errors.Is(statErr, syscall.ENOTDIR) {
		return SeverityError, fmt.Sprintf("pointer %q cannot be read: %v", p.span, statErr), "fix the file's permissions", nil
	}
	if p.kind == pointerBare {
		if !exists {
			return SeverityWarning, fmt.Sprintf("source %q does not exist", p.span),
				"correct the path; a package name, a path inside a dependency, or a path made by a scaffold can stay as it is", nil
		}
		return "", "", "", nil
	}

	resolved := p.path
	if !exists && p.kind == pointerLines && !strings.Contains(p.path, "/") {
		index, err := v.basenames()
		if err != nil {
			return "", "", "", err
		}
		switch matches := index[p.path]; len(matches) {
		case 0:
		case 1:
			resolved = matches[0]
			abs = filepath.Join(v.root, filepath.FromSlash(resolved))
			info, exists = nil, true
		default:
			return SeverityError, fmt.Sprintf("pointer %q matches %d files by name (%s)", p.span, len(matches), strings.Join(matches, ", ")),
				"give the path from the repository root", nil
		}
	}
	if !exists {
		return SeverityError, fmt.Sprintf("pointer %q: no file at %s", p.span, p.path), "correct the path, or update the pointer to the file's new place", nil
	}
	if info != nil && info.IsDir() {
		return SeverityError, fmt.Sprintf("pointer %q names a directory; a line or symbol pointer names a file", p.span), "cite a file in it", nil
	}
	src := v.read(abs)
	if src.err != nil {
		return SeverityError, fmt.Sprintf("pointer %q cannot be read: %v", p.span, src.err), "fix the file's permissions", nil
	}
	if p.kind == pointerSymbol {
		return v.verifySymbol(p, src.lines)
	}

	n := len(src.lines)
	if p.reversed {
		return SeverityError, fmt.Sprintf("pointer %q has a range that runs backward", p.span), "write the lower line first", nil
	}
	if p.lo < 1 || p.hi > n {
		return SeverityError, fmt.Sprintf("pointer %q is out of range (%s has %d lines)", p.span, resolved, n),
			"update the cited lines to where the code is now", nil
	}
	if p.anchor == "" {
		return "", "", "", nil
	}
	start, end := max(1, p.lo-anchorWindow), min(n, p.hi+anchorWindow)
	if missing := absentWords(p.anchor, strings.Join(src.lines[start-1:end], "\n")); len(missing) > 0 {
		return SeverityError, fmt.Sprintf("the anchor for %q names %s, not found within %d lines of the cited range", p.span, quoteAll(missing), anchorWindow),
			"update the cited lines or the anchor", nil
	}
	return "", "", "", nil
}

// verifySymbol checks a path#name pointer: the declaration of name exists, and the anchor's
// words are inside its span.
func (v *verifier) verifySymbol(p pointer, lines []string) (severity, message, fix string, err error) {
	lang := symbolLangs[strings.ToLower(path.Ext(p.path))]
	start, end, ok := declarationSpan(lines, lang, p.name)
	if !ok {
		return SeverityError, fmt.Sprintf("pointer %q: no declaration of %s in %s", p.span, p.name, p.path), "correct the name, or cite the lines instead", nil
	}
	if p.anchor == "" {
		return "", "", "", nil
	}
	if missing := absentWords(p.anchor, strings.Join(lines[start:end], "\n")); len(missing) > 0 {
		return SeverityError, fmt.Sprintf("the anchor for %q names %s, not found in the declaration (lines %d-%d)", p.span, quoteAll(missing), start+1, end),
			"update the anchor, or cite the lines instead", nil
	}
	return "", "", "", nil
}

// absentWords returns the words of anchor that text does not contain. A word is present when text
// holds it as a substring, as cairn's gate tests it.
func absentWords(anchor, text string) []string {
	var missing []string
	for _, w := range anchorWordRe.FindAllString(anchor, -1) {
		if !strings.Contains(text, w) {
			missing = append(missing, w)
		}
	}
	return missing
}

func quoteAll(words []string) string {
	q := make([]string, len(words))
	for i, w := range words {
		q[i] = strconv.Quote(w)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// read returns a file's lines, split on "\n" as cairn's gate splits them, so a file ending in a
// newline counts one more line than wc -l does.
func (v *verifier) read(abs string) *sourceFile {
	if f, ok := v.files[abs]; ok {
		return f
	}
	f := &sourceFile{}
	if data, err := os.ReadFile(abs); err != nil {
		f.err = err
	} else {
		f.lines = strings.Split(string(data), "\n")
	}
	v.files[abs] = f
	return f
}

// basenames lists the repository's files by base name, built on first use. A git work tree lists
// what git tracks plus the untracked files it does not ignore; anywhere else the root is walked,
// leaving out dot directories and node_modules.
func (v *verifier) basenames() (map[string][]string, error) {
	if v.basename != nil {
		return v.basename, nil
	}
	var paths []string
	cmd := exec.Command("git", "-C", v.root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if out, err := cmd.Output(); err == nil {
		for p := range strings.SplitSeq(string(out), "\x00") {
			if p == "" {
				continue
			}
			if info, err := os.Stat(filepath.Join(v.root, filepath.FromSlash(p))); err == nil && !info.IsDir() {
				paths = append(paths, p)
			}
		}
	} else {
		err := filepath.WalkDir(v.root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if name := d.Name(); p != v.root && (strings.HasPrefix(name, ".") || name == "node_modules") {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(v.root, p)
			if err != nil {
				return err
			}
			paths = append(paths, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("list the repository's files: %w", err)
		}
	}
	v.basename = map[string][]string{}
	for _, p := range paths {
		base := path.Base(p)
		v.basename[base] = append(v.basename[base], p)
	}
	for base, list := range v.basename {
		slices.Sort(list)
		v.basename[base] = slices.Compact(list)
	}
	return v.basename, nil
}

// declarationSpan finds the declaration of name in lines. It returns the 0-based line it starts
// on and the 0-based line after it ends, which is the next column-0 declaration or the end of the
// file. lang is "go" or "ts"; ts covers JavaScript.
//
// Go matches func, method, type, var, and const at column 0, and a name in a grouped block.
// TypeScript matches function, class, interface, type, enum, const, let, var, namespace, and
// module at column 0, and `declare global` for the name global. When no column-0 line matches, an
// indented const, let, var, or function does. This is the assembler's rule extended; the
// assembler's own copy stays in internal/assemble.
func declarationSpan(lines []string, lang, name string) (start, end int, ok bool) {
	q := regexp.QuoteMeta(name)
	tail := `(?:[^A-Za-z0-9_$]|$)`
	var column0, indented []*regexp.Regexp
	var member *regexp.Regexp
	next := goColumn0Re
	switch lang {
	case "go":
		column0 = []*regexp.Regexp{
			regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?` + q + tail),
			regexp.MustCompile(`^type\s+` + q + tail),
			regexp.MustCompile(`^(?:var|const)\s+` + q + tail),
		}
		member = regexp.MustCompile(`^\t` + q + `(?:\s|=|$)`)
	case "ts":
		next = tsColumn0Re
		column0 = []*regexp.Regexp{regexp.MustCompile(tsPrefix + q + tail)}
		if name == "global" {
			column0 = append(column0, regexp.MustCompile(`^declare\s+global\b`))
		}
		indented = []*regexp.Regexp{regexp.MustCompile(`^[ \t]+(?:async\s+)?(?:(?:const|let|var)\s+|function(?:\s*\*\s*|\s+))` + q + tail)}
	default:
		return 0, 0, false
	}
	found := -1
	inGroup := false
scan:
	for i, line := range lines {
		for _, re := range column0 {
			if re.MatchString(line) {
				found = i
				break scan
			}
		}
		switch {
		case goGroupRe.MatchString(line):
			inGroup = member != nil
		case line == ")":
			inGroup = false
		case inGroup && member.MatchString(line):
			found = i
			break scan
		}
	}
	for i := 0; found < 0 && i < len(lines); i++ {
		for _, re := range indented {
			if re.MatchString(lines[i]) {
				found = i
			}
		}
	}
	if found < 0 {
		return 0, 0, false
	}
	end = len(lines)
	for i := found + 1; i < len(lines); i++ {
		if next.MatchString(lines[i]) {
			end = i
			break
		}
	}
	return found, end, true
}

package provenance

import (
	"regexp"
	"strings"
)

// Kinds of fact a sentence can state, as the messages name them.
const (
	kindDate    = "date"
	kindVersion = "version"
	kindNumeral = "numeral"
	kindPath    = "path"
	kindCommand = "command"
	kindFlag    = "flag"
	kindName    = "name"
)

// token is one machine-extractable fact in a sentence.
type token struct{ kind, value string }

// genericHeads are the command-line tools a code span is read as a command for. A project's own
// tool joins them through commandHeads, once a fact states it.
var genericHeads = []string{"git", "go", "make", "npm", "npx", "pnpm", "yarn", "node", "curl", "docker", "python", "pip", "cargo"}

// pathExtensions are the file extensions that make a bare token a path.
const pathExtensions = "md|mdx|json|ya?ml|toml|txt|go|js|mjs|cjs|ts|py|rb|rs|java|sh|html|css|sql|csv|xml|ini|cfg|conf|log|lock"

// Go's regexp has no lookaround, so the patterns below hold only the core of each match. The
// scanner checks the bytes on either side of a candidate and, on a failed check, resumes one byte
// past its start, which is the order a lookbehind or lookahead would reject candidates in.
var (
	codeSpanRe    = regexp.MustCompile("`([^`]+)`")
	linkTargetRe  = regexp.MustCompile(`\]\([^)]*\)`)
	imageRe       = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	detailsTagRe  = regexp.MustCompile(`</?(?:details|summary)>`)
	urlRe         = regexp.MustCompile("https?://[^\\s<>)\\]`]+")
	dateRe        = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
	versionRe     = regexp.MustCompile(`v?\d+(?:\.\d+)+(?:-[0-9A-Za-z]+(?:\.[0-9A-Za-z]+)*)?`)
	numeralRe     = regexp.MustCompile(`(\d{1,3}(?:,\d{3})+|\d+)(?:%|[a-zA-Z]{1,3})?`)
	flagRe        = regexp.MustCompile(`--[a-z][\w-]*`)
	proseCmdRe    = regexp.MustCompile(`(?:npm|npx|pnpm|yarn)\s+(?:run\s+)?[a-z][\w:@-]*`)
	prosePathRe   = regexp.MustCompile(`(?:(?:~|\.{1,2})?/[\w.@\[\]-]+(?:/[\w.@\[\]-]*)*|[\w.@-]+(?:/[\w.@\[\]-]*)+)`)
	nameRe        = regexp.MustCompile(`^@?[A-Za-z_$][\w$:\[\]-]*(?:\.[A-Za-z_$][\w$-]*)*$`)
	packageNameRe = regexp.MustCompile(`^@[\w-]+/[\w.-]+$`)
	extensionRe   = regexp.MustCompile(`\.(?:` + pathExtensions + `)$`)
	flagLeadRe    = regexp.MustCompile(`^--[a-z][\w-]*`)
	flagSpanRe    = regexp.MustCompile(`^--[a-z][\w-]*(?:=\S*)?$`)
	commandWordRe = regexp.MustCompile(`^[a-z][\w:@-]*$`)
	rootedRe      = regexp.MustCompile(`^(?:~|\.{1,2})?/`)
	dotNameRe     = regexp.MustCompile(`^\.\w`)
)

func isWord(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// notBefore reports whether the byte before start is absent or outside the bad set.
func notBefore(s string, start int, bad func(byte) bool) bool {
	return start == 0 || !bad(s[start-1])
}

// notAfter reports whether the byte at end is absent or outside the bad set.
func notAfter(s string, end int, bad func(byte) bool) bool {
	return end >= len(s) || !bad(s[end])
}

// notDotDigit reports whether s at end does not begin with a dot and a digit, or with a comma and a
// digit when comma is set.
func notDotDigit(s string, end int, comma bool) bool {
	if end+1 >= len(s) || !isDigit(s[end+1]) {
		return true
	}
	return s[end] != '.' && (!comma || s[end] != ',')
}

func charIn(set string) func(byte) bool {
	return func(c byte) bool { return isWord(c) || strings.IndexByte(set, c) >= 0 }
}

// scan returns the [start, end] bounds of each match of re in s that ok accepts. A rejected
// candidate resumes the search one byte past its start.
func scan(re *regexp.Regexp, s string, ok func(start, end int) bool) [][2]int {
	var out [][2]int
	for from := 0; from <= len(s); {
		loc := re.FindStringIndex(s[from:])
		if loc == nil {
			break
		}
		start, end := from+loc[0], from+loc[1]
		if ok(start, end) {
			out = append(out, [2]int{start, end})
			from = end
			if end == start {
				from++
			}
			continue
		}
		from = start + 1
	}
	return out
}

func dateOK(s string, start, end int) bool {
	return notBefore(s, start, charIn(".-")) && notAfter(s, end, charIn("-"))
}

func versionOK(s string, start, end int) bool {
	return notBefore(s, start, charIn(".")) && notAfter(s, end, isWord) && notDotDigit(s, end, false)
}

func numeralOK(s string, start, end int) bool {
	return notBefore(s, start, charIn("$.")) && notAfter(s, end, isWord) && notDotDigit(s, end, true)
}

func flagOK(s string, start, _ int) bool { return notBefore(s, start, charIn("-")) }

func proseCmdOK(s string, start, _ int) bool { return notBefore(s, start, charIn("-")) }

func prosePathOK(s string, start, _ int) bool { return notBefore(s, start, charIn("./@~-")) }

// fullMatch reports whether all of s is one accepted match of re.
func fullMatch(re *regexp.Regexp, s string, ok func(string, int, int) bool) bool {
	for _, m := range scan(re, s, func(a, b int) bool { return ok(s, a, b) }) {
		if m[0] == 0 && m[1] == len(s) {
			return true
		}
	}
	return false
}

func trimTrailing(s string) string { return strings.TrimRight(s, ".,;:!?") }

func isProsePath(t string) bool {
	return rootedRe.MatchString(t) || strings.Count(t, "/") >= 2 || extensionRe.MatchString(t)
}

// commandHeads is the set of words that begin a command in a code span: the generic tools, and
// the first word of every code span of two or more words in a store bullet's claim.
func commandHeads(claims []string) map[string]bool {
	heads := map[string]bool{}
	for _, h := range genericHeads {
		heads[h] = true
	}
	for _, claim := range claims {
		for _, m := range codeSpanRe.FindAllStringSubmatch(claim, -1) {
			if words := strings.Fields(m[1]); len(words) >= 2 {
				heads[words[0]] = true
			}
		}
	}
	return heads
}

// spanTokens returns the facts one code span states: a command first, then any flag or path
// inside it.
func spanTokens(span string, heads map[string]bool) []token {
	content := strings.TrimSpace(span)
	switch {
	case fullMatch(dateRe, content, dateOK):
		return []token{{kindDate, content}}
	case fullMatch(versionRe, content, versionOK):
		return []token{{kindVersion, content}}
	case fullMatch(numeralRe, content, numeralOK):
		return []token{{kindNumeral, digitsOf(content)}}
	case flagSpanRe.MatchString(content):
		flag, _, _ := strings.Cut(content, "=")
		return []token{{kindFlag, flag}}
	}

	fields := strings.Fields(content)
	if len(fields) > 1 && heads[fields[0]] {
		words := fields[:1]
		for _, f := range fields[1:] {
			if !commandWordRe.MatchString(f) {
				break
			}
			words = append(words, f)
		}
		found := []token{{kindCommand, strings.Join(words, " ")}}
		for _, f := range fields[len(words):] {
			if flag := flagLeadRe.FindString(f); flag != "" {
				found = append(found, token{kindFlag, flag})
			} else if !packageNameRe.MatchString(f) && (strings.Contains(f, "/") || extensionRe.MatchString(f)) {
				found = append(found, token{kindPath, f})
			}
		}
		return found
	}

	switch {
	case fullMatch(urlRe, content, func(string, int, int) bool { return true }):
		return []token{{kindPath, content}}
	case packageNameRe.MatchString(content):
		return []token{{kindName, content}}
	case !strings.ContainsAny(content, " \t\n") && (strings.Contains(content, "/") || extensionRe.MatchString(content) || dotNameRe.MatchString(content)):
		return []token{{kindPath, content}}
	}
	if bare := strings.TrimSuffix(content, "()"); nameRe.MatchString(bare) {
		return []token{{kindName, bare}}
	}
	var found []token
	for _, f := range flagRe.FindAllString(content, -1) {
		found = append(found, token{kindFlag, f})
	}
	return found
}

func digitsOf(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// extract returns every machine-extractable fact in one sentence: code-span facts in span order,
// then prose facts by class (paths and URLs, commands, flags, dates, versions, numerals). A fact
// repeated in a sentence is listed once.
func extract(text string, heads map[string]bool) []token {
	text = detailsTagRe.ReplaceAllString(text, " ")
	text = imageRe.ReplaceAllString(text, " ")
	text = linkTargetRe.ReplaceAllString(text, "]")

	var found []token
	for _, m := range codeSpanRe.FindAllStringSubmatch(text, -1) {
		found = append(found, spanTokens(m[1], heads)...)
	}
	prose := codeSpanRe.ReplaceAllString(text, " ")

	// take records each accepted match of re in the remaining prose, then blanks it so a later,
	// looser class cannot read it again.
	take := func(re *regexp.Regexp, kind string, ok func(string, int, int) bool, value func(string) (string, bool)) {
		b := []byte(prose)
		for _, m := range scan(re, prose, func(a, e int) bool { return ok(prose, a, e) }) {
			match := prose[m[0]:m[1]]
			if kind == kindNumeral {
				match = numeralRe.FindStringSubmatch(match)[1]
			}
			v, use := value(match)
			if !use {
				continue
			}
			found = append(found, token{kind, v})
			for i := m[0]; i < m[1]; i++ {
				b[i] = ' '
			}
		}
		prose = string(b)
	}
	keep := func(s string) (string, bool) { return s, true }
	always := func(string, int, int) bool { return true }
	take(urlRe, kindPath, always, func(s string) (string, bool) { return trimTrailing(s), true })
	take(prosePathRe, kindPath, prosePathOK, func(s string) (string, bool) {
		s = trimTrailing(s)
		return s, isProsePath(s)
	})
	take(proseCmdRe, kindCommand, proseCmdOK, func(s string) (string, bool) { return strings.Join(strings.Fields(s), " "), true })
	take(flagRe, kindFlag, flagOK, keep)
	take(dateRe, kindDate, dateOK, keep)
	take(versionRe, kindVersion, versionOK, keep)
	take(numeralRe, kindNumeral, numeralOK, func(s string) (string, bool) { return strings.ReplaceAll(s, ",", ""), true })

	seen := map[token]bool{}
	var out []token
	for _, t := range found {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// contains reports whether a cited bullet states t. A numeral or version is looked for in the
// claim only, since a source pointer is full of line numbers; every other kind may sit in the
// claim or the bullet's source pointers, which whole holds.
func contains(claim, whole string, t token) bool {
	switch t.kind {
	case kindNumeral:
		return findBounded(stripThousands(claim), t.value, func(s string, i int) bool {
			return notBefore(s, i, charIn(".")) && notAfter(s, i+len(t.value), isDigit) && notDotDigit(s, i+len(t.value), false)
		})
	case kindVersion:
		bare := strings.TrimPrefix(t.value, "v")
		c := stripThousands(claim)
		return findBounded(c, bare, func(s string, i int) bool {
			start := i
			if start > 0 && s[start-1] == 'v' {
				start--
			}
			end := i + len(bare)
			return notBefore(s, start, charIn(".")) && notAfter(s, end, charIn("-")) && notDotDigit(s, end, false)
		})
	case kindDate:
		return strings.Contains(whole, t.value)
	case kindPath:
		return strings.Contains(whole, strings.TrimSuffix(t.value, "/"))
	case kindCommand:
		return findBounded(whole, t.value, func(s string, i int) bool {
			return notBefore(s, i, charIn("-")) && notAfter(s, i+len(t.value), charIn(":-"))
		})
	case kindFlag:
		return findBounded(whole, t.value, func(s string, i int) bool {
			return notBefore(s, i, charIn("-")) && notAfter(s, i+len(t.value), charIn("-"))
		})
	case kindName:
		return findBounded(whole, t.value, func(s string, i int) bool {
			return notBefore(s, i, charIn("$-")) && notAfter(s, i+len(t.value), charIn("$-"))
		})
	}
	return false
}

// findBounded reports whether needle occurs in hay at an index ok accepts.
func findBounded(hay, needle string, ok func(hay string, i int) bool) bool {
	for from := 0; from <= len(hay); {
		i := strings.Index(hay[from:], needle)
		if i < 0 {
			return false
		}
		if ok(hay, from+i) {
			return true
		}
		from += i + 1
	}
	return false
}

// stripThousands removes each comma between a digit and three more digits, so 1,284 reads 1284.
func stripThousands(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == ',' && i > 0 && isDigit(s[i-1]) && i+3 < len(s) && isDigit(s[i+1]) && isDigit(s[i+2]) && isDigit(s[i+3]) {
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

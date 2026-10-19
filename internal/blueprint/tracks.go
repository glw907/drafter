package blueprint

import (
	"bufio"
	"bytes"
	"cmp"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/stock"
)

const checkTracks = "tracks"

// standards lists the style standards a track may name.
var standards = []string{"google"}

// registerSection is the heading a register keeps its anatomy entries under.
const registerSection = "The page anatomies"

// globMatch reports whether name matches pattern, both slash paths. Each segment matches as
// path.Match does, except that a "**" segment matches any number of segments, none included.
func globMatch(pattern, name string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := range len(name) + 1 {
				if matchSegments(pat[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], name[0]); err != nil || !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

// globError returns the syntax error in pattern, or nil. A segment that path.Match rejects
// reports ErrBadPattern even against an empty name, so no page is needed to find it.
func globError(pattern string) error {
	for seg := range strings.SplitSeq(pattern, "/") {
		if seg == "**" {
			continue
		}
		if _, err := path.Match(seg, ""); err != nil {
			return fmt.Errorf("segment %q: %w", seg, err)
		}
	}
	return nil
}

// matches returns the indexes of the tracks whose globs match the page path.
func matches(tracks []Track, pagePath string) []int {
	var out []int
	for i, t := range tracks {
		if slices.ContainsFunc(t.Paths, func(g string) bool { return globMatch(g, pagePath) }) {
			out = append(out, i)
		}
	}
	return out
}

// registerEntries returns the anatomy names a register holds under its "## The page anatomies"
// section, and whether the section exists. An entry is a heading below the section's level or a
// list item whose bold lead is the name.
func registerEntries(src []byte) (entries []string, found bool) {
	sc := bufio.NewScanner(bytes.NewReader(src))
	sc.Buffer(nil, 1<<24)
	inSection := false
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \t")
		if level, text := atxHeading(line); level > 0 {
			switch {
			case level == 2 && text == registerSection:
				inSection, found = true, true
			case level <= 2:
				inSection = false
			case inSection:
				entries = append(entries, text)
			}
			continue
		}
		if !inSection {
			continue
		}
		for _, marker := range []string{"- **", "* **"} {
			if rest, ok := strings.CutPrefix(line, marker); ok {
				if name, _, ok := strings.Cut(rest, "**"); ok {
					entries = append(entries, strings.TrimSpace(strings.TrimRight(name, ":.")))
				}
			}
		}
	}
	return entries, found
}

// atxHeading returns the level and text of an ATX heading line, or 0.
func atxHeading(line string) (int, string) {
	level := len(line) - len(strings.TrimLeft(line, "#"))
	if level == 0 || level > 6 || len(line) == level || line[level] != ' ' {
		return 0, ""
	}
	return level, strings.TrimSpace(strings.TrimRight(line[level:], "# "))
}

// outlineChecks runs the checks on the outline's own declarations: its tracks, and each type's
// anatomy. They run once per project check, whichever blueprints are named.
func (c *projectCheck) outlineChecks() {
	o := c.l.outline
	registers := map[string]registerFile{}
	for _, t := range o.Tracks {
		if t.Register != "" {
			registers[t.Register] = c.readRegister(t.Register)
		}
	}
	c.trackChecks(registers)
	c.anatomyChecks(registers)
}

type registerFile struct {
	entries []string
	found   bool
	err     error
}

func (c *projectCheck) readRegister(rel string) registerFile {
	data, err := os.ReadFile(c.abs(rel))
	if err != nil {
		return registerFile{err: err}
	}
	entries, found := registerEntries(data)
	return registerFile{entries: entries, found: found}
}

func (c *projectCheck) trackChecks(registers map[string]registerFile) {
	o := c.l.outline
	if len(o.Tracks) == 0 {
		return
	}
	used := make([]bool, len(o.Tracks))
	for _, pg := range o.Pages {
		if !pg.Managed {
			continue
		}
		hit := matches(o.Tracks, path.Clean(pg.Path))
		for _, i := range hit {
			used[i] = true
		}
		line := cmp.Or(pg.Lines["path"], pg.Line)
		switch {
		case len(hit) == 0:
			c.add(c.Outline, line, plan.SeverityError, checkTracks,
				fmt.Sprintf("the managed page %s matches no track", pg.Path),
				"add a glob for it to a track's paths")
		case len(hit) > 1:
			c.add(c.Outline, line, plan.SeverityError, checkTracks,
				fmt.Sprintf("the managed page %s matches %d tracks (%s and %s)", pg.Path, len(hit), o.Tracks[hit[0]].Name, o.Tracks[hit[1]].Name),
				"narrow the tracks' paths so each page matches one track")
		}
	}
	for i, t := range o.Tracks {
		line := cmp.Or(t.Line, 1)
		if !slices.Contains(standards, t.Standard) && t.Standard != "" {
			c.add(c.Outline, cmp.Or(t.Lines["standard"], line), plan.SeverityError, checkTracks,
				fmt.Sprintf("track %q names the standard %q, which drafter does not ship", t.Name, t.Standard),
				"use "+strings.Join(standards, ", "))
		}
		if t.Register != "" {
			if r := registers[t.Register]; r.err != nil || !filepath.IsLocal(t.Register) {
				c.add(c.Outline, cmp.Or(t.Lines["register"], line), plan.SeverityError, checkTracks,
					fmt.Sprintf("track %q names the register %s, which cannot be read in the repository", t.Name, t.Register),
					"name a tracked file by its path from the repository root")
			}
		}
		for _, g := range t.Paths {
			if err := globError(g); err != nil {
				c.add(c.Outline, cmp.Or(t.Lines["paths"], line), plan.SeverityError, checkTracks,
					fmt.Sprintf("track %q has the malformed glob %q: %v", t.Name, g, err), "fix the pattern")
			}
		}
		if !used[i] {
			c.add(c.Outline, line, plan.SeverityWarning, checkTracks,
				fmt.Sprintf("track %q matches no managed page", t.Name), "fix its paths, or remove the track")
		}
	}
}

// anatomyChecks reports each declared type whose anatomy is neither a stock name nor an entry in
// the register of the track that holds its pages. With no tracks a type has no way to name a
// register, so an unknown anatomy is a warning instead of an error.
func (c *projectCheck) anatomyChecks(registers map[string]registerFile) {
	o := c.l.outline
	severity := plan.SeverityError
	if len(o.Tracks) == 0 {
		severity = plan.SeverityWarning
	}
	for _, decl := range o.Types {
		if decl.Anatomy == "" || slices.Contains(stock.Anatomies, decl.Anatomy) {
			continue
		}
		reason := c.unknownAnatomy(decl, registers)
		if reason == "" {
			continue
		}
		c.add(c.Outline, cmp.Or(decl.Lines["anatomy"], decl.Line, 1), severity, checkTypes,
			fmt.Sprintf("the type %q names the anatomy %q, which is not a stock anatomy%s", decl.Name, decl.Anatomy, reason),
			"use a stock anatomy, one of "+strings.Join(stock.Anatomies, ", ")+", or add an entry for it to a register that a track names with register")
	}
}

// unknownAnatomy returns why no register holds decl's anatomy, or "" when one does.
func (c *projectCheck) unknownAnatomy(decl Type, registers map[string]registerFile) string {
	o := c.l.outline
	if len(o.Tracks) == 0 {
		return "; with no tracks declared, no register can supply it"
	}
	holds := func(t Track) (bool, string) {
		if t.Register == "" {
			return false, fmt.Sprintf("track %q names no register", t.Name)
		}
		r := registers[t.Register]
		switch {
		case r.err != nil:
			return false, fmt.Sprintf("the register %s cannot be read", t.Register)
		case !r.found:
			return false, fmt.Sprintf("the register %s has no %q section", t.Register, "## "+registerSection)
		}
		return slices.Contains(r.entries, decl.Anatomy), fmt.Sprintf("the register %s has no entry for it", t.Register)
	}
	typed := false
	for _, pg := range o.Pages {
		if !pg.Managed || pg.Type != decl.Name {
			continue
		}
		typed = true
		hit := matches(o.Tracks, path.Clean(pg.Path))
		if len(hit) != 1 {
			continue // the track check reports the page
		}
		if ok, why := holds(o.Tracks[hit[0]]); !ok {
			return fmt.Sprintf("; for %s, %s", pg.Path, why)
		}
	}
	if typed {
		return ""
	}
	why := "no track names a register"
	for _, t := range o.Tracks {
		ok, w := holds(t)
		if ok {
			return ""
		}
		if t.Register != "" {
			why = w
		}
	}
	return "; " + why
}

// TrackFor returns the one track whose globs match the page path. It reports false when no
// track matches or more than one does, which drafter check reports as an error.
func (o *Outline) TrackFor(pagePath string) (Track, bool) {
	hit := matches(o.Tracks, path.Clean(pagePath))
	if len(hit) != 1 {
		return Track{}, false
	}
	return o.Tracks[hit[0]], true
}

// AnatomyOf returns the anatomy the declared type follows, or "" for a type with none or one the
// outline doesn't declare.
func (o *Outline) AnatomyOf(typeName string) string {
	for _, t := range o.Types {
		if t.Name == typeName {
			return t.Anatomy
		}
	}
	return ""
}

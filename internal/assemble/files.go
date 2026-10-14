package assemble

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/gate"
)

// sizeLimit is the largest file drafter inlines.
const sizeLimit = 256 << 10

// repo is the project a request is assembled in: its root, the root with symlinks resolved, the
// files git tracks, and the out-of-repo paths the configuration allows.
type repo struct {
	root    string
	real    string
	tracked map[string]bool
	allowed []string
}

func openRepo(ctx context.Context, c *config.Config) (*repo, error) {
	if c.Root == "" {
		return nil, errors.New("assemble: the configuration has no project root")
	}
	root := filepath.Clean(c.Root)
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("assemble: resolve project root: %w", err)
	}
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-z")
	cmd.Dir = root
	cmd.Env = gate.Env(c)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("assemble: list tracked files: %w", err)
	}
	r := &repo{root: root, real: real, tracked: map[string]bool{}}
	for name := range strings.SplitSeq(string(out), "\x00") {
		if name != "" {
			r.tracked[name] = true
		}
	}
	for _, e := range c.InlineFiles {
		if p, ok := expand(e); ok {
			r.allowed = append(r.allowed, resolveLoose(p))
		}
	}
	return r, nil
}

// expand returns p with a leading ~ replaced by the home directory, cleaned, and reports whether
// the result is absolute.
func expand(p string) (string, bool) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		p = filepath.Join(home, p[1:])
	}
	if !filepath.IsAbs(p) {
		return "", false
	}
	return filepath.Clean(p), true
}

// resolveLoose resolves symlinks in the longest existing prefix of p, a cleaned absolute path,
// and appends the rest, so a path that does not exist still compares against real ones.
func resolveLoose(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(resolveLoose(parent), filepath.Base(p))
}

// within reports whether p is dir or lies below it, matching only at a separator boundary.
func within(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}

func (r *repo) allows(real string) bool {
	for _, a := range r.allowed {
		if within(real, a) {
			return true
		}
	}
	return false
}

// tokens splits text on whitespace, backticks, quotes, parentheses, brackets, angle brackets, and
// commas, in order of first appearance.
func tokens(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range strings.FieldsFunc(text, func(c rune) bool {
		return c == ' ' || c == '\t' || c == '\n' || c == '\r' || strings.ContainsRune("`'\"()[]<>,", c)
	}) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// target is one file a token names, with the path it is shown under and the path it is read from.
type target struct {
	name string
	real string
}

// resolve returns the file token names, when drafter may inline it. A tracked file whose symlinks
// stay inside the root is shown by its repo-relative path; any other path must be allowed by
// inline_files after resolution.
func (r *repo) resolve(token string) (target, bool) {
	if rel, ok := r.trackedRel(token); ok {
		real := resolveLoose(filepath.Join(r.root, rel))
		if within(real, r.real) {
			return target{rel, real}, true
		}
		return r.outOfRepo(filepath.Join(r.root, rel), real)
	}
	p, ok := expand(token)
	if !ok {
		return target{}, false
	}
	if rel, ok := r.under(p); ok {
		if !r.tracked[rel] {
			return target{}, false
		}
		return r.resolve(rel)
	}
	return r.outOfRepo(p, resolveLoose(p))
}

func (r *repo) outOfRepo(name, real string) (target, bool) {
	if within(real, r.real) || !r.allows(real) {
		return target{}, false
	}
	return target{name, real}, true
}

// trackedRel returns token as a tracked repo-relative path, also trying it without a trailing
// sentence mark.
func (r *repo) trackedRel(token string) (string, bool) {
	for _, t := range []string{token, strings.TrimRight(token, ".:;!?")} {
		if r.tracked[t] {
			return t, true
		}
	}
	return "", false
}

// under returns p relative to the root when p lies in it, by either spelling of the root.
func (r *repo) under(p string) (string, bool) {
	for _, root := range []string{r.root, r.real} {
		if p != root && within(p, root) {
			rel, err := filepath.Rel(root, p)
			return filepath.ToSlash(rel), err == nil
		}
	}
	return "", false
}

// fileBlocks returns one block for each distinct file the texts name and drafter may inline.
func (r *repo) fileBlocks(texts ...string) []string {
	var blocks []string
	seen := map[string]bool{}
	for _, text := range texts {
		for _, tok := range tokens(text) {
			t, ok := r.resolve(tok)
			if !ok && strings.TrimRight(tok, ".:;!?") != tok {
				t, ok = r.resolve(strings.TrimRight(tok, ".:;!?"))
			}
			if !ok || seen[t.real] {
				continue
			}
			seen[t.real] = true
			if b, ok := fileBlock(t); ok {
				blocks = append(blocks, b)
			}
		}
	}
	return blocks
}

// pageBlocks returns a block for each repo-relative path, which must exist.
func (r *repo) pageBlocks(paths ...string) ([]string, error) {
	var blocks []string
	for _, p := range paths {
		b, ok := fileBlock(target{p, filepath.Join(r.root, p)})
		if !ok {
			return nil, fmt.Errorf("assemble: %s is not a file", p)
		}
		blocks = append(blocks, b)
	}
	return blocks, nil
}

// fileBlock reads the file t names. It reports false for a directory or another special file.
func fileBlock(t target) (string, bool) {
	info, err := os.Stat(t.real)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return block("file", t.name, "not inlined: missing"), true
	case err != nil || !info.Mode().IsRegular():
		return "", false
	case info.Size() > sizeLimit:
		return block("file", t.name, fmt.Sprintf("not inlined: %d bytes", info.Size())), true
	}
	data, err := os.ReadFile(t.real)
	if err != nil {
		return "", false
	}
	return block("file", t.name, string(data)), true
}

// block wraps body between the header and footer lines every block shares.
func block(kind, label, body string) string {
	return "=== " + kind + ": " + label + " ===\n" + strings.TrimSuffix(body, "\n") + "\n=== end " + kind + ": " + label + " ==="
}

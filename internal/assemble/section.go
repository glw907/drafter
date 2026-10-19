package assemble

import (
	"fmt"
	"os"
	"strings"

	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/stock"
)

// drafterPrefix starts a token that names a file embedded in the drafter binary.
const drafterPrefix = "drafter:"

// tokenBlock returns the block a "drafter:<name>" token or a "<path>#<slug>" token yields, or false
// when tok is neither or names a path drafter may not inline.
func (r *repo) tokenBlock(tok string) (string, bool) {
	path, slug, ok := strings.Cut(tok, "#")
	if !ok {
		if name, ok := strings.CutPrefix(tok, drafterPrefix); ok {
			return drafterBlock(tok, name)
		}
		return "", false
	}
	if path == "" || slug == "" {
		return "", false
	}
	if name, ok := strings.CutPrefix(path, drafterPrefix); ok {
		data, found := stock.Files(name)
		if !found {
			return block("excerpt", tok, "not inlined: no drafter file "+name), true
		}
		return sectionBlock(tok, data, slug), true
	}
	t, ok := r.resolve(path)
	if !ok {
		return "", false
	}
	data, err := os.ReadFile(t.real)
	if err != nil {
		if os.IsNotExist(err) {
			return block("excerpt", tok, "not inlined: missing"), true
		}
		return "", false
	}
	return sectionBlock(tok, data, slug), true
}

// drafterBlock inlines the embedded file name under the label tok. An empty name is prose, not a
// token.
func drafterBlock(tok, name string) (string, bool) {
	if name == "" {
		return "", false
	}
	data, ok := stock.Files(name)
	if !ok {
		return block("file", tok, "not inlined: no drafter file "+name), true
	}
	return block("file", tok, string(data)), true
}

// sectionBlock returns the section of a Markdown file whose heading has slug: from that heading to
// the next heading of the same or a higher level. The size limit applies to the section.
func sectionBlock(label string, src []byte, slug string) string {
	heads := plan.Headings(src)
	at := -1
	for i, h := range heads {
		if h.Slug == slug {
			at = i
			break
		}
	}
	if at < 0 {
		return block("excerpt", label, "no excerpt: no heading "+slug)
	}
	lines := strings.Split(string(src), "\n")
	end := len(lines)
	for _, h := range heads[at+1:] {
		if h.Level <= heads[at].Level && h.Line > heads[at].Line {
			end = h.Line - 1
			break
		}
	}
	body := strings.TrimRight(strings.Join(lines[heads[at].Line-1:end], "\n"), "\n")
	if len(body) > sizeLimit {
		return block("excerpt", label, fmt.Sprintf("not inlined: %d bytes", len(body)))
	}
	return block("excerpt", label, body)
}

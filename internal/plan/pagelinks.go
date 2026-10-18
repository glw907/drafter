package plan

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// schemeRe matches a destination that starts with a URL scheme (https:, mailto:) or is
// protocol-relative; neither names a file in the repository.
var schemeRe = regexp.MustCompile(`^(?:[A-Za-z][A-Za-z0-9+.-]*:|//)`)

// CheckPageLinks checks the internal links of the Markdown page at page, a slash path relative
// to root. Each link or image whose destination has no URL scheme must resolve, from the page's
// directory or from root when it starts with a slash, to a path inside root that exists, and a
// #anchor must match a heading of its target, or of the page itself for a bare anchor. Links
// inside code are not links. Findings use the links check, on the link's line in the page. The
// error means the page could not be read.
func CheckPageLinks(root, page string) ([]Finding, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(page)))
	if err != nil {
		return nil, fmt.Errorf("read page: %w", err)
	}
	src := withoutFrontMatter(data)
	offset := len(data) - len(src)
	doc := goldmark.New().Parser().Parse(text.NewReader(src))
	own := anchorsOf(data)
	targets := map[string]map[string]bool{}
	var findings []Finding
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var dest []byte
		switch n := n.(type) {
		case *ast.Link:
			dest = n.Destination
		case *ast.Image:
			dest = n.Destination
		default:
			return ast.WalkContinue, nil
		}
		target := string(dest)
		if schemeRe.MatchString(target) {
			return ast.WalkContinue, nil
		}
		line := 1 + bytes.Count(data[:offset+linkOffset(n)], []byte("\n"))
		add := func(msg, fix string) {
			findings = append(findings, Finding{File: page, Line: line, Severity: SeverityError, Check: checkLinks, Message: msg, Fix: fix})
		}
		file, anchor, _ := cutDecoded(target)
		if file == "" {
			if anchor != "" && !own[anchor] {
				add(fmt.Sprintf("anchor #%s matches no heading on this page", anchor), "use the slug of a heading on this page")
			}
			return ast.WalkContinue, nil
		}
		rel := path.Join(path.Dir(page), file)
		if rest, ok := strings.CutPrefix(file, "/"); ok {
			rel = path.Clean(rest)
		}
		if rel == ".." || strings.HasPrefix(rel, "../") {
			add(fmt.Sprintf("target %s leaves the repository", file), "point the link at a file under the repository root")
			return ast.WalkContinue, nil
		}
		full := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		switch {
		case os.IsNotExist(err):
			add(fmt.Sprintf("target %s does not exist", rel), "correct the path")
		case err != nil:
			add(fmt.Sprintf("target %s cannot be read: %v", rel, err), "fix the file's permissions")
		case anchor == "":
		case info.IsDir():
			add(fmt.Sprintf("target %s is a directory, which has no headings for #%s to match", rel, anchor),
				"link to a file in the directory, or drop the anchor")
		default:
			anchors, ok := targets[rel]
			if !ok {
				body, err := os.ReadFile(full)
				if err != nil {
					add(fmt.Sprintf("target %s cannot be read: %v", rel, err), "fix the file's permissions")
					return ast.WalkContinue, nil
				}
				anchors = anchorsOf(body)
				targets[rel] = anchors
			}
			if !anchors[anchor] {
				add(fmt.Sprintf("anchor #%s matches no heading in %s", anchor, rel),
					"use the slug of a heading in that file, or add the heading")
			}
		}
		return ast.WalkContinue, nil
	})
	return findings, nil
}

// cutDecoded splits a destination at its first # and percent-decodes both parts.
func cutDecoded(target string) (file, anchor string, found bool) {
	f, a, found := strings.Cut(target, "#")
	if v, err := url.PathUnescape(f); err == nil {
		f = v
	}
	if v, err := url.PathUnescape(a); err == nil {
		a = v
	}
	return f, a, found
}

// linkOffset is a byte offset in src that lies on the link's line: its first text child, or the
// start of the nearest enclosing block.
func linkOffset(n ast.Node) int {
	first := -1
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := c.(*ast.Text); ok && entering {
			first = t.Segment.Start
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	if first >= 0 {
		return first
	}
	for p := n.Parent(); p != nil; p = p.Parent() {
		if p.Type() == ast.TypeBlock && p.Lines().Len() > 0 {
			return p.Lines().At(0).Start
		}
	}
	return 0
}

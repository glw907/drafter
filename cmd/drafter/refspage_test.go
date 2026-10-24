package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRefsPage(t *testing.T) {
	outline := strings.Replace(readCatalog(t, "outline.yaml"), "pages:\n", "pages:\n  - path: docs/keys.md\n    slug: keys\n    managed: false\n", 1) +
		"crossLinks:\n  - from: guide\n    to: keys\n    why: The guide sends readers to the reference.\n"
	root := project(t, map[string]string{"blueprints/outline.yaml": outline})

	t.Run("lists a crossLink", func(t *testing.T) {
		out, _, code := run(t, "refs", "--root", root, "--page", "keys")
		if code != 0 || !strings.HasPrefix(out, "blueprints/outline.yaml:") || !strings.HasSuffix(out, ": crossLink\n") {
			t.Errorf("code %d, stdout %q", code, out)
		}
		jsonOut, _, code := run(t, "refs", "--root", root, "--json", "--page", "docs/keys.md")
		var rows []pageRefRow
		if err := json.Unmarshal([]byte(jsonOut), &rows); err != nil || code != 0 || len(rows) != 1 || rows[0].Kind != "crossLink" || rows[0].Line < 1 {
			t.Errorf("json %q, code %d, err %v", jsonOut, code, err)
		}
	})
	t.Run("a page nothing links to exits 0", func(t *testing.T) {
		out, _, code := run(t, "refs", "--root", root, "--json", "--page", "guide")
		if code != 0 || strings.TrimSpace(out) != "[]" {
			t.Errorf("code %d, stdout %q", code, out)
		}
	})
	t.Run("an unknown page exits 1", func(t *testing.T) {
		_, errs, code := run(t, "refs", "--root", root, "--page", "nowhere")
		if code != 1 || !strings.Contains(errs, "nowhere") {
			t.Errorf("code %d, stderr %q", code, errs)
		}
	})
	t.Run("a fact id with --page is a usage error", func(t *testing.T) {
		if _, _, code := run(t, "refs", "--root", root, "--page", "keys", "f:bbbbbb"); code != 2 {
			t.Errorf("code %d, want 2", code)
		}
	})
}

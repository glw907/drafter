package main

import (
	"regexp"
	"testing"

	"github.com/glw907/drafter/internal/config"
)

// An outline that fails to parse names the line yaml.v3 reports, at each site that decodes it.
func TestMalformedOutlineNamesItsLine(t *testing.T) {
	const bad = "version: 1\ntypes: [\n"
	atLine := regexp.MustCompile(`blueprints/outline\.yaml:\d+: `)

	t.Run("drafter outline", func(t *testing.T) {
		root := project(t, map[string]string{"blueprints/outline.yaml": bad})
		_, errs, code := run(t, "outline", "docs/guide.md", "--root", root)
		if code != 2 || !atLine.MatchString(errs) {
			t.Errorf("code %d, stderr %q, want 2 and file:line", code, errs)
		}
	})

	f := newCLIFixture(t)
	f.write("blueprints/outline.yaml", bad)
	f.commit("a broken outline", "blueprints/outline.yaml")
	c, err := config.Load(f.root, config.Flags{})
	if err != nil {
		t.Fatal(err)
	}
	c.Prompts = ""

	t.Run("the stock track check", func(t *testing.T) {
		err := checkStockTracks(c, f.root, f.rev("HEAD"), nil)
		if err == nil || !atLine.MatchString(err.Error()) {
			t.Errorf("error %v, want file:line", err)
		}
	})
	t.Run("blueprint handoffs", func(t *testing.T) {
		_, err := blueprintHandoffs(c, f.root, f.rev("HEAD"), []string{f.blueprint()})
		if err == nil || !atLine.MatchString(err.Error()) {
			t.Errorf("error %v, want file:line", err)
		}
	})
}

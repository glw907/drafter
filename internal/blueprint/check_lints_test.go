package blueprint

import (
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

func TestCheckExhaustiveEachMatchOwnFinding(t *testing.T) {
	md := strings.Replace(goodMD, "- You need the admin role.", "- Only admins rotate all keys, and always agree.", 1)
	got, err := mdProject(t, md, nil).Check(nil)
	if err != nil {
		t.Fatal(err)
	}
	var claims []string
	for _, f := range findingsFor(got, plan.CheckExhaustive) {
		if f.Line != lineOf(t, md, "Only admins") {
			t.Errorf("finding on line %d, want the bullet's line", f.Line)
		}
		claims = append(claims, f.Message)
	}
	if len(claims) != 3 {
		t.Fatalf("findings = %q, want one each for only, all, and always", claims)
	}
	for i, want := range []string{`"only"`, `"all"`, `"always"`} {
		if !strings.Contains(claims[i], want) {
			t.Errorf("finding %d = %q, want %s", i, claims[i], want)
		}
	}
}

func TestCheckSentenceLength(t *testing.T) {
	const bullet = "- You need the admin role."
	tests := []struct {
		name, old, new string
		limit          int
		anch           string // the text on the line under test; the rest of the blueprint is not
		words          string // "" for no warning
	}{
		{"off by default", bullet, "- You need the admin role to rotate the key in the settings page.", 0, "You need the admin role to", ""},
		{"at the limit passes", bullet, bullet, 5, "You need the admin role.", ""},
		{"over the limit warns", bullet, bullet, 4, "You need the admin", "5 words"},
		{"a code span is one word", bullet, "- Run `rotate --all now --force` today.", 3, "Run `rotate", ""},
		{"the second sentence counts alone", bullet, "- Sign in. Then you need the admin role.", 6, "Sign in.", ""},
		{"a long sentence after a short one", bullet, "- Sign in. Then you need the admin role.", 5, "Sign in", "6 words"},
		{"a row line", "**Fix.** Ask an admin to grant the role.", "**Fix.** Ask an admin to grant the role.", 4, "**Fix.**", "7 words"},
		{"a reader line", "An operator who rotates keys.", "An operator who rotates keys.", 3, "**Who reads it.**", "5 words"},
		{"a constraints line", "## Before you begin", ":::constraints\nState one two three four five.\n:::\n\n## Before you begin", 5, "State one", "6 words"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := strings.Replace(goodMD, tt.old, tt.new, 1)
			p := mdProject(t, md, nil)
			p.SentenceWords = tt.limit
			got, err := p.Check(nil)
			if err != nil {
				t.Fatal(err)
			}
			ws := slices.DeleteFunc(findingsFor(got, checkSentence), func(f plan.Finding) bool { return f.Line != lineOf(t, md, tt.anch) })
			if tt.words == "" {
				if len(ws) != 0 {
					t.Fatalf("warnings = %+v, want none", ws)
				}
				return
			}
			if len(ws) != 1 || ws[0].Severity != plan.SeverityWarning || !strings.Contains(ws[0].Message, tt.words) {
				t.Fatalf("warnings = %+v, want one naming %q", ws, tt.words)
			}
		})
	}
}

func TestCheckNamedSkipsOutlineChecks(t *testing.T) {
	outline := strings.Replace(readFixture(t, "outline.yaml"), "pages:\n", "  - name: lookup\n    anatomy: Symptom row\npages:\n", 1)
	p := catalogProject(t, map[string]string{"blueprints/outline.yaml": outline})
	tests := []struct {
		name  string
		named []string
		want  bool
	}{
		{"unnamed reports the unused type", nil, true},
		{"named skips it", []string{guideBlueprint}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.Check(tt.named)
			if err != nil {
				t.Fatal(err)
			}
			var found bool
			for _, f := range findingsFor(got, "types") {
				found = found || strings.Contains(f.Message, `"lookup"`)
			}
			if found != tt.want {
				t.Errorf("types finding for lookup = %v, want %v: %+v", found, tt.want, got)
			}
		})
	}
}

func TestCrossLinkToUnmanagedPageResolves(t *testing.T) {
	outline := readFixture(t, "outline.yaml") +
		"  - path: docs/legacy.md\n    managed: false\ncrossLinks:\n  - from: guide\n    to: docs/legacy.md\n    why: The reader continues there.\n"
	p := catalogProject(t, map[string]string{"blueprints/outline.yaml": outline})
	got, err := p.Check([]string{guideBlueprint})
	if err != nil {
		t.Fatal(err)
	}
	var unlinked bool
	for _, f := range findingsFor(got, "links") {
		if strings.Contains(f.Message, "names no page the outline lists") {
			t.Errorf("the cross-link to an unmanaged page did not resolve: %+v", f)
		}
		unlinked = unlinked || strings.Contains(f.Message, "no link item targets docs/legacy.md")
	}
	if !unlinked {
		t.Errorf("the resolved cross-link was not checked for a link item: %+v", got)
	}
}

func TestLinkAnchorsFromTargetBlueprint(t *testing.T) {
	const otherMD = "---\npage: Other page\ntype: task guide\ntrack: operate\n---\n\n# Other page\n\n:::reader\n**Who reads it.** An operator. :fact[aaaaaa]\n:::\n\n## Extra setup\n\n*Sets up the extra.*\n\n- Do the setup.\n"
	outline := readFixture(t, "outline.yaml") + tracksBlock
	outline = strings.Replace(outline, "pages:\n", "pages:\n  - path: docs/other.md\n    slug: other\n    title: Other page\n    type: task guide\n    job: A job.\n  - path: docs/plain.md\n    managed: false\n", 1)
	tests := []struct {
		name, target string
		want         string // "" for no links error
	}{
		{"a heading only the blueprint has", "other.md#extra-setup", ""},
		{"the blueprint's title", "other.md#other-page", ""},
		{"a heading the published page has and the blueprint lacks", "other.md#old-heading", "matches no heading in the blueprint for docs/other.md"},
		{"a page with no blueprint uses the published page", "plain.md#plain-heading", ""},
		{"a missing heading on a page with no blueprint", "plain.md#nope", "matches no heading in docs/plain.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := strings.Replace(goodMD, "- See [the key reference](https://example.com/docs/keys).", "- See [the page]("+tt.target+") for the setup case.", 1)
			p := mdProject(t, md, map[string]string{
				"blueprints/outline.yaml":            outline,
				"blueprints/docs/other.blueprint.md": otherMD,
				"docs/other.md":                      "# Other page\n\n## Old heading\n",
				"docs/plain.md":                      "# Plain\n\n## Plain heading\n",
			})
			got, err := p.Check([]string{guideMD})
			if err != nil {
				t.Fatal(err)
			}
			var errs []plan.Finding
			for _, f := range findingsFor(got, "links") {
				if f.Severity == plan.SeverityError {
					errs = append(errs, f)
				}
			}
			if tt.want == "" {
				if len(errs) != 0 {
					t.Fatalf("links errors = %+v, want none", errs)
				}
				return
			}
			if len(errs) != 1 || !strings.Contains(errs[0].Message, tt.want) {
				t.Fatalf("links errors = %+v, want one containing %q", errs, tt.want)
			}
		})
	}
}

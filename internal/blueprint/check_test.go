package blueprint

import (
	"os"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

const guideBlueprint = "blueprints/docs/guide.blueprint.yaml"

// lineOf returns the 1-based line of the first line of text that contains frag.
func lineOf(t *testing.T, text, frag string) int {
	t.Helper()
	for i, l := range strings.Split(text, "\n") {
		if strings.Contains(l, frag) {
			return i + 1
		}
	}
	t.Fatalf("no line contains %q", frag)
	return 0
}

func TestCheckFramingCitations(t *testing.T) {
	typo := withEdit(t, "for the two-key model.", "for the two-key model and `f:aaaaab`.")
	dupFacts := "# Facts\n\n- `f:aaaaaa` A second bullet with the same id. Source: `src/x.go`. [verified]\n"
	tests := []struct {
		name  string
		extra map[string]string
		msg   string
		line  int
	}{
		{"typo'd id", map[string]string{guideBlueprint: typo}, `"f:aaaaab" is in no facts file`, lineOf(t, typo, "readers: |")},
		{"id on two bullets", map[string]string{"blueprints/facts/more.md": dupFacts}, `"f:aaaaaa" is on 2 bullets`, lineOf(t, readFixture(t, "base.blueprint.yaml"), "readers: |")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := catalogProject(t, tt.extra)
			got, err := p.Check(nil)
			if err != nil {
				t.Fatal(err)
			}
			var hit bool
			for _, f := range findingsFor(got, "facts") {
				if f.Severity == plan.SeverityError && strings.Contains(f.Message, tt.msg) && f.Line == tt.line {
					hit = true
				}
			}
			if !hit {
				t.Errorf("no facts error %q at line %d in %+v", tt.msg, tt.line, got)
			}
		})
	}
}

func TestCheckNewChecks(t *testing.T) {
	baseOutline := readFixture(t, "outline.yaml")
	base := readFixture(t, "base.blueprint.yaml")
	tests := []struct {
		name  string
		extra map[string]string
		req   bool
		file  string
		check string
		sev   string // empty means the twin passes: no finding of the check at all
		msg   string
	}{
		{"managed page without a blueprint warns", map[string]string{guideBlueprint: ""}, false, "blueprints/outline.yaml", "blueprints", "warning", "has no blueprint"},
		{"managed page without a blueprint errors when required", map[string]string{guideBlueprint: ""}, true, "blueprints/outline.yaml", "blueprints", "error", "has no blueprint"},
		{"every managed page has one", nil, true, "", "blueprints", "", ""},
		{"blueprint for an unlisted page", map[string]string{"blueprints/docs/other.blueprint.yaml": base}, false, "blueprints/docs/other.blueprint.yaml", "blueprints", "error", "lists no page"},
		{"blueprint for an unmanaged page", map[string]string{
			"blueprints/docs/other.blueprint.yaml": base,
			"blueprints/outline.yaml":              baseOutline + "  - path: docs/other.md\n    managed: false\n",
		}, false, "blueprints/docs/other.blueprint.yaml", "blueprints", "error", "managed: false"},
		{"blueprint away from the path rule", map[string]string{"blueprints/.blueprint.yaml": base}, false, "blueprints/.blueprint.yaml", "blueprints", "error", "path-rule location"},
		{"wrong blueprint extension", map[string]string{"blueprints/docs/x.blueprint.yml": base}, false, "blueprints/docs/x.blueprint.yml", "blueprints", "error", "path-rule location"},
		{"undeclared page type", map[string]string{"blueprints/outline.yaml": strings.Replace(baseOutline, "    type: task guide", "    type: tutorial", 1)}, false,
			"blueprints/outline.yaml", "types", "error", `"tutorial"`},
		{"type with no anatomy", map[string]string{"blueprints/outline.yaml": strings.Replace(baseOutline, "    anatomy: Task guide\n", "", 1)}, false,
			"blueprints/outline.yaml", "types", "error", "names no anatomy"},
		{"section reference that resolves", map[string]string{guideBlueprint: strings.Replace(base,
			"The reader needs the admin role before the first step.", "The reader needs the admin role before [[rotate]] begins.", 1)}, false, "", "refs", "", ""},
		{"use opening Omit", map[string]string{guideBlueprint: strings.Replace(base, "Only the role requirement.", "Omit the sizes.", 1)}, false, "", "use", "", ""},
		{"use opening with a longer word", map[string]string{guideBlueprint: strings.Replace(base, "Only the role requirement.", "Onlyx the role.", 1)}, false,
			guideBlueprint, "use", "error", "Only or Omit"},
		{"no use at all", map[string]string{guideBlueprint: strings.Replace(base, "        use: |\n          Only the role requirement.\n", "", 1)}, false, "", "use", "", ""},
		{"lock entry for a drafted page on disk", map[string]string{"blueprints/drafted.lock": "version: 1\npages:\n  docs/guide.md:\n    page: aa\n"}, false, "", "lock", "", ""},
		{"lock entry for a page missing from disk", map[string]string{"blueprints/drafted.lock": "version: 1\npages:\n  docs/gone.md:\n    page: aa\n"}, false,
			"blueprints/drafted.lock", "lock", "error", "not on disk"},
		{"accepted-only entry for a listed page", map[string]string{"blueprints/drafted.lock": "version: 1\npages:\n  docs/guide.md:\n    accepted: aa\n"}, false, "", "lock", "", ""},
		{"failure handled by a fact", map[string]string{guideBlueprint: withEdit(t, "handledBy: h-ask-admin", "handledBy: needs-admin")}, false, guideBlueprint, "handler", "warning", "hand off to the step or section"},
		{"failure handled by a step", map[string]string{guideBlueprint: withEdit(t, "handledBy: h-ask-admin", "handledBy: s-verify")}, false, "", "handler", "", ""},
		{"failure handled by a verbatim sentence", map[string]string{guideBlueprint: handlerAs(t, "sentence\n        mode: verbatim")}, false, guideBlueprint, "handler", "warning", "hand off to the step or section"},
		{"failure handled by a claim sentence", map[string]string{guideBlueprint: handlerAs(t, "sentence\n        mode: claim")}, false, "", "handler", "", ""},
		{"failure handled by a link back by anchor", map[string]string{guideBlueprint: handlerLink(t, "target: \"#rotate-the-key\"\n        text: Ask an admin")}, false, guideBlueprint, "handler", "warning", "hand off to the step or section"},
		{"failure handled by a link to another section's anchor", map[string]string{guideBlueprint: handlerLink(t, "target: \"#verify-the-new-key\"\n        text: Ask an admin")}, false, "", "handler", "", ""},
		{"failure handled by a link back by section id", map[string]string{guideBlueprint: handlerLink(t, "target: https://example.com/docs/keys\n        text: Back to [[rotate]]")}, false, guideBlueprint, "handler", "warning", "hand off to the step or section"},
		{"failure handled by a link to another section by id", map[string]string{guideBlueprint: handlerLink(t, "target: https://example.com/docs/keys\n        text: On to [[verify]]")}, false, "", "handler", "", ""},
		{"failure handled by a handoff back to the section", map[string]string{guideBlueprint: withEdit(t, "Ask an admin to grant the role, then try again.", "Go back to [[rotate]] and try again.")}, false, guideBlueprint, "handler", "warning", "hand off to the step or section"},
		{"failure handled by a handoff to another section", map[string]string{guideBlueprint: withEdit(t, "Ask an admin to grant the role, then try again.", "Go on to [[verify]] and try again.")}, false, "", "handler", "", ""},
		{"accepted-only entry for an unlisted page", map[string]string{"blueprints/drafted.lock": "version: 1\npages:\n  docs/new.md:\n    accepted: aa\n"}, false,
			"blueprints/drafted.lock", "lock", "error", "lists no such page"},
		{"bar-only entry for a listed page", map[string]string{"blueprints/drafted.lock": barOnlyLock("docs/guide.md")}, false, "", "lock", "", ""},
		{"bar-only entry for an unlisted page", map[string]string{"blueprints/drafted.lock": barOnlyLock("docs/new.md")}, false,
			"blueprints/drafted.lock", "lock", "error", "records a blocked read, but the outline lists no such page"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := catalogProject(t, tt.extra)
			p.Required = tt.req
			got, err := p.Check(nil)
			if err != nil {
				t.Fatal(err)
			}
			hits := findingsFor(got, tt.check)
			if tt.check == "handler" && plan.HasError(got) {
				t.Errorf("a handler fixture has an error finding: %+v", got)
			}
			if tt.sev == "" {
				if len(hits) != 0 {
					t.Errorf("the passing twin has %s findings: %+v", tt.check, hits)
				}
				return
			}
			for _, f := range hits {
				if f.File == tt.file && f.Severity == tt.sev && strings.Contains(f.Message, tt.msg) {
					return
				}
			}
			t.Errorf("no %s %s finding in %s containing %q: %+v", tt.sev, tt.check, tt.file, tt.msg, hits)
		})
	}
}

// barOnlyLock is a lock whose one entry, for page, holds only a read bar.
func barOnlyLock(page string) string {
	return "version: 1\npages:\n  " + page + ":\n    blocked:\n      commit: 0123abcd\n      findings:\n        - location: rotate\n          text: A step is missing.\n"
}

// handlerAs returns the base blueprint with the failure's handler h-ask-admin recast as kind
// (which may carry a mode line), keeping its text.
func handlerAs(t *testing.T, kind string) string {
	t.Helper()
	return withEdit(t, "kind: handoff\n        text: |\n          Ask an admin", "kind: "+kind+"\n        text: |\n          Ask an admin")
}

// handlerLink returns the base blueprint with the failure's handler recast as a link with the
// given target and text lines.
func handlerLink(t *testing.T, target string) string {
	t.Helper()
	return withEdit(t, "kind: handoff\n        text: |\n          Ask an admin to grant the role, then try again.\n",
		"kind: link\n        "+target+"\n")
}

func TestCheckConvention(t *testing.T) {
	t.Run("blueprint root off the convention", func(t *testing.T) {
		p := catalogProject(t, nil)
		moved := p
		moved.BlueprintRoot, moved.Outline = "plans", "plans/outline.yaml"
		if err := os.Rename(p.abs("blueprints"), p.abs("plans")); err != nil {
			t.Fatal(err)
		}
		moved.FactsDir = moved.abs("plans/facts")
		got, err := moved.Check(nil)
		if err != nil {
			t.Fatal(err)
		}
		hits := findingsFor(got, "convention")
		if len(hits) != 1 || hits[0].Severity != plan.SeverityWarning || !strings.Contains(hits[0].Message, `blueprints.root is "plans"`) {
			t.Errorf("convention findings: %+v", hits)
		}
		if plan.HasError(got) {
			t.Errorf("a moved root alone is an error: %+v", got)
		}
	})
	t.Run("facts directory off the convention", func(t *testing.T) {
		p := catalogProject(t, map[string]string{"elsewhere/facts.md": readFixture(t, "facts/facts.md")})
		p.FactsDir = p.abs("elsewhere")
		got, err := p.Check(nil)
		if err != nil {
			t.Fatal(err)
		}
		hits := findingsFor(got, "convention")
		if len(hits) != 1 || hits[0].Severity != plan.SeverityWarning || !strings.Contains(hits[0].Message, "facts.dir is") {
			t.Errorf("convention findings: %+v", hits)
		}
	})
}

func TestCheckCannotRun(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]string
		named []string
		want  string
	}{
		{"no blueprint root", nil, nil, "no blueprint root"},
		{"no outline", map[string]string{"blueprints/outline.yaml": ""}, nil, "no outline at blueprints/outline.yaml"},
		{"malformed outline", map[string]string{"blueprints/outline.yaml": "version: 1\ntypes: [\n"}, nil, "outline blueprints/outline.yaml is malformed"},
		{"outline with a schema fault", map[string]string{"blueprints/outline.yaml": "version: 2\ntypes: []\npages: []\n"}, nil, "is malformed"},
		{"malformed lock", map[string]string{"blueprints/drafted.lock": "version: [\n"}, nil, "lock "},
		{"named file not a blueprint", nil, []string{"docs/guide.md"}, "is not a blueprint.yaml file"},
		{"named file off the path rule", nil, []string{"blueprints/.blueprint.yaml"}, "is not a blueprint.yaml file"},
		{"named file missing", nil, []string{"blueprints/docs/gone.blueprint.yaml"}, "blueprint blueprints/docs/gone.blueprint.yaml"},
		{"facts directory empty", map[string]string{"blueprints/facts/facts.md": ""}, nil, "holds no facts file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := catalogProject(t, tt.extra)
			if tt.name == "no blueprint root" {
				p.FactsDir = ""
				if err := os.RemoveAll(p.abs("blueprints")); err != nil {
					t.Fatal(err)
				}
			}
			_, err := p.Check(tt.named)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestCheckNamedFilesAndContinuesPastDecodeFailure(t *testing.T) {
	p := catalogProject(t, map[string]string{
		"blueprints/docs/bad.blueprint.yaml": "version: 1\nargument: [\n",
		"blueprints/outline.yaml":            readFixture(t, "outline.yaml") + "  - path: docs/bad.md\n    slug: bad\n    title: Bad\n    type: task guide\n    job: A job.\n",
	})
	all, err := p.Check(nil)
	if err != nil {
		t.Fatal(err)
	}
	var badFile, otherErrors bool
	for _, f := range all {
		if f.File == "blueprints/docs/bad.blueprint.yaml" && f.Severity == plan.SeverityError {
			badFile = true
		}
		if f.File == guideBlueprint && f.Severity == plan.SeverityError {
			otherErrors = true
		}
	}
	if !badFile || otherErrors {
		t.Errorf("badFile=%v otherErrors=%v: %+v", badFile, otherErrors, all)
	}
	named, err := p.Check([]string{guideBlueprint})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range named {
		if f.File != guideBlueprint {
			t.Errorf("a named run reported %s", f.File)
		}
	}
}

func TestCrossLinksNeedTheOutlineConstructor(t *testing.T) {
	outline := readFixture(t, "outline.yaml") +
		"  - path: docs/next.md\n    slug: next\n    title: Next\n    type: task guide\n    job: A job.\ncrossLinks:\n  - from: guide\n    to: next\n    why: The reader continues there.\n"
	p := catalogProject(t, map[string]string{"blueprints/outline.yaml": outline})
	got, err := p.Check(nil)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, f := range findingsFor(got, "links") {
		if f.File == guideBlueprint && f.Severity == plan.SeverityWarning && strings.Contains(f.Message, "no link item targets docs/next.md") {
			found = true
		}
	}
	if !found {
		t.Errorf("no crossLinks warning: %+v", got)
	}
}

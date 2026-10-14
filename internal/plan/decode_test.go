package plan

import (
	"os"
	"strings"
	"testing"
)

func readValid(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/valid.plan.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// mutate returns the valid plan with old replaced by repl, failing if old is absent so a
// stale row can't pass on an unchanged plan.
func mutate(t *testing.T, old, repl string) string {
	t.Helper()
	src := readValid(t)
	if !strings.Contains(src, old) {
		t.Fatalf("valid plan lacks %q", old)
	}
	return strings.Replace(src, old, repl, 1)
}

// sentenceText is the text line of the valid plan's t-verify sentence, and handoff the first
// line of its h-next item at line 70, where the rows insert items.
const (
	sentenceText = "        text: \"The health endpoint reports the new key's ID.\"\n"
	handoff      = "      - id: h-next\n"
)

func codeItem(source string) string {
	return "      - id: c-demo\n        kind: code\n        source: " + source + "\n        text: |\n          The key command.\n"
}

func figureItem(extra string) string {
	return "      - id: fig-1\n        kind: figure\n        text: |\n          A diagram.\n" + extra
}

func TestDecodeValid(t *testing.T) {
	p, fs := Decode("valid.plan.yaml", []byte(readValid(t)))
	if len(fs) != 0 {
		t.Fatalf("findings on the valid plan: %+v", fs)
	}
	if p.Version != 1 || p.Page.Title != "Rotate the signing key" || p.Page.Type != "task-guide" {
		t.Errorf("page decoded wrong: %+v", p.Page)
	}
	if got := len(p.Sections); got != 3 {
		t.Fatalf("sections = %d, want 3", got)
	}
	step := p.Sections[1].Items[0]
	if step.Kind != "step" || !step.CanFail || step.Requires[0] != "signing-key-exists" || step.Produces[0] != "new-key" {
		t.Errorf("step decoded wrong: %+v", step)
	}
	if step.Line != 35 || step.Lines["canFail"] != 41 {
		t.Errorf("step lines = %d, %d; want 35, 41", step.Line, step.Lines["canFail"])
	}
	if got := p.Sections[1].Items[2].Text; got != "A quota error means the account already holds the maximum number of keys.\n" {
		t.Errorf("block scalar text = %q", got)
	}
	if p.Dispositions[0].Reference != "../reference/keys.md#grace-period" || p.Dispositions[1].Disposition != "cut" {
		t.Errorf("dispositions decoded wrong: %+v", p.Dispositions)
	}
	if len(p.Constraints) != 1 || len(p.Friction) != 1 || len(p.Preconditions) != 1 {
		t.Errorf("lists decoded wrong: %+v %+v %+v", p.Constraints, p.Friction, p.Preconditions)
	}
}

// Each refusal row mutates the valid plan in one place, so the fixture fails for that reason
// alone, and the row asserts the single finding's line.
func TestDecodeRefusals(t *testing.T) {
	tests := []struct {
		name, old, repl string
		line            int
		contains        string
	}{
		{"duplicate key", "      - id: l-keys\n", "      - id: l-keys\n        id: other\n", 29, `duplicate key "id" (first at line 28)`},
		{"unknown top field", "orderReason: |", "audience: all\norderReason: |", 16, `unknown field "audience"`},
		{"unknown nested field", "  type: task-guide\n", "  type: task-guide\n  author: someone\n", 6, `page: unknown field "author"`},
		{"missing required", "orderReason: |\n  The new key goes live first, so a failed deploy leaves the old key working.\n", "", 1, `missing required field "orderReason"`},
		{"missing nested required", "  type: task-guide\n", "", 3, `page: missing required field "type"`},
		{"version is not an integer", "version: 1\n", "version: one\n", 1, "version: must be an integer"},
		{"canFail is not a boolean", "canFail: true", "canFail: sometimes", 41, "must be true or false"},
		{"list written as a string", "requires: [new-key]", "requires: new-key", 59, "must be a list"},
		{"prose field with no value", "argument: |\n  Rotating the signing key takes three steps, and the order matters: the new key must be\n  live before the old one is revoked.\n", "argument:\n", 6, "argument: must be a string"},
		{"prose field empty", "        text: |\n          Revoke the old key, as described in the revocation guide.\n", "        text: \"\"\n", 72, "is empty"},
		{"required list empty", "    items:\n      - id: f-key-ttl\n        kind: fact\n        fact: signing-key-ttl\n      - id: l-keys\n        kind: link\n        target: ../reference/keys.md#key-lifetime\n", "    items: []\n", 24, "must hold at least one entry"},
		{"unsupported version", "version: 1\n", "version: 2\n", 1, "unsupported version 2"},
		{"unknown kind", "kind: handoff", "kind: quote", 71, `unknown kind "quote"`},
		{"field of another kind", "        fact: signing-key-ttl\n", "        fact: signing-key-ttl\n        text: stray\n", 28, `field "text" does not belong to kind "fact"`},
		{"kind missing its field", "        target: ../reference/keys.md#key-lifetime\n", "", 28, `kind "link" needs the field "target"`},
		{"unknown mode", "mode: claim", "mode: quoted", 67, `unknown mode "quoted"`},
		{"id is not a token", "id: f-key-ttl", "id: F Key", 25, "is not a lowercase token"},
		{"unknown disposition", "disposition: cut", "disposition: hide", 81, `unknown disposition "hide"`},
		{"subordinated with no reference", "    reference: ../reference/keys.md#grace-period\n", "", 75, "needs a reference"},
		{"alias", "  - signing-key-exists\n", "  - &k signing-key-exists\n  - *k\n", 20, "aliases are not supported"},
		{"plain prose with a line comment", "        text: \"Deploy the service with the new key.\"\n", "        text: Deploy the service first # then the worker\n", 58, "carries a line comment"},
		{"plain multi-line prose with a line comment", "        text: \"Deploy the service with the new key.\"\n", "        text: Deploy the service\n          with the new key # then the worker\n", 58, "carries a line comment"},
		{"plain constraints item with a line comment", "  - |\n    Keep each step to one action.\n", "  - Keep each step to one action # then stop\n", 84, "carries a line comment"},
		{"absolute page path", "  path: docs/guides/rotate-the-signing-key.md\n", "  path: /docs/guides/rotate-the-signing-key.md\n", 3, "is not a path inside the repository"},
		{"page path climbing out", "  path: docs/guides/rotate-the-signing-key.md\n", "  path: ../guides/rotate-the-signing-key.md\n", 3, "is not a path inside the repository"},
		{"page path climbing out after a descent", "  path: docs/guides/rotate-the-signing-key.md\n", "  path: docs/../../x.md\n", 3, "is not a path inside the repository"},
		{"plain title with a line comment", "  title: Rotate the signing key\n", "  title: Rotate the signing key # draft\n", 4, "carries a line comment"},
		{"plain heading with a line comment", "  - heading: Create the new key\n", "  - heading: Create the new key # draft\n", 31, "carries a line comment"},
		{"level out of range", "  - heading: Create the new key\n", "  - heading: Create the new key\n    level: 4\n", 32, "level 4 is not a heading level"},
		{"level is not an integer", "  - heading: Create the new key\n", "  - heading: Create the new key\n    level: deep\n", 32, "level: must be an integer"},
		{"use on a link", "        target: ../reference/keys.md#key-lifetime\n", "        target: ../reference/keys.md#key-lifetime\n        use: stray\n", 31, `field "use" does not belong to kind "link"`},
		{"facts written as a string", sentenceText, sentenceText + "        facts: signing-key-ttl\n", 65, "facts: must be a list"},
		{"facts on a handoff", "        kind: handoff\n", "        kind: handoff\n        facts: [signing-key-ttl]\n", 72, `field "facts" does not belong to kind "handoff"`},
		{"cause on a step", "        canFail: true\n", "        canFail: true\n        cause: stray\n", 42, `field "cause" does not belong to kind "step"`},
		{"code with no text", handoff, "      - id: c-demo\n        kind: code\n        source: ../reference/keys.md\n" + handoff, 70, `kind "code" needs the field "text"`},
		{"introduction facts written as a string", "    Rotating the database credentials.\n", "    Rotating the database credentials.\n  facts: signing-key-ttl\n", 16, "introduction.facts: must be a list"},
		{"code facts written as a string", handoff, codeItem("../reference/keys.md") + "        facts: signing-key-ttl\n" + handoff, 75, "facts: must be a list"},
		{"step optional is not a boolean", "        canFail: true\n", "        canFail: true\n        optional: maybe\n", 42, "optional: must be true or false"},
		{"link text is not a string", "        target: ../reference/keys.md#key-lifetime\n", "        target: ../reference/keys.md#key-lifetime\n        text: [a, b]\n", 31, "text: must be a string"},
		{"section anchor is not a string", "  - heading: Create the new key\n", "  - heading: Create the new key\n    anchor: [a]\n", 32, "anchor: must be a string"},
		{"reason facts written as a string", "          A quota error means the account already holds the maximum number of keys.\n", "          A quota error means the account already holds the maximum number of keys.\n        facts: signing-key-ttl\n", 52, "facts: must be a list"},
		{"empty items above another level-2 section", "    items:\n      - id: s-create\n", "    items: []\n  - heading: Placeholder\n    opens: x\n    items:\n      - id: s-create\n", 34, "must hold at least one entry"},
		{"failure facts written as a string", "        handledBy: r-quota\n", "        handledBy: r-quota\n        facts: signing-key-ttl\n", 48, "facts: must be a list"},
		{"optional on a sentence", sentenceText, sentenceText + "        optional: true\n", 65, `field "optional" does not belong to kind "sentence"`},
		{"figure with an unknown field", handoff, figureItem("        caption: x\n") + handoff, 74, `unknown field "caption"`},
		{"figure with no text", handoff, "      - id: fig-1\n        kind: figure\n" + handoff, 70, `kind "figure" needs the field "text"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, fs := Decode("p.plan.yaml", []byte(mutate(t, tt.old, tt.repl)))
			if len(fs) != 1 {
				t.Fatalf("findings = %+v, want exactly one", fs)
			}
			f := fs[0]
			if f.Line != tt.line || !strings.Contains(f.Message, tt.contains) || f.Severity != SeverityError || f.Check != "decode" || f.File != "p.plan.yaml" || f.Fix == "" {
				t.Errorf("finding = %+v, want line %d containing %q", f, tt.line, tt.contains)
			}
		})
	}
}

// Quoted and block values hold a # that a plain value would lose, so each is the clean twin of
// the line-comment rows above.
func TestDecodeCleanTwins(t *testing.T) {
	tests := []struct{ name, old, repl string }{
		{"quoted prose with a hash", "        text: \"Deploy the service with the new key.\"\n", "        text: \"Deploy the service first # then the worker\"\n"},
		{"block prose with a hash", "        text: \"Deploy the service with the new key.\"\n", "        text: |\n          Deploy the service first # then the worker\n"},
		{"non-prose value with a comment", "        produces: [deployed]\n", "        produces: [deployed] # the final state\n"},
		{"canFail omitted", "        canFail: true\n", ""},
		{"page path with a harmless ..", "  path: docs/guides/rotate-the-signing-key.md\n", "  path: docs/other/../guides/rotate-the-signing-key.md\n"},
		{"quoted title with a hash", "  title: Rotate the signing key\n", "  title: \"Rotate the signing key # draft\"\n"},
		{"quoted heading with a hash", "  - heading: Create the new key\n", "  - heading: \"Create the new key # draft\"\n"},
		{"quoted constraints item with a hash", "  - |\n    Keep each step to one action.\n", "  - \"Keep each step to one action # then stop\"\n"},
		{"level 3", "  - heading: Create the new key\n", "  - heading: Create the new key\n    level: 3\n"},
		{"level 2 written out", "  - heading: Create the new key\n", "  - heading: Create the new key\n    level: 2\n"},
		{"failure with no from", "        from: s-create\n", ""},
		{"failure with facts", "        from: s-create\n", "        from: s-create\n        facts: [signing-key-ttl]\n"},
		{"failure with a cause", "        from: s-create\n", "        from: s-create\n        cause: |\n          The account is full.\n"},
		{"fact with a use", "        fact: signing-key-ttl\n", "        fact: signing-key-ttl\n        use: |\n          The key lives for 90 days.\n"},
		{"sentence with facts", sentenceText, sentenceText + "        facts: [signing-key-ttl]\n"},
		{"code with a symbol", handoff, codeItem("../reference/keys.md#Symbol") + handoff},
		{"figure with a source", handoff, figureItem("        source: assets/key.svg\n") + handoff},
		{"introduction with facts", "    Rotating the database credentials.\n", "    Rotating the database credentials.\n  facts: [signing-key-ttl]\n"},
		{"code with facts and no source", handoff, "      - id: c-demo\n        kind: code\n        text: |\n          The key command.\n        facts: [signing-key-ttl]\n" + handoff},
		{"code with a URL source", handoff, codeItem("https://example.com/keys.go") + handoff},
		{"step marked optional", "        canFail: true\n", "        canFail: true\n        optional: true\n"},
		{"link with text", "        target: ../reference/keys.md#key-lifetime\n", "        target: ../reference/keys.md#key-lifetime\n        text: the key lifetime\n"},
		{"section with an anchor", "  - heading: Create the new key\n", "  - heading: Create the new key\n    anchor: create-the-new-key\n"},
		{"reason with facts", "          A quota error means the account already holds the maximum number of keys.\n", "          A quota error means the account already holds the maximum number of keys.\n        facts: [signing-key-ttl]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, fs := Decode("p.plan.yaml", []byte(mutate(t, tt.old, tt.repl)))
			if len(fs) != 0 {
				t.Fatalf("findings = %+v", fs)
			}
			if tt.name == "block prose with a hash" || tt.name == "quoted prose with a hash" {
				if got := p.Sections[2].Items[0].Text; !strings.Contains(got, "first # then the worker") {
					t.Errorf("text = %q lost the comment-looking tail", got)
				}
			}
		})
	}
}

// An empty items list is a finding except in a level-2 section directly above a level-3 one.
func TestDecodeEmptyItems(t *testing.T) {
	src := readValid(t)
	empty := func(old, repl string) string {
		if !strings.Contains(src, old) {
			t.Fatalf("valid plan lacks %q", old)
		}
		return strings.Replace(src, old, repl, 1)
	}
	parent := empty("    items:\n      - id: s-create\n", "    items: []\n  - heading: Placeholder\n    level: 3\n    opens: x\n    items:\n      - id: s-create\n")
	last := src[:strings.Index(src, "    items:\n      - id: s-deploy")] + "    items: []\n" + src[strings.Index(src, "dispositions:"):]
	tests := []struct {
		name, src string
		clean     bool
	}{
		{"level-2 above a level-3 section", parent, true},
		{"last section", last, false},
		{"level-3 above a level-3 section", strings.Replace(parent, "  - heading: Create the new key\n", "  - heading: Create the new key\n    level: 3\n", 1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, fs := Decode("p.plan.yaml", []byte(tt.src))
			if tt.clean && len(fs) != 0 {
				t.Fatalf("findings = %+v", fs)
			}
			if !tt.clean && (len(fs) != 1 || !strings.Contains(fs[0].Message, "must hold at least one entry")) {
				t.Fatalf("findings = %+v, want one empty-items finding", fs)
			}
		})
	}
}

func TestDecodeNewFields(t *testing.T) {
	src := mutate(t, "    Rotating the database credentials.\n", "    Rotating the database credentials.\n  facts: [signing-key-ttl]\n")
	src = strings.Replace(src, "        canFail: true\n", "        canFail: true\n        optional: true\n", 1)
	src = strings.Replace(src, "        target: ../reference/keys.md#key-lifetime\n", "        target: ../reference/keys.md#key-lifetime\n        text: the key lifetime\n", 1)
	src = strings.Replace(src, "  - heading: Create the new key\n", "  - heading: Create the new key\n    anchor: create-the-new-key\n", 1)
	p, fs := Decode("p.plan.yaml", []byte(src))
	if len(fs) != 0 {
		t.Fatalf("findings = %+v", fs)
	}
	if got := p.Introduction.Facts; len(got) != 1 || got[0] != "signing-key-ttl" {
		t.Errorf("introduction facts = %v", got)
	}
	if !p.Sections[1].Items[0].Optional || p.Sections[2].Items[0].Optional {
		t.Errorf("optional decoded wrong: %v, %v", p.Sections[1].Items[0].Optional, p.Sections[2].Items[0].Optional)
	}
	if got := p.Sections[0].Items[1].Text; got != "the key lifetime" {
		t.Errorf("link text = %q", got)
	}
	if got := p.Sections[1].Anchor; got != "create-the-new-key" {
		t.Errorf("anchor = %q", got)
	}
}

func TestDecodeLevelDefault(t *testing.T) {
	p, fs := Decode("p.plan.yaml", []byte(mutate(t, "  - heading: Create the new key\n", "  - heading: Create the new key\n    level: 3\n")))
	if len(fs) != 0 {
		t.Fatalf("findings = %+v", fs)
	}
	if got := []int{p.Sections[0].Level, p.Sections[1].Level, p.Sections[2].Level}; got[0] != 2 || got[1] != 3 || got[2] != 2 {
		t.Errorf("levels = %v, want [2 3 2]", got)
	}
}

func TestDecodeDocumentShape(t *testing.T) {
	valid := readValid(t)
	tests := []struct {
		name, src string
		line      int
		contains  string
	}{
		{"empty file", "", 1, "no YAML document"},
		{"comment only", "# nothing here\n", 1, "no YAML document"},
		{"malformed", "version: 1\npage: [\n", 2, "malformed YAML"},
		{"second document", valid + "---\nversion: 1\n", 89, "second YAML document"},
		{"scalar document", "just words\n", 1, "must be a YAML mapping"},
		{"list document", "- a\n- b\n", 1, "must be a YAML mapping"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, fs := Decode("p.plan.yaml", []byte(tt.src))
			if p != nil {
				t.Errorf("plan = %+v, want nil", p)
			}
			if len(fs) != 1 || fs[0].Line != tt.line || !strings.Contains(fs[0].Message, tt.contains) {
				t.Errorf("findings = %+v, want one at line %d containing %q", fs, tt.line, tt.contains)
			}
		})
	}
}

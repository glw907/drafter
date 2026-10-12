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

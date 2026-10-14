package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func promptsData(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("testdata/prompts")
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func checkPrompts(t *testing.T, args ...string) result {
	t.Helper()
	return run(t, t.TempDir(), append([]string{"check-prompts"}, args...)...)
}

func TestCheckPromptsPasses(t *testing.T) {
	d := promptsData(t)
	r := checkPrompts(t, "--module", d+"/module.js", "--matrix", d+"/matrix.json", "--expect", d+"/expect.json")
	if r.code != 0 || r.stdout != "" || !strings.Contains(r.stderr, "3 cases, 0 failed") {
		t.Errorf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
}

func TestCheckPromptsMatrixAlone(t *testing.T) {
	d := promptsData(t)
	r := checkPrompts(t, "--module", d+"/module.js", "--matrix", d+"/matrix.json")
	if r.code != 0 {
		t.Errorf("exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestCheckPromptsExpectMismatch(t *testing.T) {
	d := promptsData(t)
	exp, err := os.ReadFile(d + "/expect.json")
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(exp), "Name the flag.", "Name the flags.", 1)
	r := checkPrompts(t, "--module", d+"/module.js", "--matrix", d+"/matrix.json",
		"--expect", writeTemp(t, "expect.json", changed))
	if r.code != 1 {
		t.Fatalf("exit %d, want 1; stderr %q", r.code, r.stderr)
	}
	if want := `draft round 2: expect: line 2: want "1. Name the flags.", got "1. Name the flag."`; !strings.Contains(r.stdout, want) {
		t.Errorf("stdout %q lacks %q", r.stdout, want)
	}
	if strings.Contains(r.stdout, "draft round 1") {
		t.Errorf("a passing case was reported: %q", r.stdout)
	}
}

func TestCheckPromptsExpectNames(t *testing.T) {
	d := promptsData(t)
	r := checkPrompts(t, "--module", d+"/module.js", "--matrix", d+"/matrix.json", "--expect",
		writeTemp(t, "expect.json", `{"draft round 1": "Draft docs/widget/install.md in docs/widget/README.md.", "stray": "x"}`))
	if r.code != 1 {
		t.Fatalf("exit %d, want 1", r.code)
	}
	for _, want := range []string{"draft round 2: expect: case missing from the expect file", "stray: expect: case missing from the matrix"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout %q lacks %q", r.stdout, want)
		}
	}
}

func TestCheckPromptsExpectFromStdin(t *testing.T) {
	d := promptsData(t)
	exp, err := os.ReadFile(d + "/expect.json")
	if err != nil {
		t.Fatal(err)
	}
	r := runWithStdin(t, t.TempDir(), string(exp), "check-prompts", "--module", d+"/module.js", "--matrix", d+"/matrix.json", "--expect", "-")
	if r.code != 0 {
		t.Errorf("exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestCheckPromptsDroppedFindings(t *testing.T) {
	d := promptsData(t)
	matrix, err := os.ReadFile(d + "/matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	// Point the round 2 case at the seat that drops its findings.
	lossy := strings.Replace(string(matrix), `"name": "draft round 2",
      "seat": "draft"`, `"name": "draft round 2",
      "seat": "lossy"`, 1)
	r := checkPrompts(t, "--module", d+"/module.js", "--matrix", writeTemp(t, "matrix.json", lossy))
	if r.code != 1 || !strings.Contains(r.stdout, "draft round 2: findings: prompt does not contain the findings text whole") {
		t.Errorf("exit %d, stdout %q", r.code, r.stdout)
	}
}

func TestCheckPromptsNarrowFindingDropped(t *testing.T) {
	d := promptsData(t)
	matrix, err := os.ReadFile(d + "/matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	lossy := strings.Replace(string(matrix), `"seat": "editor"`, `"seat": "lossy"`, 1)
	r := checkPrompts(t, "--module", d+"/module.js", "--matrix", writeTemp(t, "matrix.json", lossy))
	if r.code != 1 || !strings.Contains(r.stdout, "editor narrowed: findings: prompt does not contain narrow.own finding") {
		t.Errorf("exit %d, stdout %q", r.code, r.stdout)
	}
}

func TestCheckPromptsCannotRun(t *testing.T) {
	d := promptsData(t)
	module, matrix := d+"/module.js", d+"/matrix.json"
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing module", []string{"--module", d + "/nope.js", "--matrix", matrix}, "read module"},
		{"missing matrix", []string{"--module", module, "--matrix", d + "/nope.json"}, "read matrix"},
		{"no matrix flag", []string{"--module", module}, "needs --matrix"},
		{"malformed matrix", []string{"--module", module, "--matrix", writeTemp(t, "m.json", "{not json")}, "matrix"},
		{"empty cases", []string{"--module", module, "--matrix", writeTemp(t, "m.json", `{"host":{"args":{},"resolved":{}},"cases":[]}`)}, "has no cases"},
		{"malformed expect", []string{"--module", module, "--matrix", matrix, "--expect", writeTemp(t, "e.json", "[1]")}, "expect file"},
		{"missing expect", []string{"--module", module, "--matrix", matrix, "--expect", d + "/nope.json"}, "read expect file"},
		{"module that cannot load", []string{"--module", writeTemp(t, "bad.js", "const x = ;"), "--matrix", matrix}, "bad.js"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := checkPrompts(t, tt.args...)
			if r.code != 2 || !strings.Contains(r.stderr, tt.want) {
				t.Errorf("exit %d, stderr %q, want exit 2 naming %q", r.code, r.stderr, tt.want)
			}
		})
	}
}

func TestCheckPromptsJSON(t *testing.T) {
	d := promptsData(t)
	exp, err := os.ReadFile(d + "/expect.json")
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(exp), "Name the flag.", "Name the flags.", 1)
	r := checkPrompts(t, "--module", d+"/module.js", "--matrix", d+"/matrix.json",
		"--expect", writeTemp(t, "expect.json", changed), "--json")
	if r.code != 1 {
		t.Fatalf("exit %d, want 1", r.code)
	}
	var got []struct {
		Case     string
		Seat     string
		Pass     bool
		Failures []struct{ Check, Message string }
	}
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
		t.Fatalf("decode %q: %v", r.stdout, err)
	}
	if len(got) != 3 || !got[0].Pass || !got[2].Pass || got[1].Pass {
		t.Fatalf("results %+v", got)
	}
	if f := got[1].Failures; len(f) != 1 || f[0].Check != "expect" || !strings.HasPrefix(f[0].Message, "line 2:") {
		t.Errorf("failures %+v", f)
	}
}

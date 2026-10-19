package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stockModuleEnv is a page project whose drafter.toml names no prompts module.
func stockModuleEnv(t *testing.T, stub *apiStub) pageEnv {
	t.Helper()
	p := newPageEnv(t, stub)
	path := filepath.Join(p.dir, "drafter.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, strings.Replace(string(data), "prompts = \"prompts.js\"\n", "", 1))
	gitRun(t, p.dir, "add", "drafter.toml")
	gitRun(t, p.dir, "commit", "-q", "-m", "use the stock module")
	return p
}

func TestPageRefusesTheStockModuleBeforeSpend(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
	}{
		{"a real run", nil},
		{"a dry run", []string{"--dry-run"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stub := newAPIStub(t)
			p := stockModuleEnv(t, stub)
			r := p.page(t, append(tt.args, p.handoffFile(t))...)
			if r.code != 2 {
				t.Fatalf("exit %d, want 2\nstdout %s\nstderr %s", r.code, r.stdout, r.stderr)
			}
			for _, want := range []string{
				"no seat draft", "the stock prompts module carries only the blueprint read", guidePage,
			} {
				if !strings.Contains(r.stderr, want) {
					t.Errorf("stderr lacks %q:\n%s", want, r.stderr)
				}
			}
			if got := stub.submitted(); len(got) != 0 {
				t.Errorf("the refusal came after a submission: %v", got)
			}
		})
	}
}

func TestPageReportNamesTheModule(t *testing.T) {
	stub := newAPIStub(t)
	p := newPageEnv(t, stub)
	src, err := os.ReadFile(filepath.Join(p.dir, "prompts.js"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(src)
	want := "prompts: " + filepath.Join(p.dir, "prompts.js") + " sha256:" + hex.EncodeToString(sum[:])[:12]
	r := p.page(t, "--dry-run", p.handoffFile(t))
	if r.code != 0 {
		t.Fatalf("exit %d\nstdout %s\nstderr %s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, want+"\n") {
		t.Errorf("stdout lacks %q:\n%s", want, r.stdout)
	}
}

func TestPageRequirePromptsRefusesFirst(t *testing.T) {
	stub := newAPIStub(t)
	p := stockModuleEnv(t, stub)
	path := filepath.Join(p.dir, "drafter.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "require_prompts = true\n"+string(data))
	gitRun(t, p.dir, "add", "drafter.toml")
	gitRun(t, p.dir, "commit", "-q", "-m", "require prompts")
	r := p.page(t, "--dry-run", p.handoffFile(t))
	if r.code != 2 || !strings.Contains(r.stderr, "prompts is not set, and require_prompts in drafter.toml refuses the stock module") {
		t.Errorf("exit %d, stderr %q, want 2 with the require_prompts sentence", r.code, r.stderr)
	}
	if got := stub.paths; len(got) != 0 {
		t.Errorf("the API saw %v before the refusal", got)
	}
}

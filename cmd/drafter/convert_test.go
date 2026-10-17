package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/spf13/cobra"
)

const fixtures = "../../internal/blueprint/testdata/convert/"

// runConvert runs the convert command under a root of its own, as main will mount it.
func runConvert(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := &cobra.Command{Use: "drafter", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(newConvertCmd())
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(append([]string{"convert"}, args...))
	err = root.Execute()
	return out.String(), errb.String(), err
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func copyFixture(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(fixtures + name)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, name)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestConvertOutlineCommand(t *testing.T) {
	a, b := fixtures+"outline-a.json", fixtures+"outline-b.json"

	stdout, _, err := runConvert(t, "outline", a, b)
	if err != nil {
		t.Fatal(err)
	}
	o, findings := blueprint.DecodeOutline("stdout", []byte(stdout))
	if len(findings) > 0 || len(o.Pages) != 2 || len(o.Types) != 2 {
		t.Fatalf("findings %+v, outline %+v\n%s", findings, o, stdout)
	}

	file := filepath.Join(t.TempDir(), "outline.yaml")
	stdout, _, err = runConvert(t, "outline", "--out", file, a, b)
	if err != nil || stdout != "" {
		t.Fatalf("--out: stdout %q, err %v", stdout, err)
	}
	if got, _ := os.ReadFile(file); string(got) == "" {
		t.Error("--out wrote nothing")
	}
	if _, _, err := runConvert(t, "outline", "--out", file, a); err == nil {
		t.Error("--out overwrote an existing file")
	}
	if _, _, err := runConvert(t, "outline"); err == nil {
		t.Error("no input file was accepted")
	}
}

func TestConvertPlanCommand(t *testing.T) {
	root := t.TempDir()
	gitIn(t, root, "init", "-q")
	page := filepath.Join(root, "docs", "guides", "rotate-the-widget-key.md")
	if err := os.MkdirAll(filepath.Dir(page), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, []byte("## Before you begin\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-q", "-m", "page")

	work := t.TempDir()
	planFile := copyFixture(t, work, "widget.plan.yaml")
	framing := copyFixture(t, work, "widget.framing.md")
	outline := copyFixture(t, work, "widget.outline.json")

	stdout, stderr, err := runConvert(t, "plan", "--root", root, "--framing", framing, "--outline", outline, planFile)
	if err != nil {
		t.Fatal(err)
	}
	const bp = "blueprints/docs/guides/rotate-the-widget-key.blueprint.yaml"
	if stdout != bp+"\n" {
		t.Errorf("stdout = %q, want the blueprint path alone", stdout)
	}
	for _, want := range []string{
		"seeded the drafted.lock entry",
		"friction (dropped from the blueprint",
		"The plan check reports the anchor twice.",
		"departures (dropped from the blueprint; put in the conversion's commit message)",
		`The planning session dropped the "why rotate" paragraph`,
		"needs judgment:",
		`section "Verify the key" (verify-the-key): write its why`,
		`item "f-renew": its use does not start with Only or Omit`,
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(bp)))
	if err != nil {
		t.Fatal(err)
	}
	if _, findings := blueprint.Decode(bp, data); len(findings) > 0 {
		t.Errorf("findings: %+v", findings)
	}
	if strings.Contains(string(data), "why rotate") || strings.Contains(string(data), "plan check reports") {
		t.Error("friction or departures text reached the blueprint")
	}
	lock, err := blueprint.ReadLock(filepath.Join(root, "blueprints", "drafted.lock"))
	if err != nil || len(lock.Pages) != 1 {
		t.Fatalf("lock = %+v, %v", lock, err)
	}

	if _, _, err := runConvert(t, "plan", "--root", root, "--framing", framing, "--outline", outline, planFile); err == nil {
		t.Error("a second conversion was accepted")
	}
}

func TestConvertPlanCommandRefusals(t *testing.T) {
	work := t.TempDir()
	planFile := copyFixture(t, work, "widget.plan.yaml")
	outline := copyFixture(t, work, "widget.outline.json")
	other := copyFixture(t, work, "outline-a.json")
	bad := filepath.Join(work, "bad.framing.md")
	if err := os.WriteFile(bad, []byte("## Surprise\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown framing heading", []string{"--framing", bad, "--outline", outline}, `"Surprise"`},
		{"page missing from the outline", []string{"--outline", other}, "does not list the page"},
		{"no outline", nil, "needs --outline"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			args := append([]string{"plan", "--root", root}, tt.args...)
			stdout, stderr, err := runConvert(t, append(args, planFile)...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want one containing %q", err, tt.want)
			}
			if strings.Contains(stdout+stderr, "Departures") {
				t.Errorf("departures leaked: %q %q", stdout, stderr)
			}
			if _, statErr := os.Stat(filepath.Join(root, "blueprints")); statErr == nil {
				t.Error("a refusal left a blueprint tree behind")
			}
		})
	}
}

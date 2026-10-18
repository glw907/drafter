package main

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/gate"
	"github.com/spf13/cobra"
)

const (
	gateVale = "StylesPath = .vale/styles\nPackages = Google\n\n[*.md]\nBasedOnStyles = Vale, Google\n"
	gateRule = "extends: existence\nmessage: \"Avoid '%s'.\"\nlevel: error\ntokens:\n  - foo\n"
)

// gateProject is project with a Vale setup: a .vale.ini and a synthetic local style named Google,
// so no package is ever fetched. It fails when Vale is not installed, since the gate cannot run
// without it.
func gateProject(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("vale"); err != nil {
		t.Fatal("Vale is required to run these tests; install it (https://vale.sh/docs/install)")
	}
	tree := map[string]string{
		".vale.ini":                   gateVale,
		".vale/styles/Google/Foo.yml": gateRule,
	}
	maps.Copy(tree, files)
	return project(t, tree)
}

// gateRun executes drafter gate in-process and returns stdout, stderr, and the exit status.
func gateRun(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	root := &cobra.Command{Use: "drafter", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(newGateCmd())
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(append([]string{"gate"}, args...))
	err := root.Execute()
	if err == nil {
		return out.String(), errb.String(), 0
	}
	ee, ok := errors.AsType[exitError](err)
	if !ok {
		t.Fatalf("error %v is not an exitError", err)
	}
	return out.String(), errb.String() + ee.msg + "\n", ee.code
}

func gatePageArg(root, page string) []string {
	return []string{"--root", root, "--page", filepath.Join(root, page)}
}

func TestGateCleanPageExitsZero(t *testing.T) {
	root := gateProject(t, nil)
	out, errs, code := gateRun(t, gatePageArg(root, "docs/guide.md")...)
	if code != 0 || out != "" || errs != "0 errors, 0 warnings\n" {
		t.Errorf("exit %d, stdout %q, stderr %q; want a silent pass", code, out, errs)
	}
}

func TestGateFindings(t *testing.T) {
	const brokenFact = "- `f:bbbbbb` Rotating a key needs the admin role. Source: `src/rotate.go#Missing`. [verified]\n"
	const brokenOther = "- `f:eeeeee` A deleted key stops signing at once. Source: `src/keys.go#Missing`. [verified]\n"
	catalog := readCatalog(t, "facts/facts.md")
	withFact := func(id, line string) string {
		var b strings.Builder
		for l := range strings.SplitSeq(catalog, "\n") {
			if strings.HasPrefix(l, "- `"+id+"`") {
				l = strings.TrimSuffix(line, "\n")
			}
			b.WriteString(l + "\n")
		}
		return strings.TrimSuffix(b.String(), "\n")
	}
	tests := []struct {
		name     string
		files    map[string]string
		wantCode int
		wantOut  []string // fragments of stdout
	}{
		{"a Vale error", map[string]string{"docs/guide.md": "# Rotate the widget key\n\nA foo line.\n"}, 1, []string{"docs/guide.md:3:3:Google.Foo:"}},
		{"a link to a missing file", map[string]string{"docs/guide.md": "# Rotate the widget key\n\n[x](gone.md)\n"}, 1, []string{"docs/guide.md:3: error: links: target docs/gone.md does not exist"}},
		{"an anchor matching no heading", map[string]string{"docs/guide.md": "# Rotate the widget key\n\n[x](#nope)\n"}, 1, []string{"docs/guide.md:3: error: links: anchor #nope"}},
		{"an https link is not checked", map[string]string{"docs/guide.md": "# Rotate the widget key\n\n[x](https://example.com/gone.md#nope)\n"}, 0, nil},
		{"a cited fact with a broken anchor", map[string]string{"blueprints/facts/facts.md": withFact("f:bbbbbb", brokenFact)}, 1, []string{"blueprints/facts/facts.md:4: error: store:"}},
		{"a broken fact the page does not cite", map[string]string{"blueprints/facts/facts.md": withFact("f:eeeeee", brokenOther)}, 0, nil},
		{"a blueprint error", map[string]string{"blueprints/docs/guide.blueprint.yaml": strings.Replace(readCatalog(t, "base.blueprint.yaml"), "why: |\n      The reader needs the admin role before the first step.\n", "why: \"\"\n", 1)}, 1, []string{"error: decode: sections[0].why"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := gateProject(t, tt.files)
			out, errs, code := gateRun(t, gatePageArg(root, "docs/guide.md")...)
			if code != tt.wantCode {
				t.Errorf("exit %d, want %d\nstdout %s\nstderr %s", code, tt.wantCode, out, errs)
			}
			for _, frag := range tt.wantOut {
				if !strings.Contains(out, frag) {
					t.Errorf("stdout %q lacks %q", out, frag)
				}
			}
		})
	}
}

func TestGatePageWithoutABlueprintSkipsThoseChecks(t *testing.T) {
	root := gateProject(t, map[string]string{"docs/loose.md": "# A loose page\n"})
	out, _, code := gateRun(t, gatePageArg(root, "docs/loose.md")...)
	want := "no blueprint for docs/loose.md; blueprint and fact checks skipped\n"
	if code != 0 || out != want {
		t.Errorf("exit %d, stdout %q, want 0 and %q", code, out, want)
	}
}

func TestGateCannotRun(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		page    string
		wantMsg string
	}{
		{"a missing page", nil, "docs/none.md", "no page at docs/none.md"},
		{"a missing style directory", map[string]string{".vale/styles/Google/Foo.yml": ""}, "docs/guide.md", "vale"},
		{"a rule Vale cannot load", map[string]string{".vale/styles/Google/Bad.yml": "extends: nonsense\nmessage: x\nlevel: error\n"}, "docs/guide.md", "vale exited 2"},
		{"a page outside the project", nil, "../outside.md", "outside the project root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := gateProject(t, tt.files)
			_, errs, code := gateRun(t, "--root", root, "--page", filepath.Join(root, tt.page))
			if code != exitCannotRun || !strings.Contains(errs, tt.wantMsg) {
				t.Errorf("exit %d, stderr %q; want %d naming %q", code, errs, exitCannotRun, tt.wantMsg)
			}
		})
	}
}

func TestGateWithoutAViableValeFailsFirst(t *testing.T) {
	root := project(t, nil)
	_, errs, code := gateRun(t, gatePageArg(root, "docs/none.md")...)
	if code != exitCannotRun || !strings.Contains(errs, "vale") && !strings.Contains(errs, "Vale") {
		t.Errorf("exit %d, stderr %q; want the Vale preflight to fail before the missing page does", code, errs)
	}
	if strings.Contains(errs, "no page") {
		t.Errorf("stderr %q reached the page check before the preflight", errs)
	}
}

func TestGateRequiresAPage(t *testing.T) {
	_, errs, code := gateRun(t)
	if code != exitCannotRun || !strings.Contains(errs, "--page") {
		t.Errorf("exit %d, stderr %q; want a usage error naming --page", code, errs)
	}
}

// gateReplay runs the captured gate output through gate.Run as a plain command, as the pipeline
// does, and returns the parsed result.
func gateReplay(t *testing.T, captured string) gate.Result {
	t.Helper()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "captured.txt")
	if err := os.WriteFile(logFile, []byte(captured), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := gate.Run(context.Background(), config.Gate{Protocol: "plain"}, []string{"PATH=/usr/bin:/bin"},
		dir, "cat captured.txt; exit 1", filepath.Join(dir, "gate.log"))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestGateStockPreflight(t *testing.T) {
	root := gateProject(t, nil)
	if err := stockGatePreflight("make gate", t.TempDir()); err != nil {
		t.Errorf("a project's own gate command was checked: %v", err)
	}
	valePath, err := exec.LookPath("vale")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.Symlink(valePath, filepath.Join(bin, "vale")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	err = stockGatePreflight("drafter gate --page {page}", root)
	if err == nil || !strings.Contains(err.Error(), "drafter is not on PATH") {
		t.Errorf("preflight with drafter off PATH = %v, want an error naming drafter", err)
	}
	if err := os.Symlink(valePath, filepath.Join(bin, "drafter")); err != nil {
		t.Fatal(err)
	}
	if err := stockGatePreflight("drafter gate --page {page}", root); err != nil {
		t.Errorf("preflight with drafter on PATH = %v, want nil", err)
	}
}

func TestGateRedOutputReplaysThroughTheGateRunner(t *testing.T) {
	const page, brief = "docs/guide.md", "blueprints/docs/guide.blueprint.yaml"
	t.Run("a Vale line is the page's", func(t *testing.T) {
		root := gateProject(t, map[string]string{"docs/guide.md": "# Rotate the widget key\n\nA foo line.\n"})
		out, errs, code := gateRun(t, "--root", root, "--page", filepath.Join(root, page))
		if code != 1 {
			t.Fatalf("exit %d, want 1", code)
		}
		res := gateReplay(t, out+errs)
		own, foreign := res.ForPage(page, brief)
		if len(own) == 0 || foreign {
			t.Errorf("ForPage = %v, foreign %v; want the page's own red", own, foreign)
		}
	})
	t.Run("store lines alone are foreign", func(t *testing.T) {
		facts := strings.Replace(readCatalog(t, "facts/facts.md"), "Source: `src/rotate.go`.", "Source: `src/rotate.go#Missing`.", 1)
		root := gateProject(t, map[string]string{"blueprints/facts/facts.md": facts})
		out, errs, code := gateRun(t, "--root", root, "--page", filepath.Join(root, page))
		if code != 1 || !strings.Contains(out, "store:") {
			t.Fatalf("exit %d, stdout %q; want a store finding", code, out)
		}
		own, foreign := gateReplay(t, out+errs).ForPage(page, brief)
		if len(own) != 0 || !foreign {
			t.Errorf("ForPage = %v, foreign %v; want a foreign red", own, foreign)
		}
	})
}

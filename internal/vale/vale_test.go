package vale

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireVale fails the test when Vale is not installed. A suite that skipped here would pass on a
// machine that cannot run the gate.
func requireVale(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("vale"); err != nil {
		t.Fatal("Vale is required to run these tests; install it (https://vale.sh/docs/install)")
	}
}

// isolateVale points HOME and XDG_CONFIG_HOME at temp directories, so a Vale config or style
// directory on the machine cannot satisfy a test, and returns the XDG directory.
func isolateVale(t *testing.T) string {
	t.Helper()
	requireVale(t)
	xdg := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", xdg)
	return xdg
}

func writeValeFile(t *testing.T, name, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const valeIni = "StylesPath = .vale/styles\nPackages = Google\n\n[*.md]\nBasedOnStyles = Vale, Google\n"

const valeRule = "extends: existence\nmessage: \"Avoid '%s'.\"\nlevel: error\ntokens:\n  - foo\n"

// valeProject writes a complete Vale setup into a new directory and returns it.
func valeProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeValeFile(t, filepath.Join(root, ".vale.ini"), valeIni)
	writeValeFile(t, filepath.Join(root, ".vale", "styles", "Google", "Foo.yml"), valeRule)
	return root
}

func TestPreflight(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, xdg string) string // returns the project root
		want  string                                // empty means the preflight passes
	}{
		{"complete setup", func(t *testing.T, xdg string) string { return valeProject(t) }, ""},
		{"complete setup through a symlinked root", func(t *testing.T, xdg string) string {
			link := filepath.Join(t.TempDir(), "link")
			if err := os.Symlink(valeProject(t), link); err != nil {
				t.Fatal(err)
			}
			return link
		}, ""},
		{"no vale on PATH", func(t *testing.T, xdg string) string {
			t.Setenv("PATH", t.TempDir())
			return valeProject(t)
		}, "vale is not on PATH"},
		{"no .vale.ini anywhere", func(t *testing.T, xdg string) string { return t.TempDir() }, "vale ls-config failed"},
		{"no .vale.ini but a global one", func(t *testing.T, xdg string) string {
			writeValeFile(t, filepath.Join(xdg, "vale", ".vale.ini"), "[*.md]\nBasedOnStyles = Vale\n")
			return t.TempDir()
		}, "no .vale.ini at"},
		{"a .vale.ini in a parent directory only", func(t *testing.T, xdg string) string {
			parent := valeProject(t)
			child := filepath.Join(parent, "sub")
			if err := os.Mkdir(child, 0o755); err != nil {
				t.Fatal(err)
			}
			return child
		}, "no .vale.ini at"},
		{"a style with no directory", func(t *testing.T, xdg string) string {
			root := t.TempDir()
			writeValeFile(t, filepath.Join(root, ".vale.ini"), valeIni)
			if err := os.MkdirAll(filepath.Join(root, ".vale", "styles"), 0o755); err != nil {
				t.Fatal(err)
			}
			return root
		}, "style Google has no directory"},
		{"a style with no styles path at all", func(t *testing.T, xdg string) string {
			root := t.TempDir()
			writeValeFile(t, filepath.Join(root, ".vale.ini"), valeIni)
			return root
		}, "run vale sync"},
		{"a style present only globally", func(t *testing.T, xdg string) string {
			root := t.TempDir()
			writeValeFile(t, filepath.Join(root, ".vale.ini"), valeIni)
			if err := os.MkdirAll(filepath.Join(root, ".vale", "styles"), 0o755); err != nil {
				t.Fatal(err)
			}
			writeValeFile(t, filepath.Join(xdg, "vale", "styles", "Google", "Foo.yml"), valeRule)
			return root
		}, "style Google has no directory"},
		{"a style in a global base list only", func(t *testing.T, xdg string) string {
			root := t.TempDir()
			writeValeFile(t, filepath.Join(root, ".vale.ini"), "StylesPath = .vale/styles\nBasedOnStyles = Extra\n\n[*.md]\nBasedOnStyles = Vale\n")
			if err := os.MkdirAll(filepath.Join(root, ".vale", "styles"), 0o755); err != nil {
				t.Fatal(err)
			}
			return root
		}, "run vale sync"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xdg := isolateVale(t)
			root := tt.setup(t, xdg)
			err := Preflight(root)
			switch {
			case tt.want == "" && err != nil:
				t.Errorf("Preflight = %v, want nil", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Errorf("Preflight = %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

func TestPreflightMessagesNameTheFix(t *testing.T) {
	xdg := isolateVale(t)
	writeValeFile(t, filepath.Join(xdg, "vale", ".vale.ini"), "[*.md]\nBasedOnStyles = Vale\n")
	if err := Preflight(t.TempDir()); err == nil || !strings.Contains(err.Error(), "drafter init") {
		t.Errorf("a missing .vale.ini error = %v, want it to say to run drafter init", err)
	}
	root := t.TempDir()
	writeValeFile(t, filepath.Join(root, ".vale.ini"), valeIni)
	if err := os.MkdirAll(filepath.Join(root, ".vale", "styles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Preflight(root); err == nil || !strings.Contains(err.Error(), "vale sync") {
		t.Errorf("a missing style error = %v, want it to say to run vale sync", err)
	}
	t.Setenv("PATH", t.TempDir())
	if err := Preflight(root); err == nil || !strings.Contains(err.Error(), "install Vale") {
		t.Errorf("a missing binary error = %v, want it to say to install Vale", err)
	}
}

func TestPreflightRefusesOutputThatIsNotJSON(t *testing.T) {
	isolateVale(t)
	bin := t.TempDir()
	writeValeFile(t, filepath.Join(bin, "vale"), "#!/bin/sh\necho not json\n")
	if err := os.Chmod(filepath.Join(bin, "vale"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if err := Preflight(t.TempDir()); err == nil || !strings.Contains(err.Error(), "not JSON") {
		t.Errorf("Preflight = %v, want an error saying the output is not JSON", err)
	}
}

func TestLint(t *testing.T) {
	tests := []struct {
		name     string
		page     string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{"clean page", "A clean line.\n", 0, "", ""},
		{"error-level alert", "A foo line.\n", 1, "p.md:1:3:Google.Foo:", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateVale(t)
			root := valeProject(t)
			writeValeFile(t, filepath.Join(root, "p.md"), tt.page)
			var out bytes.Buffer
			code, err := Lint(root, "p.md", &out)
			if err != nil || code != tt.wantCode || !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("Lint = %d, %v, out %q; want code %d containing %q", code, err, out.String(), tt.wantCode, tt.wantOut)
			}
		})
	}
	t.Run("a rule Vale cannot load", func(t *testing.T) {
		isolateVale(t)
		root := valeProject(t)
		writeValeFile(t, filepath.Join(root, ".vale", "styles", "Google", "Bad.yml"), "extends: nonsense\nmessage: x\nlevel: error\n")
		writeValeFile(t, filepath.Join(root, "p.md"), "A line.\n")
		code, err := Lint(root, "p.md", &bytes.Buffer{})
		if code != 2 || err == nil || !strings.Contains(err.Error(), "vale exited 2") {
			t.Errorf("Lint = %d, %v; want exit 2 with an error quoting Vale", code, err)
		}
	})
}

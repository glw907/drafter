package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
	"github.com/spf13/cobra"
)

// catalogDir holds the synthetic project the blueprint package tests use.
const catalogDir = "../../internal/blueprint/testdata/catalog"

// blueprintRoot builds a cobra root that holds the two blueprint commands, since root.go
// registers them later.
func blueprintRoot() *cobra.Command {
	root := &cobra.Command{Use: "drafter", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(newCheckCmd(), newRefsCmd(), newExcerptCmd())
	return root
}

// run executes the command line and returns stdout, stderr, and the exit status the real main
// would use.
func run(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	root := blueprintRoot()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return out.String(), errb.String(), 0
	}
	ee, ok := errors.AsType[exitError](err)
	if !ok {
		t.Fatalf("error %v is not an exitError", err)
	}
	msg := errb.String()
	if ee.msg != "" {
		msg += ee.msg + "\n"
	}
	return out.String(), msg, ee.code
}

// project builds the catalog fixtures into a temp project. files add to or override the tree.
func project(t *testing.T, files map[string]string) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(catalogDir, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	tree := map[string]string{
		"blueprints/docs/guide.blueprint.yaml": read("base.blueprint.yaml"),
		"blueprints/outline.yaml":              read("outline.yaml"),
		"blueprints/facts/facts.md":            read("facts/facts.md"),
		"docs/guide.md":                        "# Rotate the widget key\n",
		"src/rotate.go":                        "package src\n\nfunc Rotate() {}\n",
		"src/sign.go":                          "package src\n\nfunc Sign() {}\n",
		"src/keys.go":                          "package src\n\nconst KeyBytes = 32\n",
	}
	maps.Copy(tree, files)
	for name, content := range tree {
		if content == "" {
			continue
		}
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestCheckPassesTheCatalog(t *testing.T) {
	root := project(t, nil)
	out, errs, code := run(t, "check", "--root", root)
	if code != 0 || out != "" || errs != "facts store: 4 pointers checked, 0 skipped\n0 errors, 0 warnings\n" {
		t.Errorf("code %d, stdout %q, stderr %q", code, out, errs)
	}
}

// verifyStoreRotateSource is the catalog's one rotate.go pointer, which the store rows rewrite.
const verifyStoreRotateSource = "Source: `src/rotate.go`. [verified]"

func TestCheckVerifiesTheStore(t *testing.T) {
	farRotate := "package src\n" + strings.Repeat("// padding\n", 26) + "var farToken = 1\n"
	dupe := "- `f:bbbbbb` A second bullet with the same id. Source: `src/sign.go`. [verified]\n"
	tests := []struct {
		name    string
		replace string // replaces the rotate.go pointer in the facts file
		append  string // added to the end of the facts file
		rotate  string // replaces src/rotate.go
		want    string
	}{
		{"a moved line", "Source: `src/rotate.go:3` (`farToken`). [verified]", "", farRotate, `names ["farToken"], not found within 10 lines`},
		{"a missing anchor", "Source: `src/rotate.go:3` (`nowhereToken`). [verified]", "", "", `names ["nowhereToken"]`},
		{"a duplicate id", "", dupe, "", `fact "f:bbbbbb" is on 2 bullets (blueprints/facts/facts.md:`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := readCatalog(t, "facts/facts.md")
			if tt.replace != "" {
				facts = strings.Replace(facts, verifyStoreRotateSource, tt.replace, 1)
			}
			files := map[string]string{"blueprints/facts/facts.md": facts + tt.append}
			if tt.rotate != "" {
				files["src/rotate.go"] = tt.rotate
			}
			root := project(t, files)
			out, errs, code := run(t, "check", "--root", root)
			if code != 1 || !strings.Contains(out, ": error: store: ") || !strings.Contains(out, tt.want) {
				t.Errorf("code %d, stdout %q, want exit 1 and a store error containing %q", code, out, tt.want)
			}
			if !strings.HasPrefix(errs, "facts store: ") {
				t.Errorf("stderr %q", errs)
			}
		})
	}
}

func TestCheckVerifiesTheStoreOnlyWithoutNamedBlueprints(t *testing.T) {
	broken := strings.Replace(readCatalog(t, "facts/facts.md"), verifyStoreRotateSource, "Source: `src/gone.go:3`. [verified]", 1)
	root := project(t, map[string]string{"blueprints/facts/facts.md": broken})
	file := filepath.Join(root, "blueprints", "docs", "guide.blueprint.yaml")
	out, errs, code := run(t, "check", "--root", root, file)
	if code != 0 || out != "" || errs != "0 errors, 0 warnings\n" {
		t.Errorf("named blueprint: code %d, stdout %q, stderr %q", code, out, errs)
	}
	if _, _, code := run(t, "check", "--root", root); code != 1 {
		t.Errorf("whole store: code %d, want 1", code)
	}
}

func TestCheckReportsFindings(t *testing.T) {
	bad := strings.Replace(readCatalog(t, "base.blueprint.yaml"), "fact: f:bbbbbb", "fact: f:zzzzzz", 1)
	root := project(t, map[string]string{"blueprints/docs/guide.blueprint.yaml": bad})
	out, errs, code := run(t, "check", "--root", root)
	if code != 1 {
		t.Errorf("code %d, want 1", code)
	}
	if !strings.HasPrefix(out, "blueprints/docs/guide.blueprint.yaml:") || !strings.Contains(out, ": error: facts: fact \"f:zzzzzz\" is in no facts file (") {
		t.Errorf("stdout %q", out)
	}
	if errs != "facts store: 4 pointers checked, 0 skipped\n1 error, 0 warnings\n" {
		t.Errorf("stderr %q", errs)
	}

	jsonOut, _, code := run(t, "check", "--root", root, "--json")
	var got []plan.Finding
	if err := json.Unmarshal([]byte(jsonOut), &got); err != nil || code != 1 || len(got) != 1 || got[0].Check != "facts" {
		t.Errorf("json %q, code %d, err %v", jsonOut, code, err)
	}
}

func readCatalog(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(catalogDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCheckRequiredBlueprints(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		wantCode int
		wantSev  string
	}{
		{"warns by default", "", 0, ": warning: blueprints: "},
		{"warns when not required", "[blueprints]\nrequired = false\n", 0, ": warning: blueprints: "},
		{"errors when required", "[blueprints]\nrequired = true\n", 1, ": error: blueprints: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{"blueprints/docs/guide.blueprint.yaml": ""}
			if tt.toml != "" {
				files["drafter.toml"] = tt.toml
			}
			root := project(t, files)
			out, _, code := run(t, "check", "--root", root)
			if code != tt.wantCode || !strings.Contains(out, tt.wantSev+"the managed page docs/guide.md has no blueprint") {
				t.Errorf("code %d, stdout %q", code, out)
			}
		})
	}
}

func TestCheckCannotRun(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		drop  string // a path to delete after the tree is built
		args  []string
		want  string
	}{
		{"no blueprint root", nil, "blueprints", nil, "no blueprint root at blueprints"},
		{"no outline", nil, "blueprints/outline.yaml", nil, "no outline at blueprints/outline.yaml"},
		{"malformed outline", map[string]string{"blueprints/outline.yaml": "version: 1\ntypes: [\n"}, "", nil, "outline blueprints/outline.yaml is malformed"},
		{"malformed lock", map[string]string{"blueprints/drafted.lock": "version: [\n"}, "", nil, "blueprints/drafted.lock"},
		{"named file is not a blueprint", nil, "", []string{"docs/guide.md"}, "docs/guide.md is not a blueprint.yaml file"},
		{"named file off the path rule", map[string]string{"blueprints/.blueprint.yaml": "version: 1\n"}, "", []string{"blueprints/.blueprint.yaml"}, "is not a blueprint.yaml file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := project(t, tt.files)
			if tt.drop != "" {
				if err := os.RemoveAll(filepath.Join(root, tt.drop)); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"check", "--root", root}
			for _, a := range tt.args {
				args = append(args, filepath.Join(root, a))
			}
			_, errs, code := run(t, args...)
			if code != 2 || !strings.Contains(errs, tt.want) {
				t.Errorf("code %d, stderr %q, want 2 and %q", code, errs, tt.want)
			}
		})
	}
}

func TestCheckAbsentLockIsValid(t *testing.T) {
	root := project(t, nil)
	if _, err := os.Stat(filepath.Join(root, "blueprints", "drafted.lock")); !os.IsNotExist(err) {
		t.Fatalf("fixture has a lock: %v", err)
	}
	if _, _, code := run(t, "check", "--root", root); code != 0 {
		t.Errorf("code %d with no lock", code)
	}
}

func TestCheckNamedFile(t *testing.T) {
	root := project(t, nil)
	file := filepath.Join(root, "blueprints", "docs", "guide.blueprint.yaml")
	if _, _, code := run(t, "check", "--root", root, file); code != 0 {
		t.Errorf("code %d for the named base blueprint", code)
	}
}

func TestCheckUsageError(t *testing.T) {
	if _, _, code := run(t, "check", "--nope"); code != 2 {
		t.Errorf("code %d for an unknown flag, want 2", code)
	}
}

func TestRefs(t *testing.T) {
	edited := strings.Replace(readCatalog(t, "base.blueprint.yaml"), "          Only the role requirement.\n",
		"          Only the role requirement.\n"+
			"      - id: needs-admin-again\n        kind: fact\n        fact: f:bbbbbb\n"+
			"      - id: c-admin\n        kind: sentence\n        mode: claim\n        text: |\n          Rotation needs the role.\n        facts: [f:bbbbbb]\n", 1)
	edited = strings.Replace(edited, "the service accepts any key it holds.", "the service accepts any key it holds. Rotation needs `f:bbbbbb`.", 1)
	root := project(t, map[string]string{"blueprints/docs/guide.blueprint.yaml": edited})

	out, _, code := run(t, "refs", "--root", root, "f:bbbbbb")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if code != 0 || len(lines) != 4 {
		t.Fatalf("code %d, stdout %q", code, out)
	}
	for i, want := range []string{"(example)", "needs-admin (fact)", "needs-admin-again (fact)", "c-admin (sentence)"} {
		if !strings.HasPrefix(lines[i], "blueprints/docs/guide.blueprint.yaml:") || !strings.HasSuffix(lines[i], want) {
			t.Errorf("line %d = %q, want a suffix %q", i, lines[i], want)
		}
	}
	if !strings.Contains(lines[0], ": _framing-background (example)") {
		t.Errorf("framing line %q", lines[0])
	}

	jsonOut, _, code := run(t, "refs", "--root", root, "--json", "f:dddddd")
	var rows []refRow
	if err := json.Unmarshal([]byte(jsonOut), &rows); err != nil || code != 0 || len(rows) != 1 || rows[0].Item != "dispositions" || rows[0].Kind != "disposition" {
		t.Errorf("json %q, code %d, err %v", jsonOut, code, err)
	}
}

func TestRefsNoneFound(t *testing.T) {
	root := project(t, nil)
	tests := []struct {
		name string
		id   string
		want string
	}{
		{"held but placed nowhere", "f:eeeeee", "fact f:eeeeee is in the facts store, but no blueprint places or cites it"},
		{"not in the store", "f:nothing", "fact f:nothing is not in the facts store, and no blueprint places or cites it"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, errs, code := run(t, "refs", "--root", root, tt.id)
			if code != 1 || out != "" || errs != tt.want+"\n" {
				t.Errorf("code %d, stdout %q, stderr %q", code, out, errs)
			}
		})
	}
	t.Run("no facts store", func(t *testing.T) {
		noStore := project(t, nil)
		if err := os.RemoveAll(filepath.Join(noStore, "blueprints", "facts")); err != nil {
			t.Fatal(err)
		}
		_, errs, code := run(t, "refs", "--root", noStore, "f:nothing")
		if code != 1 || !strings.Contains(errs, "the project has no facts store") {
			t.Errorf("code %d, stderr %q", code, errs)
		}
	})
}

func TestRefsUsage(t *testing.T) {
	if _, _, code := run(t, "refs"); code != 2 {
		t.Errorf("code %d with no fact id, want 2", code)
	}
}

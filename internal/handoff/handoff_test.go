package handoff

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	planRel    = "docs/internal/briefs/widget/install.plan.yaml"
	framingRel = "docs/internal/briefs/widget/install.framing.md"
)

// copyProject copies testdata/project into a temp dir and returns its root.
func copyProject(t *testing.T) string {
	t.Helper()
	src := filepath.Join("testdata", "project")
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// mutate decodes data as a JSON object, applies fn, and re-encodes it.
func mutate(t *testing.T, data []byte, fn func(m map[string]any)) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if fn != nil {
		fn(m)
	}
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// handoffFile writes the fixture handoff for project, after fn edits it, into a fresh dir.
func handoffFile(t *testing.T, project string, fn func(m map[string]any)) string {
	t.Helper()
	data := bytes.ReplaceAll(readTestdata(t, "handoff.json"), []byte("/abs/worktree"), []byte(project))
	if fn != nil {
		data = mutate(t, data, fn)
	}
	return writeFile(t, t.TempDir(), "handoff.json", data)
}

func TestReadFiles(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(m map[string]any)
		wantErr string
	}{
		{name: "clean"},
		{name: "unknown version", edit: func(m map[string]any) { m["version"] = 2 }, wantErr: "unknown version 2"},
		{name: "unknown field", edit: func(m map[string]any) { m["extra"] = true }, wantErr: `unknown field "extra"`},
		{name: "unknown nested field", edit: func(m map[string]any) {
			sub(m, "page")["extra"] = true
		}, wantErr: `unknown field "extra"`},
		{name: "page path inside", edit: func(m map[string]any) {
			sub(m, "page")["path"] = "docs/widget/../widget/other.md"
		}},
		{name: "page path outside", edit: func(m map[string]any) {
			sub(m, "page")["path"] = "../escape.md"
		}, wantErr: "page.path"},
		{name: "page path absolute outside", edit: func(m map[string]any) {
			sub(m, "page")["path"] = "/etc/passwd"
		}, wantErr: "page.path"},
		{name: "brief outside", edit: func(m map[string]any) {
			sub(m, "page")["brief"] = "docs/../../brief.json"
		}, wantErr: "page.brief"},
		{name: "plan outside", edit: func(m map[string]any) { m["plan"] = "../plan.yaml" }, wantErr: "plan"},
		{name: "framing outside", edit: func(m map[string]any) { m["framing"] = "../framing.md" }, wantErr: "framing"},
		{name: "exemplar outside accepted", edit: func(m map[string]any) {
			sub(m, "inputs")["exemplarSources"] = []any{"/elsewhere/exemplar.md", "../up.md"}
		}},
		{name: "checksum mismatch", edit: func(m map[string]any) {
			m["checksum"] = "sha256:" + strings.Repeat("0", 64)
		}, wantErr: "checksum"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := copyProject(t)
			file := handoffFile(t, project, tt.edit)
			hs, err := ReadFiles(file)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ReadFiles: %v", err)
				}
				if len(hs) != 1 || hs[0].File != file || hs[0].Project != project {
					t.Fatalf("got %+v", hs)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got none", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), file) {
				t.Errorf("error %q lacks %q or the file %s", err, tt.wantErr, file)
			}
		})
	}
}

func TestReadFilesTrailingData(t *testing.T) {
	project := copyProject(t)
	file := handoffFile(t, project, nil)
	data, _ := os.ReadFile(file)
	if err := os.WriteFile(file, append(data, []byte(`{"x":1}`)...), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadFiles(file)
	if err == nil || !strings.Contains(err.Error(), "trailing") || !strings.Contains(err.Error(), file) {
		t.Errorf("got %v", err)
	}
}

func TestReadFilesDecodes(t *testing.T) {
	project := copyProject(t)
	hs, err := ReadFiles(handoffFile(t, project, nil))
	if err != nil {
		t.Fatal(err)
	}
	h := hs[0]
	if h.Page.Type != "task-guide" || h.Plan != planRel || h.Framing != framingRel ||
		h.Inputs.PageType != "task guide" || h.Inputs.FactIDs[0] != "f:abc123" ||
		h.Inputs.ClaimInventory[0].Disposition != "carried" || h.Inputs.ExemplarSources[0] != "/abs/exemplars/guide.md" {
		t.Errorf("decoded %+v", h)
	}
	if !strings.Contains(string(h.Context), `"resolved"`) {
		t.Errorf("context %s", h.Context)
	}
}

func TestReadFilesProjects(t *testing.T) {
	a, b := copyProject(t), copyProject(t)
	fa, fb := handoffFile(t, a, nil), handoffFile(t, b, nil)
	fa2 := handoffFile(t, a, nil)

	if hs, err := ReadFiles(fa, fa2); err != nil || len(hs) != 2 {
		t.Fatalf("same project: %v %d", err, len(hs))
	}
	_, err := ReadFiles(fa, fb)
	if err == nil {
		t.Fatal("two projects accepted")
	}
	if !strings.Contains(err.Error(), fa) || !strings.Contains(err.Error(), fb) {
		t.Errorf("error %q does not name both files", err)
	}
}

func TestReadFilesMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "none.json")
	if _, err := ReadFiles(missing); err == nil || !strings.Contains(err.Error(), missing) {
		t.Errorf("got %v", err)
	}
}

func TestVerifyAfterEdit(t *testing.T) {
	project := copyProject(t)
	hs, err := ReadFiles(handoffFile(t, project, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := hs[0].Verify(); err != nil {
		t.Fatalf("clean Verify: %v", err)
	}
	plan := filepath.Join(project, planRel)
	f, err := os.OpenFile(plan, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("# edited\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	err = hs[0].Verify()
	if err == nil || !strings.Contains(err.Error(), hs[0].File) {
		t.Errorf("edited plan: %v", err)
	}
}

func TestNoFramingChecksumsPlanAlone(t *testing.T) {
	project := copyProject(t)
	plan, err := os.ReadFile(filepath.Join(project, planRel))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(plan)
	want := "sha256:" + hex.EncodeToString(sum[:])
	file := handoffFile(t, project, func(m map[string]any) {
		delete(m, "framing")
		m["checksum"] = want
	})
	hs, err := ReadFiles(file)
	if err != nil {
		t.Fatal(err)
	}
	if hs[0].Framing != "" {
		t.Errorf("framing %q", hs[0].Framing)
	}
	// The same checksum no longer matches once the framing is back.
	withFraming := handoffFile(t, project, func(m map[string]any) { m["checksum"] = want })
	if _, err := ReadFiles(withFraming); err == nil {
		t.Error("plan-only checksum accepted with a framing record")
	}
}

func TestChecksumMatchesSha256sum(t *testing.T) {
	project := copyProject(t)
	plan, _ := os.ReadFile(filepath.Join(project, planRel))
	framing, _ := os.ReadFile(filepath.Join(project, framingRel))
	sum := sha256.Sum256(append(append([]byte{}, plan...), framing...))
	want := "sha256:" + hex.EncodeToString(sum[:])

	got, err := Checksum(project, planRel, framingRel)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if got != "sha256:5b22998ca5a4020e320e195d2842c0e7422586c36a35e173294a8eabc6f882ea" {
		t.Errorf("fixture checksum changed: %s", got)
	}
	if bin, err := exec.LookPath("sha256sum"); err == nil {
		cmd := exec.Command(bin)
		cmd.Stdin = bytes.NewReader(append(append([]byte{}, plan...), framing...))
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		if "sha256:"+strings.Fields(string(out))[0] != got {
			t.Errorf("sha256sum says %s", out)
		}
	}
	if _, err := Checksum(project, "missing.yaml", ""); err == nil {
		t.Error("missing plan accepted")
	}
}

// runFile wraps the chain fixture as a Workflow run file whose worktree is project.
func runFile(t *testing.T, project string, edit func(handoff map[string]any), top func(run map[string]any)) string {
	t.Helper()
	chain := bytes.ReplaceAll(readTestdata(t, "chain-handoff.json"), []byte("/abs/worktree"), []byte(project))
	var h map[string]any
	if err := json.Unmarshal(chain, &h); err != nil {
		t.Fatal(err)
	}
	if edit != nil {
		edit(h)
	}
	run := map[string]any{
		"args":     map[string]any{"pages": []any{"docs/widget/install.md"}},
		"result":   map[string]any{"handoff": h, "escalated": []any{}},
		"status":   "completed",
		"newField": map[string]any{"anything": 1},
	}
	if top != nil {
		top(run)
	}
	data, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	return writeFile(t, t.TempDir(), "wf_test.json", data)
}

func resolvedPage(h map[string]any) map[string]any {
	return asMap(asSlice(sub(h, "resolved")["pages"])[0])
}

func chainPageOf(h map[string]any) map[string]any { return asMap(asSlice(h["pages"])[0]) }

// sub returns m[key] as a JSON object, or nil when it is not one.
func sub(m map[string]any, key string) map[string]any { return asMap(m[key]) }

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func TestReadWorkflowRun(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(h map[string]any)
		top     func(run map[string]any)
		wantErr string
	}{
		{name: "clean"},
		{name: "falsy figure and empty rework", edit: func(h map[string]any) {
			p := resolvedPage(h)
			p["figure"] = false
			p["rework"] = []any{}
		}},
		{name: "figure", edit: func(h map[string]any) { resolvedPage(h)["figure"] = true },
			wantErr: "page docs/widget/install.md carries a figure"},
		{name: "rework", edit: func(h map[string]any) { resolvedPage(h)["rework"] = []any{"intro"} },
			wantErr: "page docs/widget/install.md carries rework"},
		{name: "unknown field in handoff", edit: func(h map[string]any) { h["extra"] = 1 }, wantErr: `unknown field "extra"`},
		{name: "unknown field in handoff page", edit: func(h map[string]any) {
			chainPageOf(h)["extra"] = 1
		}, wantErr: `unknown field "extra"`},
		{name: "unknown version", edit: func(h map[string]any) { h["version"] = 3 }, wantErr: "unknown version 3"},
		{name: "empty pages", edit: func(h map[string]any) { h["pages"] = []any{} }, wantErr: "no pages"},
		{name: "no result", top: func(run map[string]any) { delete(run, "result") }, wantErr: "no result"},
		{name: "null result", top: func(run map[string]any) { run["result"] = nil }, wantErr: "no result"},
		{name: "null handoff", top: func(run map[string]any) {
			run["result"] = map[string]any{"handoff": nil}
		}, wantErr: "missing or null"},
		{name: "missing handoff", top: func(run map[string]any) {
			run["result"] = map[string]any{"escalated": []any{}}
		}, wantErr: "missing or null"},
		{name: "plan outside", edit: func(h map[string]any) {
			chainPageOf(h)["plan"] = "../plan.yaml"
		}, wantErr: "plan"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := copyProject(t)
			file := runFile(t, project, tt.edit, tt.top)
			hs, err := ReadWorkflowRun(file)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ReadWorkflowRun: %v", err)
				}
				if len(hs) != 1 || hs[0].File != file {
					t.Fatalf("got %+v", hs)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got none", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), file) {
				t.Errorf("error %q lacks %q or the file %s", err, tt.wantErr, file)
			}
		})
	}
}

func TestRunFileEqualsHandoffFile(t *testing.T) {
	project := copyProject(t)
	fromRun, err := ReadWorkflowRun(runFile(t, project, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	fromFile, err := ReadFiles(handoffFile(t, project, nil))
	if err != nil {
		t.Fatal(err)
	}
	a, b := fromRun[0], fromFile[0]
	a.File, b.File = "", ""
	var ca, cb any
	if err := json.Unmarshal(a.Context, &ca); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b.Context, &cb); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ca, cb) {
		t.Errorf("context\n run: %s\nfile: %s", a.Context, b.Context)
	}
	a.Context, b.Context = nil, nil
	if !reflect.DeepEqual(a, b) {
		t.Errorf("run file converts to\n%+v\nhandoff file to\n%+v", a, b)
	}
	if err := fromRun[0].Verify(); err != nil {
		t.Errorf("Verify: %v", err)
	}
}

func TestReadWorkflowRunUnreadable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "none.json")
	if _, err := ReadWorkflowRun(missing); err == nil || !strings.Contains(err.Error(), missing) {
		t.Errorf("missing: %v", err)
	}
	bad := writeFile(t, t.TempDir(), "bad.json", []byte("{"))
	if _, err := ReadWorkflowRun(bad); err == nil || !strings.Contains(err.Error(), bad) {
		t.Errorf("malformed: %v", err)
	}
}

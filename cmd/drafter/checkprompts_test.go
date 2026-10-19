package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/prompts"
	"github.com/glw907/drafter/stock"
)

const stockCheckMatrix = `{"host":{"args":{},"resolved":{"pages":[{"path":"docs/a.md"}]}},"cases":[{"name":"c","seat":"draft","context":{"page":{"path":"docs/a.md"},"round":1}}]}`

// moduleNaming returns a module whose draft seat names page.path and then text.
func moduleNaming(t *testing.T, text string) string {
	t.Helper()
	q, err := json.Marshal(text)
	if err != nil {
		t.Fatal(err)
	}
	return "return { draft: (c) => \"Draft \" + c.page.path + \" \" + " + string(q) + " };\n"
}

// runStockCheck runs check-prompts in p's project over stockCheckMatrix and returns its stdout and
// exit status.
func runStockCheck(t *testing.T, p *sinceProject) (string, int) {
	t.Helper()
	matrix := filepath.Join(t.TempDir(), "matrix.json")
	if err := os.WriteFile(matrix, []byte(stockCheckMatrix), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(p.root)
	cmd := newCheckPromptsCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--matrix", matrix})
	err := cmd.Execute()
	if err == nil {
		return out.String(), 0
	}
	ee, ok := errors.AsType[exitError](err)
	if !ok {
		t.Fatalf("check-prompts: %v", err)
	}
	return out.String(), ee.code
}

// swapStockPrompts replaces the exported stock module with src until the test ends.
func swapStockPrompts(t *testing.T, src string) {
	t.Helper()
	orig := stock.Prompts
	stock.Prompts = []byte(src)
	t.Cleanup(func() { stock.Prompts = orig })
}

func trackedStockProject(t *testing.T) *sinceProject {
	t.Helper()
	p := stockModuleProject(t)
	p.write("docs/x.md", "x\n")
	p.git("add", "docs/x.md")
	return p
}

func TestStockCheckPromptsPathAndNames(t *testing.T) {
	tests := []struct {
		name string
		text string
		code int
		want string
	}{
		{"a tracked path passes", "see blueprints/outline.yaml", 0, ""},
		{"a tracked directory passes", "see blueprints/docs/", 0, ""},
		{"a path ending a sentence resolves after the trim", "see docs/x.md.", 0, ""},
		{"a stock file with a slug passes", "read drafter:anatomies.md#task-guide", 0, ""},
		{"a URL is not a path", "see https://example.com/a/b", 0, ""},
		{"an untracked path fails, naming it", "see docs/missing.md", 1, `"docs/missing.md"`},
		{"an unknown stock file fails, naming it", "read drafter:nope.md", 1, `"drafter:nope.md"`},
	}
	for _, s := range []string{"cairn", "docs/internal", "docs-register", "sveltekit", "cloudflare", "wrangler", "glw907", "npm run check:", "option-map"} {
		tests = append(tests, struct {
			name string
			text string
			code int
			want string
		}{"names rejects " + s, "Use " + strings.ToUpper(s) + " here", 1, "names: prompt contains " + `"` + s + `"`})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := trackedStockProject(t)
			swapStockPrompts(t, moduleNaming(t, tt.text))
			out, code := runStockCheck(t, p)
			if code != tt.code {
				t.Fatalf("exit %d, want %d\n%s", code, tt.code, out)
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("output %q lacks %q", out, tt.want)
			}
		})
	}
}

func TestStockCheckPromptsSkipsAProjectModule(t *testing.T) {
	p := newSinceProject(t)
	bad := moduleNaming(t, "see docs/missing.md and Cairn")
	p.write("prompts.js", bad)
	swapStockPrompts(t, bad)
	out, code := runStockCheck(t, p)
	if code != 0 {
		t.Errorf("a project module failed the stock checks: exit %d\n%s", code, out)
	}
}

func TestStockCheckPromptsMatrixHoldsEverySeat(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "e2e", "testdata", "cli", "prompts-matrix.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m promptMatrix
	if err := decodeStrict(raw, &m); err != nil {
		t.Fatal(err)
	}
	mod, err := prompts.Load("stock", stock.Prompts, prompts.Host{Args: m.Host.Args, Resolved: m.Host.Resolved})
	if err != nil {
		t.Fatal(err)
	}
	var have []string
	for _, c := range m.Cases {
		if !slices.Contains(have, c.Seat) {
			have = append(have, c.Seat)
		}
	}
	slices.Sort(have)
	if want := mod.Seats(); !slices.Equal(have, want) {
		t.Errorf("matrix seats %v, stock seats %v", have, want)
	}
}

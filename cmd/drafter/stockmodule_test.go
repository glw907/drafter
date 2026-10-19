package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/stock"
)

// stockModuleProject is a since project whose drafter.toml names no prompts module, so every run
// in it uses the stock one.
func stockModuleProject(t *testing.T) *sinceProject {
	t.Helper()
	p := newSinceProject(t)
	data, err := os.ReadFile(filepath.Join(p.root, "drafter.toml"))
	if err != nil {
		t.Fatal(err)
	}
	p.write("drafter.toml", strings.Replace(string(data), "prompts = \"prompts.js\"\n", "", 1))
	p.git("add", "drafter.toml")
	p.git("commit", "-q", "-m", "use the stock module")
	return p
}

// stockModuleConfig runs drafter config in the project and returns its stdout.
func stockModuleConfig(t *testing.T, p *sinceProject, args ...string) (string, error) {
	t.Helper()
	t.Chdir(p.root)
	cmd := newConfigCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestConfigShowsTheStockModuleRow(t *testing.T) {
	short := regexp.MustCompile(`(?m)^prompts = drafter:prompts\.js sha256:[0-9a-f]{12}  \(default\)$`)
	t.Run("an unset prompts shows the stock module and a 12 digit hash", func(t *testing.T) {
		p := stockModuleProject(t)
		out, err := stockModuleConfig(t, p)
		if err != nil {
			t.Fatalf("config: %v", err)
		}
		if !short.MatchString(out) {
			t.Errorf("no stock prompts row in:\n%s", out)
		}
		if want := "prompts = drafter:prompts.js " + config.ShortHash(stock.Prompts); !strings.Contains(out, want) {
			t.Errorf("config lacks %q:\n%s", want, out)
		}
	})
	t.Run("a path that does not exist shows missing", func(t *testing.T) {
		p := stockModuleProject(t)
		p.write("drafter.toml", "prompts = \"absent.js\"\n"+mustRead(t, filepath.Join(p.root, "drafter.toml")))
		out, err := stockModuleConfig(t, p)
		if err != nil {
			t.Fatalf("config: %v", err)
		}
		if want := "prompts = " + filepath.Join(p.root, "absent.js") + " (missing)  (drafter.toml)"; !strings.Contains(out, want) {
			t.Errorf("config lacks %q:\n%s", want, out)
		}
	})
	t.Run("require_prompts shows as a row and refuses the stock module", func(t *testing.T) {
		p := stockModuleProject(t)
		p.write("drafter.toml", "require_prompts = true\n"+mustRead(t, filepath.Join(p.root, "drafter.toml")))
		out, err := stockModuleConfig(t, p)
		if !strings.Contains(out, "require_prompts = true  (drafter.toml)") {
			t.Errorf("config lacks the require_prompts row:\n%s", out)
		}
		if err == nil || !strings.Contains(err.Error(), config.RefusedStock) {
			t.Errorf("config error %v, want %q", err, config.RefusedStock)
		}
	})
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestReadOnTheStockModule(t *testing.T) {
	line := "prompts: drafter:prompts.js " + config.ShortHash(stock.Prompts)
	t.Run("the preflight passes on the blueprint seat and the report names the module", func(t *testing.T) {
		p := stockModuleProject(t)
		code, out, errOut := p.read(p.bp("a"))
		if code != 0 {
			t.Fatalf("exit %d\nstdout %s\nstderr %s", code, out, errOut)
		}
		if !strings.Contains(out, line+"\n") {
			t.Errorf("stdout lacks %q:\n%s", line, out)
		}
		if n := len(p.stub.requests()); n != 1 {
			t.Errorf("%d requests, want the one blueprint read", n)
		}
	})
	t.Run("the JSON report carries prompts and prompts_hash", func(t *testing.T) {
		p := stockModuleProject(t)
		code, out, errOut := p.read("--json", p.bp("a"))
		if code != 0 {
			t.Fatalf("exit %d\nstdout %s\nstderr %s", code, out, errOut)
		}
		var got struct {
			Prompts     string `json:"prompts"`
			PromptsHash string `json:"prompts_hash"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("stdout is not JSON: %v\n%s", err, out)
		}
		if got.Prompts != "drafter:prompts.js" || got.PromptsHash != config.ShortHash(stock.Prompts) {
			t.Errorf("report = %+v, want the stock module and its short hash", got)
		}
	})
	t.Run("a dry run reports the config's module and sends nothing", func(t *testing.T) {
		p := stockModuleProject(t)
		code, out, errOut := p.read("--dry-run", p.bp("a"))
		if code != 0 {
			t.Fatalf("exit %d\nstdout %s\nstderr %s", code, out, errOut)
		}
		if !strings.Contains(out, line+"\n") {
			t.Errorf("stdout lacks %q:\n%s", line, out)
		}
		if n := len(p.stub.requests()); n != 0 {
			t.Errorf("%d requests in a dry run", n)
		}
	})
	t.Run("the every-page-unchanged early return fills the fields from the config", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			args []string
			want func(t *testing.T, out string)
		}{
			{"text", nil, func(t *testing.T, out string) {
				if !strings.Contains(out, line+"\n") || !strings.Contains(out, "nothing changed since HEAD") {
					t.Errorf("stdout lacks the prompts line and the unchanged page:\n%s", out)
				}
			}},
			{"json", []string{"--json"}, func(t *testing.T, out string) {
				var got struct {
					Prompts     string `json:"prompts"`
					PromptsHash string `json:"prompts_hash"`
				}
				if err := json.Unmarshal([]byte(out), &got); err != nil {
					t.Fatalf("stdout is not JSON: %v\n%s", err, out)
				}
				if got.Prompts != "drafter:prompts.js" || got.PromptsHash != config.ShortHash(stock.Prompts) {
					t.Errorf("report = %+v, want the stock module and its short hash", got)
				}
			}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				p := stockModuleProject(t)
				code, out, errOut := p.read(append(tt.args, "--since", "HEAD", p.bp("a"))...)
				if code != 0 {
					t.Fatalf("exit %d\nstdout %s\nstderr %s", code, out, errOut)
				}
				tt.want(t, out)
				if n := len(p.stub.requests()); n != 0 {
					t.Errorf("%d API calls for an unchanged page", n)
				}
			})
		}
	})
	t.Run("require_prompts refuses before any request", func(t *testing.T) {
		p := stockModuleProject(t)
		p.write("drafter.toml", "require_prompts = true\n"+mustRead(t, filepath.Join(p.root, "drafter.toml")))
		p.git("add", "drafter.toml")
		p.git("commit", "-q", "-m", "require prompts")
		code, _, errOut := p.read(p.bp("a"))
		if code != 2 || !strings.Contains(errOut, config.RefusedStock) {
			t.Errorf("exit %d, stderr %q, want 2 with %q", code, errOut, config.RefusedStock)
		}
		if n := len(p.stub.requests()); n != 0 {
			t.Errorf("%d requests after a refusal", n)
		}
	})
}

func TestCheckPromptsOnTheStockModule(t *testing.T) {
	tests := []struct {
		name    string
		require bool
		code    int
		stderr  string
	}{
		{"the stock module renders its blueprint seat", false, 0, ""},
		{"require_prompts refuses the stock module", true, 2, config.RefusedStock},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := stockModuleProject(t)
			if tt.require {
				p.write("drafter.toml", "require_prompts = true\n"+mustRead(t, filepath.Join(p.root, "drafter.toml")))
			}
			matrix := filepath.Join(t.TempDir(), "matrix.json")
			body := `{"host":{"args":{},"resolved":{"pages":[{"path":"docs/a.md"}]}},"cases":[{"name":"c","seat":"blueprint","context":{"page":{"path":"docs/a.md"},"round":1}}]}`
			if err := os.WriteFile(matrix, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Chdir(p.root)
			cmd := newCheckPromptsCmd()
			cmd.SilenceUsage, cmd.SilenceErrors = true, true
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"--matrix", matrix})
			err := cmd.Execute()
			code := 0
			if err != nil {
				ee, ok := errors.AsType[exitError](err)
				if !ok {
					t.Fatalf("check-prompts: %v", err)
				}
				code = ee.code
				if !strings.Contains(ee.msg, tt.stderr) {
					t.Errorf("message %q lacks %q", ee.msg, tt.stderr)
				}
			}
			if code != tt.code {
				t.Errorf("exit %d, want %d\n%s", code, tt.code, out.String())
			}
		})
	}
}

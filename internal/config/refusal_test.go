package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sandbox is one test's project root and user config file.
type sandbox struct {
	root string
	user string // the user layer's config.toml path
}

// newSandbox clears every DRAFTER_* variable and points the user layer and HOME at temp
// directories, so no test reads the machine's real configuration.
func newSandbox(t *testing.T) sandbox {
	t.Helper()
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "DRAFTER_") {
			t.Setenv(name, "")
			os.Unsetenv(name)
		}
	}
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	return sandbox{
		root: t.TempDir(),
		user: filepath.Join(xdg, "drafter", "config.toml"),
	}
}

func (s sandbox) project() string { return filepath.Join(s.root, "drafter.toml") }

func (s sandbox) local() string { return filepath.Join(s.root, "drafter.local.toml") }

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func row(t *testing.T, c *Config, key string) Source {
	t.Helper()
	for _, r := range c.Sources() {
		if r.Key == key {
			return r
		}
	}
	t.Fatalf("no Sources row for %s", key)
	return Source{}
}

func TestCapsResolveToLowest(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		local    string
		env      map[string]string
		key      string
		want     float64
		wantFrom string
		wantSets []string
	}{
		{
			name:     "local lower than user",
			user:     "page_cap = 10\n",
			local:    "page_cap = 5\n",
			key:      "page_cap",
			want:     5,
			wantFrom: "drafter.local.toml",
			wantSets: []string{"user: 10", "drafter.local.toml: 5"},
		},
		{
			name:     "user lower than local",
			user:     "page_cap = 5\n",
			local:    "page_cap = 10\n",
			key:      "page_cap",
			want:     5,
			wantFrom: "user",
			wantSets: []string{"user: 5", "drafter.local.toml: 10"},
		},
		{
			name:     "ceiling local lower than user",
			user:     "cycle_ceiling = 200\n",
			local:    "cycle_ceiling = 150.5\n",
			key:      "cycle_ceiling",
			want:     150.5,
			wantFrom: "drafter.local.toml",
			wantSets: []string{"user: 200", "drafter.local.toml: 150.5"},
		},
		{
			name:     "ceiling user lower than local",
			user:     "cycle_ceiling = 100\n",
			local:    "cycle_ceiling = 150.5\n",
			key:      "cycle_ceiling",
			want:     100,
			wantFrom: "user",
			wantSets: []string{"user: 100", "drafter.local.toml: 150.5"},
		},
		{
			name:     "environment lower than user",
			user:     "page_cap = 10\n",
			env:      map[string]string{"DRAFTER_PAGE_CAP": "3"},
			key:      "page_cap",
			want:     3,
			wantFrom: "env DRAFTER_PAGE_CAP",
			wantSets: []string{"user: 10", "env DRAFTER_PAGE_CAP: 3"},
		},
		{
			name:     "environment higher than user",
			user:     "page_cap = 5\n",
			env:      map[string]string{"DRAFTER_PAGE_CAP": "9"},
			key:      "page_cap",
			want:     5,
			wantFrom: "user",
			wantSets: []string{"user: 5", "env DRAFTER_PAGE_CAP: 9"},
		},
		{
			name:     "ceiling environment lower than local",
			local:    "cycle_ceiling = 80\n",
			env:      map[string]string{"DRAFTER_CYCLE_CEILING": "60"},
			key:      "cycle_ceiling",
			want:     60,
			wantFrom: "env DRAFTER_CYCLE_CEILING",
			wantSets: []string{"drafter.local.toml: 80", "env DRAFTER_CYCLE_CEILING: 60"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			runGit(t, sb.root, "init", "-q")
			writeFile(t, sb.project(), "mode = \"batch\"\n")
			writeFile(t, filepath.Join(sb.root, ".gitignore"), "drafter.local.toml\n")
			if tt.user != "" {
				writeFile(t, sb.user, tt.user)
			}
			if tt.local != "" {
				writeFile(t, sb.local(), tt.local)
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			c, err := Load(sb.root, Flags{})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			got := c.PageCap
			if tt.key == "cycle_ceiling" {
				got = c.CycleCeiling
			}
			if got != tt.want {
				t.Errorf("%s = %v, want %v", tt.key, got, tt.want)
			}
			r := row(t, c, tt.key)
			if r.From != tt.wantFrom {
				t.Errorf("%s from %q, want %q", tt.key, r.From, tt.wantFrom)
			}
			if strings.Join(r.Sets, "|") != strings.Join(tt.wantSets, "|") {
				t.Errorf("%s sets %q, want %q", tt.key, r.Sets, tt.wantSets)
			}
		})
	}
}

func TestProjectLayerRefusesMoneyAndCredentials(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantKey string
		line    string
	}{
		{"key_command", "mode = \"batch\"\nkey_command = \"op read x\"\n", "key_command", "2"},
		{"key_env", "key_env = \"MY_KEY\"\n", "key_env", "1"},
		{"workspace_id", "\n\nworkspace_id = \"wrkspc_01\"\n", "workspace_id", "3"},
		{"prices", "mode = \"batch\"\n\n[prices]\ndate = \"2026-10-13\"\n", "prices", "3"},
		{"page_cap", "page_cap = 5\n", "page_cap", "1"},
		{"cycle_ceiling", "cycle_ceiling = 100\n", "cycle_ceiling", "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			writeFile(t, sb.project(), tt.content)
			_, err := Load(sb.root, Flags{})
			if err == nil {
				t.Fatal("Load accepted the setting in drafter.toml")
			}
			want := sb.project() + ":" + tt.line + ": " + tt.wantKey +
				" is not allowed here; move it to the user config or drafter.local.toml"
			if err.Error() != want {
				t.Errorf("error %q, want %q", err, want)
			}
		})
	}

	t.Run("clean twin", func(t *testing.T) {
		sb := newSandbox(t)
		writeFile(t, sb.project(), "mode = \"sync\"\n")
		c, err := Load(sb.root, Flags{})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if c.Mode != "sync" {
			t.Errorf("mode = %q, want sync", c.Mode)
		}
	})
}

func TestLocalFileMustBeIgnored(t *testing.T) {
	const refusal = ": not ignored by git; add drafter.local.toml to .gitignore, " +
		"because it can hold a key source and caps"
	tests := []struct {
		name        string
		ignore      string
		track       bool
		wantRefusal bool
	}{
		{"tracked and listed", "drafter.local.toml\n", true, true},
		{"untracked and unlisted", "", false, true},
		{"untracked and listed", "drafter.local.toml\n", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			runGit(t, sb.root, "init", "-q")
			writeFile(t, sb.project(), "mode = \"batch\"\n")
			writeFile(t, sb.local(), "mode = \"sync\"\n")
			if tt.track {
				runGit(t, sb.root, "add", "drafter.local.toml")
			}
			if tt.ignore != "" {
				writeFile(t, filepath.Join(sb.root, ".gitignore"), tt.ignore)
			}
			c, err := Load(sb.root, Flags{})
			if tt.wantRefusal {
				if err == nil {
					t.Fatal("Load accepted a drafter.local.toml git does not ignore")
				}
				if want := sb.local() + refusal; err.Error() != want {
					t.Errorf("error %q, want %q", err, want)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if c.Mode != "sync" {
				t.Errorf("mode = %q, want sync from the local layer", c.Mode)
			}
		})
	}
}

package run

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func TestNewID(t *testing.T) {
	at := time.Date(2026, 10, 13, 14, 22, 33, 0, time.UTC)
	a, b := NewID(at), NewID(at)
	if !regexp.MustCompile(`^20261013T142233Z-[0-9a-f]{8}$`).MatchString(a) {
		t.Errorf("NewID = %q, want timestamp and 8 hex characters", a)
	}
	if a == b {
		t.Errorf("two IDs at one instant are both %q", a)
	}
}

func TestPageKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"docs/guide/intro.md", "docs--guide--intro"},
		{"index.md", "index"},
		{"a/b.txt", "a--b.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := PageKey(tt.in); got != tt.want {
				t.Errorf("PageKey(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	state := t.TempDir()
	dir, err := Create(state, "repo", "20261013T142233Z-7f3a09c1")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(state, "runs", "repo", "20261013T142233Z-7f3a09c1"); dir != want {
		t.Errorf("Create = %q, want %q", dir, want)
	}
	if _, err := Create(state, "repo", "20261013T142233Z-7f3a09c1"); err == nil {
		t.Error("second Create on the same run directory succeeded")
	}
}

func TestLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drafter.lock")
	l, err := TryLock(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = TryLock(path)
	if err == nil {
		t.Fatal("second TryLock on a held lock succeeded")
	}
	if pid := strconv.Itoa(os.Getpid()); !strings.Contains(err.Error(), pid) {
		t.Errorf("error %q does not name pid %s", err, pid)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("lock file gone after Release: %v", err)
	}
	l2, err := TryLock(path)
	if err != nil {
		t.Fatalf("TryLock after Release: %v", err)
	}
	if err := l2.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := &State{
		Schema: StateSchema, DrafterVersion: "v0.1.0", RunID: "id", Repo: "r",
		Project: "p", Branch: "main", Head: "abc", Stopped: "cap",
		Pages: map[string]*PageState{"a--b": {Status: "held"}, "c": {Status: "pending"}},
	}
	if err := want.Save(dir); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestLoadStateErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string // empty: no file
		want    []string
	}{
		{"missing", "", []string{"state.json"}},
		{"malformed", "{nope", []string{"state.json"}},
		{"other schema", `{"schema": 7}`, []string{"schema 7", "schema 1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.content != "" {
				if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, err := LoadState(dir)
			if err == nil {
				t.Fatal("LoadState succeeded")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err, w)
				}
			}
		})
	}
}

func TestPrune(t *testing.T) {
	old := "20260801T000000Z-aaaaaaa1"
	young := "20261010T000000Z-aaaaaaa2"
	pages := func(statuses ...string) map[string]*PageState {
		m := map[string]*PageState{}
		for i, s := range statuses {
			m[string(rune('a'+i))] = &PageState{Status: s}
		}
		return m
	}
	tests := []struct {
		name   string
		id     string
		state  *State // nil: no state file
		raw    string // written as the state file when set
		pruned bool
	}{
		{"old finished", old, &State{Schema: StateSchema, Pages: pages("accepted", "escalated")}, "", true},
		{"old pending", old, &State{Schema: StateSchema, Pages: pages("accepted", "pending")}, "", false},
		{"old running", old, &State{Schema: StateSchema, Pages: pages("running")}, "", false},
		{"old held", old, &State{Schema: StateSchema, Pages: pages("held")}, "", false},
		{"old stopped", old, &State{Schema: StateSchema, Stopped: "cap", Pages: pages("accepted")}, "", false},
		{"young finished", young, &State{Schema: StateSchema, Pages: pages("accepted")}, "", false},
		{"not a run id", "scratch", &State{Schema: StateSchema, Pages: pages("accepted")}, "", false},
		{"no state", old, nil, "", false},
		{"malformed state", old, nil, "{nope", false},
		{"other schema", old, nil, `{"schema": 9}`, false},
		{"null page", old, nil, `{"schema": 1, "pages": {"a": null}}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			dir := filepath.Join(state, "runs", "repo", tt.id)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.state != nil {
				if err := tt.state.Save(dir); err != nil {
					t.Fatal(err)
				}
			}
			if tt.raw != "" {
				if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(tt.raw), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for _, f := range []string{"ledger.jsonl", "ledger.lock"} {
				if err := os.WriteFile(filepath.Join(state, f), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := Prune(state, 30, now)
			if err != nil {
				t.Fatal(err)
			}
			_, statErr := os.Stat(dir)
			if tt.pruned {
				if len(got) != 1 || got[0] != dir || statErr == nil {
					t.Errorf("Prune = %v, dir stat err %v; want %s deleted", got, statErr, dir)
				}
			} else if len(got) != 0 || statErr != nil {
				t.Errorf("Prune = %v, dir stat err %v; want kept", got, statErr)
			}
			for _, f := range []string{"ledger.jsonl", "ledger.lock"} {
				if _, err := os.Stat(filepath.Join(state, f)); err != nil {
					t.Errorf("%s touched: %v", f, err)
				}
			}
		})
	}
}

func TestPruneMissingDirs(t *testing.T) {
	for _, dir := range []string{filepath.Join(t.TempDir(), "none"), t.TempDir()} {
		got, err := Prune(dir, 30, now)
		if err != nil || got != nil {
			t.Errorf("Prune(%s) = %v, %v; want nil, nil", dir, got, err)
		}
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestRepo(t *testing.T) {
	base := t.TempDir()
	main := filepath.Join(base, "myrepo")
	if err := os.Mkdir(main, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, main, "init", "-q")
	gitIn(t, main, "commit", "-q", "--allow-empty", "-m", "init")
	wt := filepath.Join(base, "elsewhere")
	gitIn(t, main, "worktree", "add", "-q", wt)
	for _, root := range []string{main, wt} {
		got, err := Repo(root)
		if err != nil || got != "myrepo" {
			t.Errorf("Repo(%s) = %q, %v; want myrepo", root, got, err)
		}
	}
}

package key

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glw907/drafter/internal/config"
)

const sentinel = "SENTINEL-not-a-real-key-7f3a"

// clearEnv removes every variable that could supply a credential.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name == "DRAFTER_API_KEY" || name == "CLAUDE_CODE_OAUTH_TOKEN" || strings.HasPrefix(name, "ANTHROPIC_") {
			t.Setenv(name, "")
			_ = os.Unsetenv(name)
		}
	}
}

// captureStderr redirects the process's standard error and returns a func that
// restores it and returns what was written.
func captureStderr(t *testing.T) func() string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	return func() string {
		os.Stderr = orig
		_ = w.Close()
		b, _ := io.ReadAll(r)
		_ = r.Close()
		return string(b)
	}
}

func TestResolveRefusals(t *testing.T) {
	tests := []struct {
		name   string
		setEnv map[string]string
		keyEnv string
	}{
		{"only ANTHROPIC_API_KEY", map[string]string{"ANTHROPIC_API_KEY": sentinel}, "DRAFTER_TEST_REFUSE_A"},
		{"only CLAUDE_CODE_OAUTH_TOKEN", map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": sentinel}, "DRAFTER_TEST_REFUSE_B"},
		{"only ANTHROPIC_AUTH_TOKEN", map[string]string{"ANTHROPIC_AUTH_TOKEN": sentinel}, "DRAFTER_TEST_REFUSE_C"},
		{"key_env unset", nil, "DRAFTER_TEST_REFUSE_D"},
		{"key_env empty", map[string]string{"DRAFTER_TEST_REFUSE_E": ""}, "DRAFTER_TEST_REFUSE_E"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			for k, v := range tt.setEnv {
				t.Setenv(k, v)
			}
			got, err := Resolve(&config.Config{KeyEnv: tt.keyEnv})
			if err == nil {
				t.Fatalf("Resolve = %q, want error", got)
			}
			if !strings.Contains(err.Error(), tt.keyEnv) {
				t.Errorf("error %q does not name %s", err, tt.keyEnv)
			}
			if strings.Contains(err.Error(), sentinel) {
				t.Errorf("error %q holds the sentinel", err)
			}
		})
	}
}

func TestResolveUnsetSaysUnset(t *testing.T) {
	clearEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", sentinel)
	_, err := Resolve(&config.Config{KeyEnv: "DRAFTER_TEST_UNSET_MSG"})
	if err == nil || !strings.Contains(err.Error(), "unset") {
		t.Fatalf("error = %v, want one saying unset", err)
	}
}

func TestResolveReadsNamedVariable(t *testing.T) {
	clearEnv(t)
	t.Setenv("DRAFTER_TEST_OTHER_VAR", "  fake-key-value\n")
	t.Setenv("ANTHROPIC_API_KEY", sentinel)
	got, err := Resolve(&config.Config{KeyEnv: "DRAFTER_TEST_OTHER_VAR"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "fake-key-value" {
		t.Errorf("Resolve = %q, want fake-key-value", got)
	}
}

func TestResolveCommandWinsOverVariable(t *testing.T) {
	clearEnv(t)
	t.Setenv("DRAFTER_TEST_WINS_VAR", "from-env")
	got, err := Resolve(&config.Config{KeyEnv: "DRAFTER_TEST_WINS_VAR", KeyCommand: "echo from-command"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-command" {
		t.Errorf("Resolve = %q, want from-command", got)
	}
}

func TestResolveCommandRefusals(t *testing.T) {
	tests := []struct {
		name    string
		command string
		timeout time.Duration
	}{
		{"sentinel on both streams, exit 1", "echo " + sentinel + "; echo " + sentinel + " >&2; exit 1", 0},
		{"whitespace only", "printf '  \\n\\t \\n'", 0},
		{"two words", "echo " + sentinel + " " + sentinel, 0},
		{"sleeps past the timeout", "echo " + sentinel + "; sleep 5", 100 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			if tt.timeout > 0 {
				old := timeout
				timeout = tt.timeout
				t.Cleanup(func() { timeout = old })
			}
			restore := captureStderr(t)
			start := time.Now()
			got, err := Resolve(&config.Config{KeyEnv: "DRAFTER_TEST_CMD_REFUSE", KeyCommand: tt.command})
			captured := restore()
			if err == nil {
				t.Fatalf("Resolve = %q, want error", got)
			}
			if time.Since(start) > 3*time.Second {
				t.Errorf("Resolve took %v", time.Since(start))
			}
			if strings.Contains(err.Error(), sentinel) {
				t.Errorf("error %q holds the sentinel", err)
			}
			if strings.Contains(captured, sentinel) {
				t.Errorf("captured stderr %q holds the sentinel", captured)
			}
		})
	}
}

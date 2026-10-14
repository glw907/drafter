package gate

import (
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/config"
)

func TestCommand(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		page string
		want string
	}{
		{"all four", "g {page} {brief} {slug} {baseline}", "p.md", "g 'p.md' 'b.json' 's' 'main'"},
		{"space", "g {page}", "my page.md", "g 'my page.md'"},
		{"quote", "g {page}", "it's.md", `g 'it'\''s.md'`},
		{"shell variable stays", "for x in a; do echo ${page} {page}; done", "p.md", "for x in a; do echo ${page} 'p.md'; done"},
		{"awk brace stays", "awk '{print $1}' {page}", "p.md", "awk '{print $1}' 'p.md'"},
		{"arm stays", "echo ${arm}", "p.md", "echo ${arm}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Command(tt.tmpl, tt.page, "b.json", "s", "main")
			if got != tt.want {
				t.Errorf("Command = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnv(t *testing.T) {
	t.Setenv("DRAFTER_TEST_KEY", "secret")
	t.Setenv("ANTHROPIC_API_KEY", "other")
	t.Setenv("ANTHROPIC_BASE_URL", "u")
	t.Setenv("RUN_GATE_IF_BUSY", "defer")
	t.Setenv("DRAFTER_KEEP", "yes")
	env := Env(&config.Config{KeyEnv: "DRAFTER_TEST_KEY"})
	joined := "\n" + strings.Join(env, "\n")
	for _, gone := range []string{"DRAFTER_TEST_KEY=", "ANTHROPIC_API_KEY=", "ANTHROPIC_BASE_URL=", "RUN_GATE_IF_BUSY="} {
		if strings.Contains(joined, "\n"+gone) {
			t.Errorf("Env still holds %s", gone)
		}
	}
	if !slices.Contains(env, "DRAFTER_KEEP=yes") {
		t.Errorf("Env dropped an unrelated variable")
	}
}

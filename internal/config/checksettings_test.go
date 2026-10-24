package config

import (
	"strings"
	"testing"
)

func TestCheckSentenceWords(t *testing.T) {
	tests := []struct {
		name, body string
		want       int
		from       string
	}{
		{"unset is off", "mode = \"batch\"\n", 0, "unset"},
		{"set in the project layer", "[check]\nsentence_words = 25\n", 25, "drafter.toml"},
		{"zero is off", "[check]\nsentence_words = 0\n", 0, "drafter.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			writeFile(t, sb.project(), tt.body)
			c, err := Load(sb.root, Flags{})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if c.Check.SentenceWords != tt.want {
				t.Errorf("Check.SentenceWords = %d, want %d", c.Check.SentenceWords, tt.want)
			}
			if r := row(t, c, "check.sentence_words"); r.From != tt.from {
				t.Errorf("row = %+v, want it from %s", r, tt.from)
			}
		})
	}
}

func TestCheckSentenceWordsRefusals(t *testing.T) {
	tests := []struct {
		name, layer, body, want string
	}{
		{"user layer", "user", "[check]\nsentence_words = 20\n", "check is not allowed here; move it to drafter.toml or drafter.local.toml"},
		{"negative", "project", "[check]\nsentence_words = -1\n", "check.sentence_words must be at least 0, not -1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			writeFile(t, sb.project(), "mode = \"batch\"\n")
			if tt.layer == "user" {
				writeFile(t, sb.user, tt.body)
			} else {
				writeFile(t, sb.project(), tt.body)
			}
			_, err := Load(sb.root, Flags{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

package plan

import (
	"slices"
	"testing"
)

func TestExhaustiveClaims(t *testing.T) {
	tests := []struct {
		name, text string
		want       []string
	}{
		{"four matches", "Only admins rotate all keys, and three roles always agree.", []string{"only", "all", "three roles", "always"}},
		{"one match", "Every key expires.", []string{"every"}},
		{"negations are no claim", "It never retries and has no limit.", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExhaustiveClaims(tt.text); !slices.Equal(got, tt.want) {
				t.Errorf("ExhaustiveClaims(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestExhaustiveClaim(t *testing.T) {
	tests := []struct{ name, text, want string }{
		{"only", "Only admins rotate keys.", "only"},
		{"every", "Every key expires.", "every"},
		{"all", "It reads all the files.", "all"},
		{"no is no claim", "There is no limit.", ""},
		{"none", "None expire.", "none"},
		{"never is no claim", "It never retries.", ""},
		{"always", "It always retries.", "always"},
		{"spelled count", "The tool has three roles.", "three roles"},
		{"lowest spelled count", "Two modes exist.", "two modes"},
		{"highest spelled count", "Twelve flags exist.", "twelve flags"},
		{"a digit count is no claim", "It runs 4 steps in Svelte 5 snippets and 10,000 characters.", ""},
		{"case folded", "ALWAYS retry.", "always"},
		{"code span", "Run `rm --all` and `none`.", ""},
		{"no claim", "The key expires after a day.", ""},
		{"hyphenated word", "A no-op and an only-child.", ""},
		{"longer word", "The normal allocator.", ""},
		{"count before a singular", "It waits 3 minute or two attempt.", ""},
		{"thirteen is not listed", "Thirteen flags exist.", ""},
		{"count at the end", "There are two", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExhaustiveClaim(tt.text); got != tt.want {
				t.Errorf("ExhaustiveClaim(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

package plan

import "testing"

func TestExhaustiveClaim(t *testing.T) {
	tests := []struct{ name, text, want string }{
		{"only", "Only admins rotate keys.", "only"},
		{"every", "Every key expires.", "every"},
		{"all", "It reads all the files.", "all"},
		{"no", "There is no limit.", "no"},
		{"none", "None expire.", "none"},
		{"never", "It never retries.", "never"},
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

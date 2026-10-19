package provenance

import (
	"slices"
	"testing"
)

func tokenStrings(ts []token) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.kind+":"+t.value)
	}
	return out
}

func TestProvenanceExtractKinds(t *testing.T) {
	heads := commandHeads([]string{"Run `tally count` to add."})
	tests := []struct {
		name, text string
		want       []string
	}{
		{"a prose date", "Support ended 2026-03-14.", []string{"date:2026-03-14"}},
		{"a code span date", "On `2026-03-14` it ended.", []string{"date:2026-03-14"}},
		{"a prose version", "It needs 2.4.1 or later.", []string{"version:2.4.1"}},
		{"a v-prefixed version with a prerelease", "Try v1.2.3-rc.1 first.", []string{"version:v1.2.3-rc.1"}},
		{"a code span version", "Use `1.2.3`.", []string{"version:1.2.3"}},
		{"a prose numeral", "It has 12 steps.", []string{"numeral:12"}},
		{"a code span numeral", "It has `12` steps.", []string{"numeral:12"}},
		{"a numeral with a percent sign", "About 50% of items.", []string{"numeral:50"}},
		{"a numeral with a unit suffix", "Up to 16k users.", []string{"numeral:16"}},
		{"a prose URL loses its trailing period", "See https://example.com/x/y.", []string{"path:https://example.com/x/y"}},
		{"a rooted prose path", "Open /admin now.", []string{"path:/admin"}},
		{"a prose path with two slashes", "Edit docs/api/page now.", []string{"path:docs/api/page"}},
		{"a prose path with an extension", "Edit docs/guide.md now.", []string{"path:docs/guide.md"}},
		{"one slash and no extension is not a prose path", "Use the read/write mode.", nil},
		{"a code span path", "Edit `src/a.go`.", []string{"path:src/a.go"}},
		{"a dot file code span", "Delete `.tally`.", []string{"path:.tally"}},
		{"a package manager command in prose", "Run npm run build now.", []string{"command:npm run build"}},
		{"a bare install command in prose", "Run npm install first.", []string{"command:npm install"}},
		{"a generic command in a code span", "Run `git status --short`.", []string{"command:git status", "flag:--short"}},
		{"a derived command in a code span", "Run `tally count --by 2 notes.txt`.", []string{"command:tally count", "flag:--by", "path:notes.txt"}},
		{"a one word code span is not a command", "Run `git`.", []string{"name:git"}},
		{"a prose flag", "Pass --force or --dry-run.", []string{"flag:--force", "flag:--dry-run"}},
		{"a flag code span with a value", "Pass `--by=2`.", []string{"flag:--by"}},
		{"a camelCase name", "Call `defineThing`.", []string{"name:defineThing"}},
		{"a dotted name", "Read `config.key`.", []string{"name:config.key"}},
		{"a call drops its parentheses", "Call `run()`.", []string{"name:run"}},
		{"a scoped package", "Install `@scope/pkg`.", []string{"name:@scope/pkg"}},
		{"a link target is not read", "See [the guide](docs/guide.md).", nil},
		{"an image is not read", "Look: ![alt text 12](img/a.png).", nil},
		{"a repeated token is listed once", "Pass `--a` and --a.", []string{"flag:--a"}},
		{"a multi-word span that is no command", "Say `some words here`.", nil},
		{"plain prose", "Tally keeps a count.", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenStrings(extract(tt.text, heads))
			if !slices.Equal(got, tt.want) {
				t.Errorf("extract(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

// Each row exercises a pattern that would be written with a lookbehind or a lookahead in an engine that has them.
func TestProvenanceExtractBoundaries(t *testing.T) {
	tests := []struct {
		name, text string
		want       []string
	}{
		{"a date ends a sentence", "It ended 2026-03-14.", []string{"date:2026-03-14"}},
		{"a date inside a longer token is no date", "Build x2026-03-14 ran.", []string{"numeral:03", "numeral:14"}},
		{"a date followed by a hyphen is no date", "Tag 2026-03-14-rc ran.", []string{"numeral:2026", "numeral:03", "numeral:14"}},
		{"a date after a dot is no date", "File a.2026-03-14 ran.", []string{"numeral:03", "numeral:14"}},
		{"a version ends a sentence", "It needs 1.2.3.", []string{"version:1.2.3"}},
		{"a version followed by a letter is no version", "It needs 1.2.3a now.", nil},
		{"a version followed by a dot and a digit stays whole", "It needs 2.0.1.5 now.", []string{"version:2.0.1.5"}},
		{"a version after a word character is no version", "It needs x1.2.3 now.", nil},
		{"a decimal reads as a version, not a numeral", "It is 3.5 wide.", []string{"version:3.5"}},
		{"a numeral with a thousands comma", "It holds 1,284 items.", []string{"numeral:1284"}},
		{"a numeral with two thousands commas", "It holds 1,234,567 items.", []string{"numeral:1234567"}},
		{"a numeral after a dollar sign is no numeral", "It costs $5 today.", nil},
		{"a numeral inside a word is no numeral", "It is step2 of it.", nil},
		{"a numeral followed by a word is no numeral", "It is 12abcd long.", nil},
		{"a flag after a word character is no flag", "Use a--b now.", nil},
		{"a flag after a hyphen is no flag", "Use x---force now.", nil},
		{"a path after a word character starts at the word", "Edit xdocs/a/b now.", []string{"path:xdocs/a/b"}},
		{"a command after a hyphen is no command", "Run my-npm install now.", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenStrings(extract(tt.text, commandHeads(nil)))
			if !slices.Equal(got, tt.want) {
				t.Errorf("extract(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestProvenanceCommandHeads(t *testing.T) {
	tests := []struct {
		name   string
		claims []string
		want   []string
		absent []string
	}{
		{"the generic set alone", nil, []string{"git", "go", "make", "npm", "npx", "pnpm", "yarn", "node", "curl", "docker", "python", "pip", "cargo"}, []string{"tally"}},
		{"a multi-word span adds its first word", []string{"Run `tally count` now."}, []string{"tally", "git"}, nil},
		{"a one-word span adds nothing", []string{"Run `tally` now."}, []string{"git"}, []string{"tally"}},
		{"the first word of each of several spans", []string{"`a b` and `c d e`"}, []string{"a", "c"}, []string{"b", "d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			heads := commandHeads(tt.claims)
			for _, w := range tt.want {
				if !heads[w] {
					t.Errorf("heads lack %q", w)
				}
			}
			for _, w := range tt.absent {
				if heads[w] {
					t.Errorf("heads hold %q", w)
				}
			}
		})
	}
}

func TestProvenanceContainsBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		claim string
		tok   token
		want  bool
	}{
		{"a numeral in the claim", "Holds 12 items.", token{kindNumeral, "12"}, true},
		{"a numeral in a longer numeral", "Holds 123 items.", token{kindNumeral, "12"}, false},
		{"a numeral after a dot", "Holds 2.5 items.", token{kindNumeral, "5"}, false},
		{"a numeral before a decimal", "Holds 2.5 items.", token{kindNumeral, "2"}, false},
		{"a numeral with a thousands comma in the claim", "Holds 1,284 items.", token{kindNumeral, "1284"}, true},
		{"a numeral with two thousands commas in the claim", "Holds 1,234,567 items.", token{kindNumeral, "1234567"}, true},
		{"a numeral at the end of a sentence", "The default is 1.", token{kindNumeral, "1"}, true},
		{"a version in the claim", "Needs 2.4.1 or later.", token{kindVersion, "2.4.1"}, true},
		{"a v-prefixed claim for a bare version", "Needs v2.4.1 or later.", token{kindVersion, "2.4.1"}, true},
		{"a bare claim for a v-prefixed version", "Needs 2.4.1 or later.", token{kindVersion, "v2.4.1"}, true},
		{"a version that continues with a dot and a digit", "Needs 2.4.1.5 or later.", token{kindVersion, "2.4.1"}, false},
		{"a version that continues with a hyphen", "Needs 2.4.1-rc or later.", token{kindVersion, "2.4.1"}, false},
		{"a version after a word character", "Needs x2.4.1 or later.", token{kindVersion, "2.4.1"}, false},
		{"a version at the end of a sentence", "Needs 2.4.1.", token{kindVersion, "2.4.1"}, true},
		{"a date in the bullet", "Ended 2026-03-14.", token{kindDate, "2026-03-14"}, true},
		{"a path with a trailing slash", "Keep files in src/lib.", token{kindPath, "src/lib/"}, true},
		{"a command as whole words", "Use tally count here.", token{kindCommand, "tally count"}, true},
		{"a command inside a longer command word", "Use tally counter here.", token{kindCommand, "tally count"}, false},
		{"a command followed by a colon", "Use tally count: here.", token{kindCommand, "tally count"}, false},
		{"a flag as a whole word", "Pass --by now.", token{kindFlag, "--by"}, true},
		{"a flag inside a longer flag", "Pass --by-name now.", token{kindFlag, "--by"}, false},
		{"a name as a whole word", "Call `defineThing` now.", token{kindName, "defineThing"}, true},
		{"a name inside a longer name", "Call `defineThings` now.", token{kindName, "defineThing"}, false},
		{"a name after a dollar sign", "Call $defineThing now.", token{kindName, "defineThing"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contains(tt.claim, tt.claim, tt.tok); got != tt.want {
				t.Errorf("contains(%q, %v) = %v, want %v", tt.claim, tt.tok, got, tt.want)
			}
		})
	}
	t.Run("a numeral in the sources only", func(t *testing.T) {
		if contains("Holds items.", "Holds items. src/a.go:5", token{kindNumeral, "5"}) {
			t.Error("a numeral in the sources was contained")
		}
		if !contains("Holds items.", "Holds items. src/a.go:5", token{kindPath, "src/a.go"}) {
			t.Error("a path in the sources was not contained")
		}
	})
}

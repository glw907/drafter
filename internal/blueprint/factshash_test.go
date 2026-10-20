package blueprint

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

func hashOf(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:12]
}

func TestFactsHashes(t *testing.T) {
	const base = "- `f:a` Tally keeps its count in a file. Source: `main.go:12-13` (`stateFile`). [verified]\n" +
		"- `f:b` The flag --by takes a whole number. Source: `main.go:36` (`Int`). [verified]\n"
	store := func(md string) *plan.Facts {
		return plan.ParseFacts(map[string]string{"facts.md": md}, "/nowhere")
	}
	tests := []struct {
		name  string
		ids   []string
		facts *plan.Facts
		want  map[string]string
	}{
		{"the first 12 hex digits of the hash of the whole bullet", []string{"f:a"}, store(base),
			map[string]string{"f:a": hashOf("`f:a` Tally keeps its count in a file. Source: `main.go:12-13` (`stateFile`). [verified]")}},
		{"an id the store lacks is left out", []string{"f:a", "f:zzz"}, store(base),
			map[string]string{"f:a": hashOf("`f:a` Tally keeps its count in a file. Source: `main.go:12-13` (`stateFile`). [verified]")}},
		{"no store gives no map", []string{"f:a"}, nil, nil},
		{"no ids give no map", nil, store(base), nil},
		{"a duplicated id hashes the count alone, since drafter check refuses it before any read", []string{"f:a"}, store(base + "- `f:a` A second bullet. [verified]\n"),
			map[string]string{"f:a": hashOf("duplicate x2")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FactsHashes(tt.ids, tt.facts); !maps.Equal(got, tt.want) {
				t.Errorf("FactsHashes = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("a moved pointer span, a tag qualifier, and a parenthesized anchor each change the hash", func(t *testing.T) {
		was := FactsHashes([]string{"f:a"}, store(base))["f:a"]
		for name, md := range map[string]string{
			"span":      "- `f:a` Tally keeps its count in a file. Source: `main.go:12-14` (`stateFile`). [verified]\n",
			"anchor":    "- `f:a` Tally keeps its count in a file. Source: `main.go:12-13` (`otherName`). [verified]\n",
			"qualifier": "- `f:a` Tally keeps its count in a file. Source: `main.go:12-13` (`stateFile`). [verified: retested]\n",
		} {
			if got := FactsHashes([]string{"f:a"}, store(md))["f:a"]; got == was || got == "" {
				t.Errorf("%s: hash %q did not move from %q", name, got, was)
			}
		}
	})

	t.Run("whitespace in the bullet does not move the hash", func(t *testing.T) {
		was := FactsHashes([]string{"f:a"}, store(base))["f:a"]
		spaced := "- `f:a`   Tally keeps its count\n  in a file.  Source: `main.go:12-13` (`stateFile`). [verified]\n"
		if got := FactsHashes([]string{"f:a"}, store(spaced))["f:a"]; got != was {
			t.Errorf("hash %q, want %q", got, was)
		}
	})
}

func TestChangedFacts(t *testing.T) {
	tests := []struct {
		name              string
		recorded, current map[string]string
		want              []string
	}{
		{"equal maps", map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1", "b": "2"}, nil},
		{"a differing hash", map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1", "b": "9"}, []string{"b"}},
		{"an id now cited that the map lacks", map[string]string{"a": "1"}, map[string]string{"a": "1", "c": "3"}, []string{"c"}},
		{"an id the map holds that is no longer cited", map[string]string{"a": "1", "z": "9"}, map[string]string{"a": "1"}, []string{"z"}},
		{"all three, sorted", map[string]string{"b": "2", "z": "9"}, map[string]string{"b": "0", "a": "1"}, []string{"a", "b", "z"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ChangedFacts(tt.recorded, tt.current); !slices.Equal(got, tt.want) {
				t.Errorf("ChangedFacts = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFactsMatch(t *testing.T) {
	tests := []struct {
		name              string
		recorded, current map[string]string
		want              bool
	}{
		{"equal", map[string]string{"a": "1"}, map[string]string{"a": "1"}, true},
		{"a missing map matches an empty read set", nil, map[string]string{}, true},
		{"a missing map against a read set", nil, map[string]string{"a": "1"}, false},
		{"a map against an empty read set", map[string]string{"a": "1"}, nil, false},
		{"differing hashes", map[string]string{"a": "1"}, map[string]string{"a": "2"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FactsMatch(tt.recorded, tt.current); got != tt.want {
				t.Errorf("FactsMatch = %v, want %v", got, tt.want)
			}
		})
	}
}

package coverage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

type fixture struct {
	plan   *plan.Plan
	page   string
	brief  string
	answer string
}

func load(t *testing.T) fixture {
	t.Helper()
	read := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	p, fs := plan.Decode("plan.yaml", read("plan.yaml"))
	if plan.HasError(fs) {
		t.Fatalf("fixture plan does not decode: %v", fs)
	}
	return fixture{p, string(read("page.md")), string(read("brief.json")), string(read("answer.json"))}
}

// swap replaces the one occurrence of old in s, failing the test when old is absent so a
// fixture edit can't silently turn a mutation into a no-op.
func swap(t *testing.T, s, old, repl string) string {
	t.Helper()
	if strings.Count(s, old) != 1 {
		t.Fatalf("mutation target %q occurs %d times, want 1", old, strings.Count(s, old))
	}
	return strings.Replace(s, old, repl, 1)
}

func summarize(ms []Miss) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.Item+"|"+m.Kind+"|"+m.Reason)
	}
	return out
}

func TestNormalize(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"whitespace", "  a \n\t b  c ", "a b c"},
		{"emphasis", "a **b** and *c* and __d__", "a b and c and d"},
		{"code", "run `make check` now", "run make check now"},
		{"plain", "nothing to do", "nothing to do"},
		{"empty", " \n ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Normalize(tt.in); got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	f := load(t)
	const (
		heading  = "heading not found"
		verbatim = "verbatim sentence not found in this section"
	)
	tests := []struct {
		name   string
		facts  bool
		mutate func(t *testing.T, page, brief string) (string, string)
		want   []string
	}{
		{name: "everything present", facts: true},
		{
			name: "verbatim sentence absent", facts: true,
			mutate: func(t *testing.T, page, brief string) (string, string) {
				return swap(t, page, "The health endpoint reports the new key ID.", "The endpoint is healthy."), brief
			},
			want: []string{"v-health|verbatim|" + verbatim},
		},
		{
			name: "fact cited only in another section", facts: true,
			mutate: func(t *testing.T, page, brief string) (string, string) {
				return page, swap(t, brief, "Revoke the old key with the revoke command.", "The old key keeps working for 24 hours after rotation.")
			},
			want: []string{"f-revoke|fact|no brief sentence in this section cites fact revoke-step"},
		},
		{
			name: "fact items skipped without facts", facts: false,
			mutate: func(t *testing.T, page, brief string) (string, string) {
				return page, swap(t, brief, `"id": "revoke-step"`, `"id": "other"`)
			},
		},
		{
			name: "fact item uncited with facts", facts: true,
			mutate: func(t *testing.T, page, brief string) (string, string) {
				return page, swap(t, brief, `"id": "revoke-step"`, `"id": "other"`)
			},
			want: []string{"f-revoke|fact|no brief sentence in this section cites fact revoke-step"},
		},
		{
			name: "link in another section", facts: true,
			mutate: func(t *testing.T, page, brief string) (string, string) {
				page = swap(t, page, "[key reference](../reference/keys.md#key-lifetime)", "key reference")
				page = swap(t, page, "## Revoke the old key\n", "## Revoke the old key\n\nSee [ref](../reference/keys.md#key-lifetime).\n")
				return page, brief
			},
			want: []string{"l-keys|link|no link to ../reference/keys.md#key-lifetime in this section"},
		},
		{
			name: "same-page anchor differs", facts: true,
			mutate: func(t *testing.T, page, brief string) (string, string) {
				return swap(t, page, "(#revoke-the-old-key)", "(#other)"), brief
			},
			want: []string{"l-anchor|link|no link to #revoke-the-old-key in this section"},
		},
		{
			name: "url differs", facts: true,
			mutate: func(t *testing.T, page, brief string) (string, string) {
				return swap(t, page, "https://example.com/docs)", "https://example.com/docs/)"), brief
			},
			want: []string{"l-url|link|no link to https://example.com/docs in this section"},
		},
		{
			name: "link anchor dropped", facts: true,
			mutate: func(t *testing.T, page, brief string) (string, string) {
				return swap(t, page, "keys.md#key-lifetime)", "keys.md)"), brief
			},
			want: []string{"l-keys|link|no link to ../reference/keys.md#key-lifetime in this section"},
		},
		{
			name: "brief sentence differs from the page", facts: true,
			mutate: func(t *testing.T, page, brief string) (string, string) {
				return page, swap(t, brief, "lives for 90 days", "lives for 91 days")
			},
			want: []string{
				"The key lives for 91 days.|brief|brief sentence not found on the page",
				"f-ttl|fact|no brief sentence in this section cites fact key-ttl",
			},
		},
		{
			name: "brief sentence not on the page", facts: false,
			mutate: func(t *testing.T, page, brief string) (string, string) {
				return page, swap(t, brief, `"sentences": [`, `"sentences": [{"text": "Nowhere on the page.", "id": "x"},`)
			},
			want: []string{"Nowhere on the page.|brief|brief sentence not found on the page"},
		},
		{
			name: "sentence under an unplanned heading moved to another section", facts: true,
			mutate: func(t *testing.T, page, brief string) (string, string) {
				return page, swap(t, brief, "The deploy reads the key from the environment.", "The key lives for 90 days.")
			},
			want: []string{"f-detail|fact|no brief sentence in this section cites fact detail-fact"},
		},
		{
			name: "section heading missing", facts: true,
			mutate: func(t *testing.T, page, brief string) (string, string) {
				return swap(t, page, "## Revoke the old key", "## Retire the old key"), brief
			},
			want: []string{"f-revoke|fact|" + heading, "v-final|verbatim|" + heading},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, brief := f.page, f.brief
			if tt.mutate != nil {
				page, brief = tt.mutate(t, page, brief)
			}
			got, err := Check(f.plan, []byte(page), []byte(brief), tt.facts)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(summarize(got), tt.want) {
				t.Errorf("misses = %q, want %q", summarize(got), tt.want)
			}
		})
	}
}

func TestCheckSectionFields(t *testing.T) {
	f := load(t)
	page := swap(t, f.page, "## Revoke the old key", "## Retire the old key")
	got, err := Check(f.plan, []byte(page), []byte(f.brief), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].Section != "Revoke the old key" {
		t.Errorf("misses = %+v, want the plan section's heading in Section", got)
	}
}

func TestCheckBriefErrors(t *testing.T) {
	f := load(t)
	if _, err := Check(f.plan, []byte(f.page), []byte("not json"), true); err == nil {
		t.Error("a brief that isn't JSON must return an error")
	}
	if _, err := Check(f.plan, []byte(f.page), []byte(`{"sentences": [{"text": "x", "id": 4}]}`), true); err == nil {
		t.Error("a brief id that is neither a string nor a list must return an error")
	}
}

func TestBriefDecodes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "brief-cuts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var b Brief
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatalf("a brief with cuts and local fields must decode: %v", err)
	}
	if len(b.Sentences) != 2 {
		t.Fatalf("sentences = %d, want 2", len(b.Sentences))
	}
	if len(b.Sentences[0].ID) != 0 {
		t.Errorf("no-claim id = %q, want no fact ids", b.Sentences[0].ID)
	}
	if want := []string{"key-ttl", "grace-window"}; !slices.Equal(b.Sentences[1].ID, want) {
		t.Errorf("list id = %q, want %q", b.Sentences[1].ID, want)
	}
	f := load(t)
	if _, err := Check(f.plan, []byte(f.page), data, true); err != nil {
		t.Errorf("Check on a brief with cuts: %v", err)
	}
}

func TestPrompt(t *testing.T) {
	f := load(t)
	got := Prompt(f.plan, []byte(f.page))
	for _, want := range []string{
		"id: c-old", "section: Rotate the key", "The old key keeps working until you revoke it.",
		"id: r-why", "id: h-next", "section: Revoke the old key", "Point the reader to the key reference.",
		f.page,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	for _, not := range []string{"id: v-health", "id: v-final", "id: f-ttl", "id: l-keys"} {
		if strings.Contains(got, not) {
			t.Errorf("prompt lists %q, which the coverage read doesn't answer", not)
		}
	}
}

func TestSchema(t *testing.T) {
	var schema any
	if err := json.Unmarshal(Schema(), &schema); err != nil {
		t.Fatal(err)
	}
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch v := v.(type) {
		case map[string]any:
			for _, banned := range []string{"minLength", "maxLength", "minimum", "maximum", "maxItems", "pattern"} {
				if _, ok := v[banned]; ok {
					t.Errorf("%s: %s is outside the structured-output limits", path, banned)
				}
			}
			if n, ok := v["minItems"].(float64); ok && n > 1 {
				t.Errorf("%s: minItems %v, only 0 or 1 allowed", path, n)
			}
			if v["type"] == "object" {
				if v["additionalProperties"] != false {
					t.Errorf("%s: object must set additionalProperties false", path)
				}
				props, _ := v["properties"].(map[string]any)
				req, _ := v["required"].([]any)
				if len(req) != len(props) {
					t.Errorf("%s: required lists %d of %d properties", path, len(req), len(props))
				}
			}
			for k, c := range v {
				walk(path+"/"+k, c)
			}
		case []any:
			for i, c := range v {
				walk(path+"/"+string(rune('0'+i)), c)
			}
		}
	}
	walk("", schema)
}

func TestVerify(t *testing.T) {
	f := load(t)
	tests := []struct {
		name       string
		mutate     func(t *testing.T, answer string) string
		wantMisses []string
		wantFaults []string
	}{
		{name: "every quote in its section"},
		{
			name: "quote not on the page",
			mutate: func(t *testing.T, a string) string {
				return swap(t, a, "The old key keeps working until you revoke it.", "Never on the page.")
			},
			wantMisses: []string{"c-old|claim|the quoted passage is not in this section"},
			wantFaults: []string{`coverage quote for c-old is not in section "Rotate the key"`},
		},
		{
			name: "quote on the page in another section",
			mutate: func(t *testing.T, a string) string {
				return swap(t, a, "The old key keeps working until you revoke it.", "Revocation cannot be undone.")
			},
			wantMisses: []string{"c-old|claim|the quoted passage is not in this section"},
			wantFaults: []string{`coverage quote for c-old is not in section "Rotate the key" (it is in section "Revoke the old key")`},
		},
		{
			name: "item left out of the answer",
			mutate: func(t *testing.T, a string) string {
				return swap(t, a, `{"id": "r-why", "quote": "Deploying first means a failed deploy leaves the old key working."},`, "")
			},
			wantMisses: []string{"r-why|reason|the coverage read found no passage for this item"},
		},
		{
			name: "empty quote",
			mutate: func(t *testing.T, a string) string {
				return swap(t, a, "Next, read the key reference.", "")
			},
			wantMisses: []string{"h-next|handoff|the coverage read found no passage for this item"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer := f.answer
			if tt.mutate != nil {
				answer = tt.mutate(t, answer)
			}
			misses, faults, err := Verify(f.plan, []byte(f.page), []byte(answer))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(summarize(misses), tt.wantMisses) {
				t.Errorf("misses = %q, want %q", summarize(misses), tt.wantMisses)
			}
			if !slices.Equal(faults, tt.wantFaults) {
				t.Errorf("faults = %q, want %q", faults, tt.wantFaults)
			}
		})
	}
}

func TestVerifyHeadingMissing(t *testing.T) {
	f := load(t)
	page := swap(t, f.page, "## Revoke the old key", "## Retire the old key")
	misses, faults, err := Verify(f.plan, []byte(page), []byte(f.answer))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"h-next|handoff|heading not found"}; !slices.Equal(summarize(misses), want) {
		t.Errorf("misses = %q, want %q", summarize(misses), want)
	}
	if len(faults) != 0 {
		t.Errorf("faults = %q, want none for a missing heading", faults)
	}
}

func TestVerifyBadAnswer(t *testing.T) {
	f := load(t)
	if _, _, err := Verify(f.plan, []byte(f.page), []byte("not json")); err == nil {
		t.Error("an answer that doesn't parse must return an error")
	}
}

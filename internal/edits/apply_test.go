package edits

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

const (
	basePage  = "alpha beta\nalpha gamma\n"
	baseBrief = `{"sentences": [{"text": "alpha", "id": "f:1"}]}`
)

var baseFindings = []Finding{
	{N: 1, Seat: "reader", Location: "line 1", Text: "beta is vague", Rewrite: "delta", Blocking: true},
	{N: 2, Seat: "reader", Location: "line 2", Text: "gamma is odd", Rewrite: "omega"},
}

// cleanAnswer is the accepted twin every refusal row mutates.
func cleanAnswer() Answer {
	return Answer{
		Edits: []Edit{{File: "page", Old: "beta", New: "delta"}},
		Resolutions: []Resolution{
			{Finding: 1, Status: "edited", Edits: []int{0}, Reason: "done"},
			{Finding: 2, Status: "declined", Reason: "fine as is"},
		},
	}
}

func TestApplyClean(t *testing.T) {
	got, err := Apply(basePage, baseBrief, baseFindings, nil, cleanAnswer(), nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if want := "alpha delta\nalpha gamma\n"; got.Page != want {
		t.Errorf("Page = %q, want %q", got.Page, want)
	}
	if got.Brief != baseBrief {
		t.Errorf("Brief = %q, want unchanged", got.Brief)
	}
	if len(got.Resolutions) != 2 || got.Resolutions[0].Finding != 1 || got.Resolutions[1].Finding != 2 {
		t.Errorf("Resolutions = %+v, want findings 1 and 2 in order", got.Resolutions)
	}
}

func TestApplyRefusals(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(a *Answer)
		want   []string
	}{
		{
			name:   "old span found zero times",
			mutate: func(a *Answer) { a.Edits[0].Old = "zeta" },
			want:   []string{"edit 0: old text not found in the page"},
		},
		{
			name:   "old span found several times",
			mutate: func(a *Answer) { a.Edits[0].Old = "alpha" },
			want:   []string{"edit 0: old text occurs 2 times in the page; it must occur exactly once"},
		},
		{
			name:   "file neither page nor brief",
			mutate: func(a *Answer) { a.Edits[0].File = "readme" },
			want:   []string{`edit 0: file "readme" is neither "page" nor "brief"`},
		},
		{
			name:   "old and new equal",
			mutate: func(a *Answer) { a.Edits[0].New = "beta" },
			want:   []string{"edit 0: old and new text are equal"},
		},
		{
			name: "edit no resolution claims",
			mutate: func(a *Answer) {
				a.Edits = append(a.Edits, Edit{File: "page", Old: "gamma", New: "omega"})
			},
			want: []string{"edit 1: no resolution claims it"},
		},
		{
			name:   "edit index that does not exist",
			mutate: func(a *Answer) { a.Resolutions[0].Edits = []int{0, 1} },
			want:   []string{"resolution for finding 1 names edit 1, which does not exist (this answer has 1 edits, indexed from 0)"},
		},
		{
			name: "finding number that does not exist",
			mutate: func(a *Answer) {
				a.Resolutions = append(a.Resolutions, Resolution{Finding: 9, Status: "declined", Reason: "x"})
			},
			want: []string{"resolution names finding 9, which does not exist"},
		},
		{
			name:   "blocking finding with no resolution",
			mutate: func(a *Answer) { a.Resolutions = a.Resolutions[1:]; a.Edits = nil },
			want:   []string{"finding 1 has no resolution"},
		},
		{
			name:   "advisory finding with no resolution",
			mutate: func(a *Answer) { a.Resolutions = a.Resolutions[:1] },
			want:   []string{"finding 2 has no resolution"},
		},
		{
			name: "finding resolved twice",
			mutate: func(a *Answer) {
				a.Resolutions = append(a.Resolutions, Resolution{Finding: 2, Status: "declined", Reason: "again"})
			},
			want: []string{"finding 2 has more than one resolution in this answer"},
		},
		{
			name: "blocking finding edited with no edits",
			mutate: func(a *Answer) {
				a.Edits = nil
				a.Resolutions[0].Edits = nil
			},
			want: []string{"finding 1 is blocking and resolved edited but names no edit"},
		},
		{
			name: "brief not valid JSON after the edits",
			mutate: func(a *Answer) {
				a.Edits = append(a.Edits, Edit{File: "brief", Old: `"id": "f:1"}`, New: `"id": "f:1"`})
				a.Resolutions[0].Edits = []int{0, 1}
			},
			want: []string{"the brief is not valid JSON after the edits"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := cleanAnswer()
			tt.mutate(&a)
			got, err := Apply(basePage, baseBrief, baseFindings, nil, a, nil)
			var ref *Refusal
			if !errors.As(err, &ref) {
				t.Fatalf("err = %v, want *Refusal", err)
			}
			if !slices.Equal(ref.Problems, tt.want) {
				t.Errorf("Problems = %q, want %q", ref.Problems, tt.want)
			}
			if got.Page != "" || got.Brief != "" || got.Resolutions != nil {
				t.Errorf("refused answer returned %+v, want zero Applied", got)
			}
		})
	}
}

func TestApplyBriefEditClean(t *testing.T) {
	a := cleanAnswer()
	a.Edits = append(a.Edits, Edit{File: "brief", Old: `"alpha"`, New: `"alpha beta"`})
	a.Resolutions[0].Edits = []int{0, 1}
	got, err := Apply(basePage, baseBrief, baseFindings, nil, a, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if want := `{"sentences": [{"text": "alpha beta", "id": "f:1"}]}`; got.Brief != want {
		t.Errorf("Brief = %q, want %q", got.Brief, want)
	}
}

func TestApplyEditsInOrder(t *testing.T) {
	page := "one two\none three\n"
	fs := []Finding{{N: 1, Blocking: true}}
	res := []Resolution{{Finding: 1, Status: "edited", Edits: []int{0, 1}}}

	// "one" occurs twice in the original; after edit 0 it occurs once.
	ok := Answer{
		Edits: []Edit{
			{File: "page", Old: "one two", New: "uno two"},
			{File: "page", Old: "one", New: "eins"},
		},
		Resolutions: res,
	}
	got, err := Apply(page, "{}", fs, nil, ok, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if want := "uno two\neins three\n"; got.Page != want {
		t.Errorf("Page = %q, want %q", got.Page, want)
	}

	// The reverse order leaves "one" twice at edit 0.
	bad := Answer{Edits: []Edit{ok.Edits[1], ok.Edits[0]}, Resolutions: res}
	_, err = Apply(page, "{}", fs, nil, bad, nil)
	var ref *Refusal
	if !errors.As(err, &ref) {
		t.Fatalf("err = %v, want *Refusal", err)
	}
	want := []string{"edit 0: old text occurs 2 times in the page; it must occur exactly once"}
	if !slices.Equal(ref.Problems, want) {
		t.Errorf("Problems = %q, want %q", ref.Problems, want)
	}
}

func TestApplyResolutionsAccumulate(t *testing.T) {
	fs := []Finding{
		{N: 1, Blocking: true},
		{N: 2, Blocking: true},
		{N: 3},
	}
	first := Answer{
		Edits: []Edit{{File: "page", Old: "beta", New: "delta"}},
		Resolutions: []Resolution{
			{Finding: 1, Status: "edited", Edits: []int{0}, Reason: "r1"},
			{Finding: 2, Status: "declined", Reason: "r2"},
			{Finding: 3, Status: "declined", Reason: "r3"},
		},
	}
	applied, err := Apply(basePage, baseBrief, fs[:3], nil, first, nil)
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}

	// The gate adds finding 4; the second answer resolves only it, with
	// its own edit indexes starting at 0.
	fs = append(fs, Finding{N: 4, Seat: "gate", Blocking: true})
	second := Answer{
		Edits:       []Edit{{File: "page", Old: "gamma", New: "omega"}},
		Resolutions: []Resolution{{Finding: 4, Status: "edited", Edits: []int{0}, Reason: "r4"}},
	}
	got, err := Apply(applied.Page, applied.Brief, fs, applied.Resolutions, second, nil)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if want := "alpha delta\nalpha omega\n"; got.Page != want {
		t.Errorf("Page = %q, want %q", got.Page, want)
	}
	if len(got.Resolutions) != 4 {
		t.Fatalf("union has %d resolutions, want 4: %+v", len(got.Resolutions), got.Resolutions)
	}

	// A third answer re-resolves finding 2 and overrides it.
	third := Answer{
		Edits:       []Edit{{File: "page", Old: "delta", New: "theta"}},
		Resolutions: []Resolution{{Finding: 2, Status: "edited", Edits: []int{0}, Reason: "r2b"}},
	}
	got, err = Apply(got.Page, got.Brief, fs, got.Resolutions, third, nil)
	if err != nil {
		t.Fatalf("third Apply: %v", err)
	}
	want := Resolution{Finding: 2, Status: "edited", Edits: []int{0}, Reason: "r2b"}
	if !reflect.DeepEqual(got.Resolutions[1], want) {
		t.Errorf("finding 2 = %+v, want %+v", got.Resolutions[1], want)
	}
	if len(got.Resolutions) != 4 {
		t.Errorf("union has %d resolutions, want 4", len(got.Resolutions))
	}
}

func TestApplyRefusalAddsNothingToUnion(t *testing.T) {
	fs := []Finding{{N: 1}, {N: 2}}
	prior := []Resolution{{Finding: 1, Status: "declined", Reason: "p"}}
	bad := Answer{
		Edits:       []Edit{{File: "page", Old: "zeta", New: "x"}},
		Resolutions: []Resolution{{Finding: 2, Status: "edited", Edits: []int{0}}},
	}
	if _, err := Apply(basePage, baseBrief, fs, prior, bad, nil); err == nil {
		t.Fatal("Apply accepted a bad answer")
	}
	// The caller still holds only prior, so finding 2 stays unresolved.
	_, err := Apply(basePage, baseBrief, fs, prior, Answer{}, nil)
	var ref *Refusal
	if !errors.As(err, &ref) || !slices.Equal(ref.Problems, []string{"finding 2 has no resolution"}) {
		t.Errorf("err = %v, want finding 2 unresolved", err)
	}
}

func TestApplyDeclinedFindings(t *testing.T) {
	fs := []Finding{
		{N: 1, Seat: "reader", Blocking: true},
		{N: 2, Seat: "reader"},
		{N: 3, Seat: "reader", Blocking: true},
	}
	a := Answer{Resolutions: []Resolution{
		{Finding: 1, Status: "declined", Reason: "wrong about the page"},
		{Finding: 2, Status: "declined", Reason: "advisory, fine"},
		{Finding: 3, Status: "edited", Edits: []int{}, Reason: "x"},
	}}
	// Finding 3 is blocking and edited with no edits: refused; fix it.
	if _, err := Apply(basePage, baseBrief, fs, nil, a, nil); err == nil {
		t.Fatal("Apply accepted a blocking edited finding with no edits")
	}
	a.Resolutions[2] = Resolution{Finding: 3, Status: "declined", Reason: "also wrong"}
	got, err := Apply(basePage, baseBrief, fs, nil, a, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	d := Declined(fs, got.Resolutions)
	if len(d) != 2 {
		t.Fatalf("Declined = %+v, want findings 1 and 3", d)
	}
	if d[0].Finding.N != 1 || d[0].Reason != "wrong about the page" {
		t.Errorf("d[0] = %+v", d[0])
	}
	if d[1].Finding.N != 3 || d[1].Reason != "also wrong" {
		t.Errorf("d[1] = %+v", d[1])
	}
}

func TestDeclinedOverride(t *testing.T) {
	fs := []Finding{{N: 1, Blocking: true}}
	rs := []Resolution{
		{Finding: 1, Status: "declined", Reason: "first"},
		{Finding: 1, Status: "edited", Edits: []int{0}},
	}
	if d := Declined(fs, rs); len(d) != 0 {
		t.Errorf("Declined = %+v, want none after the override", d)
	}
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

func TestApplyReaderRedraftFixture(t *testing.T) {
	var a Answer
	if err := json.Unmarshal([]byte(readFile(t, "reader-redraft.answer.json")), &a); err != nil {
		t.Fatalf("decode answer: %v", err)
	}
	fs := []Finding{
		{N: 1, Seat: "reader"},
		{N: 2, Seat: "reader", Blocking: true},
		{N: 3, Seat: "reader", Blocking: true},
		{N: 4, Seat: "reader"},
		{N: 5, Seat: "reader"},
	}
	got, err := Apply(readFile(t, "reader-redraft.page.md"), readFile(t, "reader-redraft.brief.json"), fs, nil, a, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !json.Valid([]byte(got.Brief)) {
		t.Errorf("Brief is not valid JSON: %s", got.Brief)
	}
	if len(got.Resolutions) != 5 {
		t.Errorf("got %d resolutions, want 5", len(got.Resolutions))
	}
	d := Declined(fs, got.Resolutions)
	if len(d) != 1 || d[0].Finding.N != 3 {
		t.Errorf("Declined = %+v, want finding 3", d)
	}
}

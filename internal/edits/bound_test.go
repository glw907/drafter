package edits

import (
	"errors"
	"strings"
	"testing"
)

const boundPage = `# Title

Intro line.

## Alpha

Alpha body.

## Beta

Beta body.

## Gamma

Gamma body.
`

const boundBrief = `{"sentences":[{"text":"Intro line.","id":"no-claim"},{"text":"Alpha body.","id":"f-a"},{"text":"Beta body.","id":["f-b"]},{"text":"Gamma body.","id":"f-g"}]}`

func boundAnswer(es ...Edit) Answer {
	idx := make([]int, len(es))
	for i := range es {
		idx[i] = i
	}
	return Answer{Edits: es, Resolutions: []Resolution{{Finding: 1, Status: StatusEdited, Edits: idx, Reason: "r"}}}
}

func pageEdit(old, new string) Edit  { return Edit{File: FilePage, Old: old, New: new} }
func briefEdit(old, new string) Edit { return Edit{File: FileBrief, Old: old, New: new} }

func TestRegionBound(t *testing.T) {
	headings := []string{"Alpha", "Beta", "Gamma"}
	beta := &Bound{Headings: headings, Allowed: []string{"Beta"}}
	alpha := &Bound{Headings: headings, Allowed: []string{"Alpha"}}
	tests := []struct {
		name  string
		bound *Bound
		edits []Edit
		// want are substrings of the one refusal; none means the answer applies.
		want []string
	}{
		{"an edit inside an allowed section applies", beta,
			[]Edit{pageEdit("Beta body.", "Beta body, revised.")}, nil},
		{"an edit in another section is refused naming its line", beta,
			[]Edit{pageEdit("Alpha body.", "Alpha text.")},
			[]string{`section "Alpha"`, `line 7 now reads "Alpha text."`, `only the sections "Beta"`}},
		{"an edit in the preamble is refused naming its line", beta,
			[]Edit{pageEdit("Intro line.", "Intro text.")},
			[]string{"text above the first section", `line 3 now reads "Intro text."`}},
		{"an edit of a heading line is refused naming it", beta,
			[]Edit{pageEdit("## Beta", "##   Beta")},
			[]string{`heading line of section "Beta"`, `line 9 now reads "##   Beta"`}},
		{"deleting an allowed heading is refused", beta,
			[]Edit{pageEdit("## Beta\n\n", "")},
			[]string{`remove the heading of section "Beta"`}},
		{"an unbalanced fence that hides a later heading is refused", beta,
			[]Edit{pageEdit("Beta body.", "```\nBeta body.")},
			[]string{`remove the heading of section "Gamma"`}},
		{"a heading duplicating the next section's is refused", beta,
			[]Edit{pageEdit("Beta body.", "Beta body.\n\n## Gamma\n\nMore.")},
			[]string{`section "Gamma"`}},
		{"a heading duplicating a later section's is refused", alpha,
			[]Edit{pageEdit("Alpha body.", "Alpha body.\n\n## Gamma\n\nMore.")},
			[]string{`add a heading to section "Alpha" that repeats the heading of section "Gamma"`}},
		{"a brief edit to an out-of-scope sentence is refused", beta,
			[]Edit{briefEdit(`"id":"f-a"`, `"id":"f-x"`)},
			[]string{`brief sentence "Alpha body."`}},
		{"an added sentence lying only outside the allowed sections is refused", beta,
			[]Edit{briefEdit(`"f-g"}]}`, `"f-g"},{"text":"Gamma body.","id":"no-claim"}]}`)},
			[]string{`add the brief sentence "Gamma body."`}},
		{"an in-scope sentence may be edited", beta,
			[]Edit{briefEdit(`["f-b"]`, `["f-b","f-c"]`)}, nil},
		{"an in-scope sentence may be added", beta,
			[]Edit{pageEdit("Beta body.", "Beta body. Beta more."), briefEdit(`"f-g"}]}`, `"f-g"},{"text":"Beta more.","id":"no-claim"}]}`)}, nil},
		{"an in-scope sentence may be removed", beta,
			[]Edit{briefEdit(`,{"text":"Beta body.","id":["f-b"]}`, "")}, nil},
		{"a nil bound limits nothing", nil,
			[]Edit{pageEdit("Alpha body.", "Alpha text."), pageEdit("## Beta", "## Beta renamed")}, nil},
	}
	fs := []Finding{{N: 1, Seat: "blueprint", Location: "Beta", Text: "t", Blocking: true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Apply(boundPage, boundBrief, fs, nil, boundAnswer(tt.edits...), tt.bound)
			if len(tt.want) == 0 {
				if err != nil {
					t.Fatalf("Apply refused: %v", err)
				}
				if got.Page == boundPage && got.Brief == boundBrief {
					t.Error("the edits did not apply")
				}
				return
			}
			ref, ok := errors.AsType[*Refusal](err)
			if !ok {
				t.Fatalf("err = %v, want a refusal", err)
			}
			msg := strings.Join(ref.Problems, "\n")
			for _, w := range tt.want {
				if !strings.Contains(msg, w) {
					t.Errorf("refusal %q lacks %q", msg, w)
				}
			}
		})
	}
}

func TestRegionBoundWaitsForEveryEditToApply(t *testing.T) {
	b := &Bound{Headings: []string{"Alpha", "Beta", "Gamma"}, Allowed: []string{"Beta"}}
	a := boundAnswer(pageEdit("Alpha body.", "Alpha text."), pageEdit("not on the page", "x"))
	_, err := Apply(boundPage, boundBrief, []Finding{{N: 1, Blocking: true}}, nil, a, b)
	ref, ok := errors.AsType[*Refusal](err)
	if !ok {
		t.Fatalf("err = %v, want a refusal", err)
	}
	for _, p := range ref.Problems {
		if strings.Contains(p, "outside") || strings.Contains(p, "only the sections") {
			t.Errorf("a bound problem %q was reported though an edit did not apply", p)
		}
	}
}

func TestBoundInstructionsSayToDecline(t *testing.T) {
	got := BoundInstructions()
	for _, s := range []string{"declined", "outside the sections"} {
		if !strings.Contains(got, s) {
			t.Errorf("BoundInstructions lacks %q", s)
		}
	}
}

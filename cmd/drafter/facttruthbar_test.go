package main

import (
	"reflect"
	"slices"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/pipeline"
)

func TestBarKeepsSeatAndKind(t *testing.T) {
	got := barOf("abc", []pipeline.FindingResult{
		{Location: "intro", Text: "Say more.", Blocking: true, Seat: "blueprint"},
		{Location: "fact f:a", Text: "wider: less (excerpt: x)", Blocking: true, Seat: "facttruth", Kind: "fact"},
		{Location: "fact f:b", Text: "no-excerpt: none", Seat: "facttruth", Kind: "fact"},
	})
	want := &blueprint.Blocked{Commit: "abc", Findings: []blueprint.BlockedFinding{
		{Location: "intro", Text: "Say more.", Seat: "blueprint"},
		{Location: "fact f:a", Text: "wider: less (excerpt: x)", Seat: "facttruth", Kind: "fact"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("barOf = %+v, want %+v", got, want)
	}
}

func TestRenderBarLeavesOutFactTruthFindings(t *testing.T) {
	b := &blueprint.Blocked{Commit: "abc", Findings: []blueprint.BlockedFinding{
		{Location: "fact f:a", Text: "wider: less", Seat: "facttruth", Kind: "fact"},
		{Location: "steps", Text: "A step is missing.", Seat: "consistency"},
		{Location: "fact f:b", Text: "contradicts: other", Seat: "facttruth", Kind: "fact"},
		{Location: "intro", Text: "Say what the page leaves out."},
	}}
	if got, want := renderBar(b), "1. [steps] A step is missing.\n2. [intro] Say what the page leaves out."; got != want {
		t.Errorf("renderBar = %q, want %q (the bytes a bar without fact-truth findings renders)", got, want)
	}
	if got, want := barFactIDs(b), []string{"f:a", "f:b"}; !slices.Equal(got, want) {
		t.Errorf("barFactIDs = %v, want %v", got, want)
	}
	if got := barFactIDs(&blueprint.Blocked{Findings: []blueprint.BlockedFinding{{Location: "fact f:c", Text: "x"}}}); got != nil {
		t.Errorf("barFactIDs of a seatless finding = %v, want none", got)
	}
}

func TestHeldPageKeepsTheBarsSeat(t *testing.T) {
	b := &blueprint.Blocked{Commit: "abcdef0123456789", Findings: []blueprint.BlockedFinding{{Location: "fact f:a", Text: "wider: less", Seat: "facttruth", Kind: "fact"}}}
	r := heldPage("docs/a.md", b)
	if len(r.Findings) != 1 || r.Findings[0].Seat != "facttruth" || r.Findings[0].Kind != "fact" {
		t.Errorf("held findings = %+v, want the bar's seat and kind", r.Findings)
	}
}

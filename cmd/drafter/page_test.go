package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/pipeline"
)

func TestDryExitCode(t *testing.T) {
	tests := []struct {
		name string
		s    pipeline.Summary
		want int
	}{
		{"pages still running", pipeline.Summary{Pages: []pipeline.PageResult{{Status: pipeline.StatusRunning}}}, 0},
		{"a page the plan check escalated", pipeline.Summary{Pages: []pipeline.PageResult{{Status: pipeline.StatusEscalated}}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dryExitCode(tt.s); got != tt.want {
				t.Errorf("dry exit code = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestExitCodeFor(t *testing.T) {
	accepted := pipeline.PageResult{Status: pipeline.StatusAccepted}
	escalated := pipeline.PageResult{Status: pipeline.StatusEscalated}
	tests := []struct {
		name string
		s    pipeline.Summary
		want int
	}{
		{"every page accepted", pipeline.Summary{Pages: []pipeline.PageResult{accepted, accepted}}, 0},
		{"one page escalated", pipeline.Summary{Pages: []pipeline.PageResult{accepted, escalated}}, 1},
		{"stopped with every page pending", pipeline.Summary{Stop: pipeline.StopCycleCap, Pages: []pipeline.PageResult{{Status: pipeline.StatusPending}}}, 3},
		{"stopped with an escalated page outranks it", pipeline.Summary{Stop: pipeline.StopSpendLimit, Pages: []pipeline.PageResult{escalated, accepted}}, 3},
		{"a page left running with no stop", pipeline.Summary{Pages: []pipeline.PageResult{{Status: pipeline.StatusRunning}}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCodeFor(tt.s); got != tt.want {
				t.Errorf("exit code = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSummaryPrintsFindings(t *testing.T) {
	c := stockSummaryConfig(t)
	page := pipeline.PageResult{
		Path: "docs/a.md", Status: pipeline.StatusEscalated, Rule: "plan", Cost: 0.25,
		Findings: []pipeline.FindingResult{
			{Location: "rotate", Text: "A step the page needs is missing.", Blocking: true},
			{Location: "intro", Text: "Nit: the opening repeats the title."},
			{Location: "", Text: "Already present: the page has no summary."},
		},
		Warnings: []string{"Unplaced: scope difference, not reported"},
		Detail:   "lumen-key-ttl conflicts with lumen-grace: they disagree",
	}
	var text, js bytes.Buffer
	if err := writeSummaryText(&text, c, pipeline.Summary{Prompts: "drafter:prompts.js", PromptsHash: "sha256:0123456789ab", Pages: []pipeline.PageResult{page}}, false); err != nil {
		t.Fatal(err)
	}
	want := "prices: 2026-10-18 (default)\nprompts: drafter:prompts.js sha256:0123456789ab\ndocs/a.md: escalated (plan) $0.2500\n" +
		"  blocking: rotate: A step the page needs is missing.\n" +
		"  non-blocking: intro: Nit: the opening repeats the title.\n" +
		"  non-blocking: Already present: the page has no summary.\n" +
		"  warning: Unplaced: scope difference, not reported\n" +
		"  escalation: lumen-key-ttl conflicts with lumen-grace: they disagree\n"
	if text.String() != want {
		t.Errorf("text summary = %q, want %q", text.String(), want)
	}
	if err := writeSummaryJSON(&js, c, pipeline.Summary{Pages: []pipeline.PageResult{page}}); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Pages []struct {
			Findings []struct {
				Location string
				Text     string
				Blocking bool
			}
			Warnings []string
			Detail   string
		}
	}
	if err := json.Unmarshal(js.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	g := got.Pages[0]
	if len(g.Findings) != 3 || !g.Findings[0].Blocking || g.Findings[1].Blocking || g.Findings[1].Location != "intro" || g.Findings[2].Blocking {
		t.Errorf("JSON findings = %+v, want three with only the first blocking", g.Findings)
	}
	if len(g.Warnings) != 1 || g.Detail != page.Detail {
		t.Errorf("JSON warnings %q, detail %q", g.Warnings, g.Detail)
	}

	var quiet bytes.Buffer
	if err := writeSummaryJSON(&quiet, c, pipeline.Summary{Pages: []pipeline.PageResult{{Path: "docs/a.md", Status: pipeline.StatusAccepted}}}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"findings"`, `"warnings"`, `"detail"`} {
		if strings.Contains(quiet.String(), key) {
			t.Errorf("a page with none of them carries %s:\n%s", key, quiet.String())
		}
	}
}

func TestStoppedSyncReadPrintsAResumeCommandDrafterAccepts(t *testing.T) {
	p := newSinceProject(t)
	p.stub.failFrom = 1
	code, out, errOut := p.read(p.bp("a"), p.bp("b"))
	if code != 3 {
		t.Fatalf("exit %d, want 3\nstdout %s\nstderr %s", code, out, errOut)
	}
	_, after, ok := strings.Cut(out, "resume with: ")
	if !ok {
		t.Fatalf("no resume line in %q", out)
	}
	line := strings.TrimSpace(after)
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "drafter" || fields[1] != "read" || fields[2] != "--sync" || fields[3] != "--resume" {
		t.Fatalf("resume line = %q, want drafter read --sync --resume <run-id> <blueprint>...", line)
	}
	if got, want := strings.Join(fields[5:], " "), p.bp("a")+" "+p.bp("b"); got != want {
		t.Errorf("resume line blueprints = %q, want %q", got, want)
	}
	p.stub.mu.Lock()
	p.stub.failFrom, p.stub.bodies = 0, nil
	p.stub.mu.Unlock()
	if code, out, errOut := p.read(fields[3:]...); code != 0 {
		t.Errorf("the printed command exits %d\nstdout %s\nstderr %s", code, out, errOut)
	}
}

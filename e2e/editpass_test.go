package e2e

import (
	"slices"
	"strings"
	"testing"
)

// TestDefaultPageSendsTheDraftAndTheEdit runs drafter page with no flag on the fixture: the page is
// drafted, gated, edited once, and accepted on the gate, and no read or review request is sent.
func TestDefaultPageSendsTheDraftAndTheEdit(t *testing.T) {
	e := newDraftEnv(t)
	r := e.pageDefault(draftPage)
	if r.code != 0 {
		t.Fatalf("exit %d, sent %v\n%s%s", r.code, e.stub.sent(), r.stdout, r.stderr)
	}
	acceptedPage(t, e.summary(r))
	if got, want := e.stub.sent(), []string{"draft", "edit"}; !slices.Equal(got, want) {
		t.Errorf("requests = %v, want %v", got, want)
	}
	entry := e.lockEntry()
	if entry.Page == "" || entry.Accepted != "" {
		t.Errorf("lock entry = %+v, want the drafted page and no accepted blueprint, since no read ran", entry)
	}
	e.gate()
}

// TestJudgePageSendsTodaysSequenceAndTheEdit runs the same page with --judge: every configured read,
// with the edit request after the draft.
func TestJudgePageSendsTodaysSequenceAndTheEdit(t *testing.T) {
	e := newDraftEnv(t)
	r := e.page(draftPage)
	if r.code != 0 {
		t.Fatalf("exit %d, sent %v\n%s%s", r.code, e.stub.sent(), r.stdout, r.stderr)
	}
	acceptedPage(t, e.summary(r))
	want := []string{"consistency", "read", "draft", "edit", "coverage", "read", "read", "read", "reader1", "read"}
	if got := e.stub.sent(); !slices.Equal(got, want) {
		t.Errorf("requests = %v, want %v", got, want)
	}
	if e.lockEntry().Accepted == "" {
		t.Error("a judged page's lock entry lacks the accepted blueprint")
	}
}

// TestRedGateAfterTheEditRunsOneGateFixExchange has the edit pass break the page and its brief's
// agreement, which the stock gate's provenance check refuses; the gate-fix turn repairs it.
func TestRedGateAfterTheEditRunsOneGateFixExchange(t *testing.T) {
	e := newDraftEnv(t)
	e.stub.setAnswer("edit", `{"edits":[{"file":"page","old":"The file is named .tally.","new":"The file is called .tally."}],`+
		`"resolutions":[{"finding":1,"status":"edited","edits":[0],"reason":"Reworded."}]}`)
	e.stub.setAnswer("edits", `{"edits":[{"file":"page","old":"The file is called .tally.","new":"The file is named .tally."}],`+
		`"resolutions":[{"finding":2,"status":"edited","edits":[0],"reason":"Restored the sentence the brief records."}]}`)
	r := e.pageDefault(draftPage)
	if r.code != 0 {
		t.Fatalf("exit %d, sent %v\n%s%s", r.code, e.stub.sent(), r.stdout, r.stderr)
	}
	acceptedPage(t, e.summary(r))
	if got, want := e.stub.sent(), []string{"draft", "edit", "edits"}; !slices.Equal(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	if page := e.read(draftPage); strings.Contains(page, "is called .tally") {
		t.Error("the page keeps the sentence the gate refused")
	}
	e.gate()
}

// TestPreferencesReachTheDraftAndTheEdit has the page carry the project's preferences file in the
// draft and edit prompts, and refuses a run whose preferences file is missing before any request.
func TestPreferencesReachTheDraftAndTheEdit(t *testing.T) {
	const prefs = "Spell out a flag the first time it appears."
	e := newDraftEnv(t)
	e.write("drafter.toml", e.read("drafter.toml")+"\n[editing]\npreferences = \"editing.md\"\n")
	e.commit("name the preferences file", "drafter.toml")

	r := e.pageDefault(draftPage)
	if r.code != 2 || !strings.Contains(r.stderr, "editing.preferences") {
		t.Fatalf("exit %d, stderr %q; want 2 naming editing.preferences", r.code, r.stderr)
	}
	if n := len(e.stub.sent()); n != 0 {
		t.Fatalf("%d requests before the refusal", n)
	}

	e.write("editing.md", prefs+"\n")
	e.commit("add the preferences file", "editing.md")
	r = e.pageDefault(draftPage)
	if r.code != 0 {
		t.Fatalf("exit %d, sent %v\n%s%s", r.code, e.stub.sent(), r.stdout, r.stderr)
	}
	if got, want := e.stub.sent(), []string{"draft", "edit"}; !slices.Equal(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	for i, body := range e.stub.sentBodies() {
		if !strings.Contains(body, "Editing preferences.") || !strings.Contains(body, prefs) {
			t.Errorf("request %d lacks the preferences", i)
		}
	}
}

// TestJudgeRequiresBlueprintArguments refuses --judge on a handoff file before any request.
func TestJudgeRequiresBlueprintArguments(t *testing.T) {
	e := newDraftEnv(t)
	r := e.drafter("page", "--judge", "handoff.json")
	if r.code != 2 || !strings.Contains(r.stderr, "--judge runs on blueprints and pages") {
		t.Errorf("exit %d, stderr %q, want 2 and the --judge refusal", r.code, r.stderr)
	}
	if n := len(e.stub.sent()); n != 0 {
		t.Errorf("%d requests before the refusal", n)
	}
}

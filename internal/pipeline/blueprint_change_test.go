package pipeline

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/blueprint"
)

// changeBlockA is a change block as drafter read builds it, shortened to the parts the tests find.
const changeBlockA = "The change this read judges follows.\n\n=== blueprint diff ===\n-old line\n+new line\n=== end blueprint diff ===\n\n" +
	"=== changed facts ===\n- lumen-key-ttl\n  was: [verified] ninety days\n  now: [verified] sixty days\n=== end changed facts ==="

func scopedFacts(ids []string, facts ...string) blueprint.Scope {
	if facts == nil {
		facts = []string{}
	}
	return blueprint.Scope{Kind: blueprint.ScopeScoped, IDs: ids, Facts: facts}
}

// scopedBasePairing is a scoped page as Compare builds it now: facts the change touched, the
// facts whose bullet changed, and the facts that joined the page. Both lists are non-nil.
func scopedBasePairing(facts, bullets, joined []string) blueprint.Scope {
	return blueprint.Scope{Kind: blueprint.ScopeScoped, Facts: facts, Bullets: append([]string{}, bullets...), Joined: append([]string{}, joined...)}
}

func statusPage(s Summary, page string) PageResult {
	for _, p := range s.Pages {
		if p.Path == page {
			return p
		}
	}
	return PageResult{}
}

// TestScopeReachesTheBlueprintPrompt checks that a scoped page's ids and change block reach the
// request, that a whole page sends neither, and that a resume with no scope in its options sends
// the bytes the run recorded. The scoped page is the one whose request is built after the resume.
func TestScopeReachesTheBlueprintPrompt(t *testing.T) {
	h := newBlueprintHarness(t, withInFlight(1))
	h.api.crash = func(call string, ids []string) string {
		if call == "create" && h.api.creates == 2 {
			return "before"
		}
		return ""
	}
	o := h.options()
	o.Since = h.head()
	o.Scopes = map[string]blueprint.Scope{
		pageA: {Kind: blueprint.ScopeWhole, Why: "no earlier blueprint"},
		pageB: scopedFacts([]string{"intro", "steps"}, "lumen-key-ttl"),
	}
	o.Changes = map[string]string{pageB: changeBlockA}
	if !dies(func() { _, _ = Run(context.Background(), o) }) {
		t.Fatal("the run did not die before the second page's batch")
	}
	h.api.crash = nil
	s, err := h.resume(h.runID())
	if err != nil {
		t.Fatalf("resume: %v\nstderr: %s", err, h.err.String())
	}
	reqs := requestsFor(h, "s1", "blueprint")
	if len(reqs) != 2 {
		t.Fatalf("%d blueprint requests, want 2", len(reqs))
	}
	var scoped, whole batch.Message
	for _, r := range reqs {
		m := r.Messages[len(r.Messages)-1]
		if strings.Contains(m.Text, "These sections changed:") {
			scoped = m
		} else {
			whole = m
		}
	}
	if want := "=== end outline entry ===\n\nThese sections changed: intro,steps.\n\n" + changeBlockA; !strings.Contains(scoped.Text, want) {
		t.Errorf("the change block, opened by the scope sentence, does not follow the outline entry:\n%s", scoped.Text)
	}
	if strings.Contains(whole.Text, "=== blueprint diff ===") || !strings.Contains(whole.Text, "=== blueprint ===") {
		t.Errorf("the whole page's third block is %q, want the blueprint and no change block", whole.Text)
	}
	for _, m := range []batch.Message{scoped, whole} {
		if len(m.Cached) != 2 {
			t.Fatalf("%d cached blocks, want 2", len(m.Cached))
		}
		if strings.Contains(m.Cached[0], "These sections changed") || strings.Contains(m.Cached[0], "=== blueprint ===") || strings.Contains(m.Cached[0], "blueprint diff") {
			t.Errorf("the first block holds the blueprint or the change:\n%s", m.Cached[0])
		}
		if !strings.Contains(m.Cached[1], "=== facts placed or cited ===") || !strings.Contains(m.Cached[1], "apart from the change block") {
			t.Errorf("the second block is not the facts block:\n%s", m.Cached[1])
		}
	}
	got := statusPage(s, pageB)
	if got.Scope == nil || got.Scope.Kind != blueprint.ScopeScoped || got.Scope.Facts == nil || len(got.Scope.Facts) != 1 || got.Scope.Facts[0] != "lumen-key-ttl" {
		t.Errorf("summary scope for the scoped page = %+v, want scoped with its facts kept across the resume", got.Scope)
	}
	if k := statusPage(s, pageA).Scope; k == nil || k.Kind != blueprint.ScopeWhole {
		t.Errorf("summary scope for the whole page = %+v", k)
	}
}

// TestOutlineOnlyChangeRendersTheChangeRead checks that a scoped page with no section ids still
// gets the change read: the seat context carries the scope "page" and the request the block.
func TestOutlineOnlyChangeRendersTheChangeRead(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"))
	o := h.options()
	o.Since = h.head()
	o.Scopes = map[string]blueprint.Scope{pageA: scopedFacts(nil)}
	o.Changes = map[string]string{pageA: "=== outline entry change ===\nbefore:\nx\nafter:\ny\n=== end outline entry change ==="}
	if _, err := Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	reqs := requestsFor(h, "s1", "blueprint")
	if len(reqs) != 1 || !strings.Contains(userText(reqs[0]), "=== end outline entry ===\n\nNo section's own content changed: the change is in the page-level fields, the order of the sections, a removed section, the outline entry, or a fact bullet cited only outside the sections.\n\n=== outline entry change ===") {
		t.Errorf("requests = %d, want one with scope page and the entry change", len(reqs))
	}
}

// TestScopedStateWithoutFactsReadsWhole covers a state recorded before Scope.Facts existed: nil
// facts mean a whole read, so neither the scope line nor the block is sent and a conflict blocks.
func TestScopedStateWithoutFactsReadsWhole(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"))
	h.api.script("s1", "consistency", okText(`{"conflicts":[{"facts":["lumen-key-ttl"],"with":"lumen-grace","conflict":"they disagree"}]}`))
	o := h.options()
	o.Since = h.head()
	o.Scopes = map[string]blueprint.Scope{pageA: {Kind: blueprint.ScopeScoped, IDs: []string{"rotate"}}}
	o.Changes = map[string]string{pageA: changeBlockA}
	s, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	reqs := requestsFor(h, "s1", "blueprint")
	if len(reqs) != 1 || strings.Contains(userText(reqs[0]), "These sections changed") || strings.Contains(userText(reqs[0]), "No section's own content changed") || strings.Contains(userText(reqs[0]), "blueprint diff") {
		t.Errorf("a scoped state with no facts sent the change read")
	}
	if statusOf(s, pageA) != StatusEscalated || h.record(s.RunID, pageA).Escalation.Seat != "consistency" {
		t.Errorf("status %s, want escalated on consistency", statusOf(s, pageA))
	}
}

// TestChangeReadsShareTheirFirstBlocks checks that two change reads of one blueprint, with
// different scopes and an added citation in the change, send byte-identical cached blocks. On the
// fake module this proves the assembler's split only: the real prompt must not name the scope.
func TestChangeReadsShareTheirFirstBlocks(t *testing.T) {
	read := func(scope blueprint.Scope, change string) batch.Message {
		h := newBlueprintHarness(t, withPages("a"))
		o := h.options()
		o.Since = h.head()
		o.Scopes = map[string]blueprint.Scope{pageA: scope}
		o.Changes = map[string]string{pageA: change}
		if _, err := Run(context.Background(), o); err != nil {
			t.Fatal(err)
		}
		reqs := requestsFor(h, "s1", "blueprint")
		if len(reqs) != 1 {
			t.Fatalf("%d blueprint requests, want 1", len(reqs))
		}
		return reqs[0].Messages[len(reqs[0].Messages)-1]
	}
	one := read(scopedFacts([]string{"intro"}, "lumen-key-ttl"), changeBlockA)
	two := read(scopedFacts([]string{"steps", "rotate"}, "lumen-key-ttl", "lumen-grace"), changeBlockA+"\n\n=== changed facts ===\n- lumen-grace\n  was: x\n  now: y\n=== end changed facts ===")
	if len(one.Cached) != 2 || !slices.Equal(one.Cached, two.Cached) {
		t.Errorf("cached blocks differ between the reads:\n%q\n%q", one.Cached, two.Cached)
	}
	if one.Text == two.Text {
		t.Error("the third blocks are equal, want them to carry each read's scope and change")
	}
}

// TestBasePairingLeavesTheChangeReadBytesAlone checks that Scope.Bullets and Scope.Joined steer
// only the consistency rule: the request a change read sends is the same with and without them.
func TestBasePairingLeavesTheChangeReadBytesAlone(t *testing.T) {
	read := func(scope blueprint.Scope) []batch.Message {
		h := newBlueprintHarness(t, withPages("a"))
		o := h.options()
		o.Since = h.head()
		o.Scopes = map[string]blueprint.Scope{pageA: scope}
		o.Changes = map[string]string{pageA: changeBlockA}
		if _, err := Run(context.Background(), o); err != nil {
			t.Fatal(err)
		}
		var out []batch.Message
		for _, r := range requestsFor(h, "s1", "blueprint") {
			out = append(out, r.Messages[len(r.Messages)-1])
		}
		return out
	}
	old := scopedFacts([]string{"intro"}, "lumen-key-ttl")
	paired := scopedBasePairing([]string{"lumen-key-ttl"}, []string{"lumen-key-ttl"}, []string{"lumen-grace"})
	paired.IDs = old.IDs
	without, with := read(old), read(paired)
	if len(without) != 1 || len(with) != 1 {
		t.Fatalf("%d and %d blueprint requests, want 1 each", len(without), len(with))
	}
	if without[0].Text != with[0].Text || !slices.Equal(without[0].Cached, with[0].Cached) {
		t.Errorf("the request changed with Bullets and Joined set:\n%q\n%q", without[0].Text, with[0].Text)
	}
}

func TestChangeReadConsistency(t *testing.T) {
	const grace, ttl = "lumen-grace", "lumen-key-ttl"
	conflict := func(facts []string, with string) string {
		return jsonText(map[string]any{"conflicts": []any{map[string]any{"facts": facts, "with": with, "conflict": "they disagree"}}})
	}
	two := func(a []string, aw, atext string, b []string, bw, btext string) string {
		return jsonText(map[string]any{"conflicts": []any{
			map[string]any{"facts": a, "with": aw, "conflict": atext},
			map[string]any{"facts": b, "with": bw, "conflict": btext},
		}})
	}
	const emptyFacts = `{"conflicts":[{"facts":[],"with":"","conflict":"scope difference, not reported"}]}`
	withLink := func(h *harness) {
		h.commitBlueprint(bpA, func(s string) string {
			return s + "      - id: l-keys\n        kind: link\n        target: ../reference/keys.md\n"
		})
	}
	tests := []struct {
		name     string
		scope    blueprint.Scope
		answer   string
		blueprnt string
		link     bool
		want     string   // "accepted", or the seat the page escalates on
		demoted  int      // the summary's Demoted on an accepted page
		warns    []string // the record's warning prefixes, in order, on an accepted page
		detail   []string // texts the escalation detail holds
	}{
		{"an untouched fact conflict is already present", scopedFacts(nil), conflict([]string{ttl}, grace), "ok", false, "accepted", 1, []string{"Already present: "}, nil},
		{"a touched fact in facts blocks", scopedFacts(nil, ttl), conflict([]string{ttl}, grace), "ok", false, "consistency", 0, nil, nil},
		{"a touched fact only in with blocks", scopedFacts(nil, grace), conflict([]string{ttl}, grace), "ok", false, "consistency", 0, nil, nil},
		{"an empty facts list blocks", scopedFacts(nil), conflict(nil, grace), "ok", false, "consistency", 0, nil, nil},
		{"an unknown fact id blocks", scopedFacts(nil), conflict([]string{"no-such-fact"}, grace), "ok", false, "consistency", 0, nil, nil},
		{"a with that matches nothing blocks", scopedFacts(nil), conflict([]string{ttl}, "somewhere else"), "ok", false, "consistency", 0, nil, nil},
		{"a whole read blocks on an untouched conflict", blueprint.Scope{Kind: blueprint.ScopeWhole, Why: "x"}, conflict([]string{ttl}, grace), "ok", false, "consistency", 0, nil, nil},
		{"an untouched conflict with a blueprint fix blocks on the blueprint", scopedFacts(nil), conflict([]string{ttl}, grace), "fix", false, "blueprint", 0, nil, nil},
		{"a scoped state from before Facts blocks", blueprint.Scope{Kind: blueprint.ScopeScoped}, conflict([]string{ttl}, grace), "ok", false, "consistency", 0, nil, nil},
		{"an untouched conflict with a linked page and heading is already present", scopedFacts(nil), conflict([]string{ttl}, "docs/reference/keys.md#Key lifetime"), "ok", true, "accepted", 1, []string{"Already present: "}, nil},
		{"a conflict with an unlinked page blocks", scopedFacts(nil), conflict([]string{ttl}, "docs/reference/anchors.md#Anchors"), "ok", true, "consistency", 0, nil, nil},
		{"a conflict with a linked page whose fact was touched blocks", scopedFacts(nil, ttl), conflict([]string{ttl}, "docs/reference/keys.md#Key lifetime"), "ok", true, "consistency", 0, nil, nil},
		{"an unplaced conflict is set aside as a warning", scopedFacts(nil), emptyFacts, "ok", false, "accepted", 1, []string{"Unplaced: scope difference, not reported"}, nil},
		{"an unplaced conflict beside an untouched one is two warnings", scopedFacts(nil), two(nil, "", "no home", []string{ttl}, grace, "old overlap"), "ok", false, "accepted", 2, []string{"Unplaced: no home", "Already present: "}, nil},
		{"an unplaced conflict beside a touched one blocks and is named", scopedFacts(nil, ttl), two(nil, "", "no home", []string{ttl}, grace, "live clash"), "ok", false, "consistency", 0, nil, []string{"no home", "live clash"}},
		{"an unplaced conflict in a whole read blocks", blueprint.Scope{Kind: blueprint.ScopeWhole, Why: "x"}, emptyFacts, "ok", false, "consistency", 0, nil, nil},
		{"an unplaced conflict in a scoped state from before Facts blocks", blueprint.Scope{Kind: blueprint.ScopeScoped}, emptyFacts, "ok", false, "consistency", 0, nil, nil},
		{"a named fact with an empty with blocks", scopedFacts(nil), conflict([]string{ttl}, ""), "ok", false, "consistency", 0, nil, nil},
		{"an empty facts list with a with blocks", scopedFacts(nil), conflict([]string{}, grace), "ok", false, "consistency", 0, nil, nil},
		{"an unplaced conflict with a blueprint fix blocks on the blueprint", scopedFacts(nil), emptyFacts, "fix", false, "blueprint", 0, nil, nil},
		{"a re-cited conflict the base carried is already present", scopedBasePairing([]string{ttl}, nil, nil), conflict([]string{ttl}, grace), "ok", false, "accepted", 1, []string{"Already present: "}, nil},
		{"a re-cited with fact the base carried is already present", scopedBasePairing([]string{grace}, nil, nil), conflict([]string{ttl}, grace), "ok", false, "accepted", 1, []string{"Already present: "}, nil},
		{"both sides re-cited are already present", scopedBasePairing([]string{ttl, grace}, nil, nil), conflict([]string{ttl}, grace), "ok", false, "accepted", 1, []string{"Already present: "}, nil},
		{"a with fact the change brought onto the page blocks", scopedBasePairing([]string{ttl, grace}, nil, []string{grace}), conflict([]string{ttl}, grace), "ok", false, "consistency", 0, nil, nil},
		{"a fact the change brought onto the page blocks when it is in facts", scopedBasePairing([]string{ttl, grace}, nil, []string{ttl}), conflict([]string{ttl}, grace), "ok", false, "consistency", 0, nil, nil},
		{"a conflict whose bullet changed blocks", scopedBasePairing([]string{ttl}, []string{ttl}, nil), conflict([]string{ttl}, grace), "ok", false, "consistency", 0, nil, nil},
		{"a conflict whose with bullet changed blocks", scopedBasePairing(nil, []string{grace}, nil), conflict([]string{ttl}, grace), "ok", false, "consistency", 0, nil, nil},
		{"a bullet change outside the conflict leaves it already present", scopedBasePairing([]string{ttl}, []string{"other-fact"}, []string{"another"}), conflict([]string{ttl}, grace), "ok", false, "accepted", 1, []string{"Already present: "}, nil},
		{"a scoped state with nil bullets and joined keeps the facts rule", blueprint.Scope{Kind: blueprint.ScopeScoped, Facts: []string{ttl}}, conflict([]string{ttl}, grace), "ok", false, "consistency", 0, nil, nil},
		{"a scoped state with nil joined keeps the facts rule", blueprint.Scope{Kind: blueprint.ScopeScoped, Facts: []string{ttl}, Bullets: []string{}}, conflict([]string{ttl}, grace), "ok", false, "consistency", 0, nil, nil},
		{"a scoped state with nil bullets keeps the facts rule", blueprint.Scope{Kind: blueprint.ScopeScoped, Facts: []string{ttl}, Joined: []string{}}, conflict([]string{ttl}, grace), "ok", false, "consistency", 0, nil, nil},
		{"a re-cited fact in a conflict with a linked page still blocks", scopedBasePairing([]string{ttl}, nil, nil), conflict([]string{ttl}, "docs/reference/keys.md#Key lifetime"), "ok", true, "consistency", 0, nil, nil},
		{"a re-cited conflict with an unknown fact still blocks", scopedBasePairing([]string{ttl}, nil, nil), conflict([]string{"no-such-fact"}, grace), "ok", false, "consistency", 0, nil, nil},
		{"a re-cited conflict with an empty with still blocks", scopedBasePairing([]string{ttl}, nil, nil), conflict([]string{ttl}, ""), "ok", false, "consistency", 0, nil, nil},
		{"a re-cited conflict with a blueprint fix blocks on the blueprint", scopedBasePairing([]string{ttl}, nil, nil), conflict([]string{ttl}, grace), "fix", false, "blueprint", 0, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newBlueprintHarness(t, withPages("a"))
			if tt.link {
				withLink(h)
			}
			h.api.script("s1", "consistency", okText(tt.answer))
			if tt.blueprnt == "fix" {
				h.api.script("s1", "blueprint", okText(readText("fix", finding("rotate", "A step is missing."))))
			}
			o := h.options()
			o.Since = h.head()
			o.Scopes = map[string]blueprint.Scope{pageA: tt.scope}
			s, err := Run(context.Background(), o)
			if err != nil {
				t.Fatalf("Run: %v\nstderr: %s", err, h.err.String())
			}
			rec := h.record(s.RunID, pageA)
			if tt.want == "accepted" {
				if statusOf(s, pageA) != StatusAccepted {
					t.Fatalf("status %s (%s), want accepted", statusOf(s, pageA), ruleOf(s, pageA))
				}
				if got := statusPage(s, pageA).Demoted; got != tt.demoted {
					t.Errorf("demoted conflicts = %d, want %d", got, tt.demoted)
				}
				var v string
				for _, x := range rec.Verdicts {
					if x.Seat == "consistency" {
						v = x.Verdict
					}
				}
				if v != "demoted" {
					t.Errorf("consistency verdict = %q, want demoted", v)
				}
				if len(rec.Warnings) != len(tt.warns) {
					t.Fatalf("warnings = %v, want prefixes %v", rec.Warnings, tt.warns)
				}
				for i, w := range tt.warns {
					if !strings.HasPrefix(rec.Warnings[i], w) {
						t.Errorf("warning %d = %q, want prefix %q", i, rec.Warnings[i], w)
					}
				}
				return
			}
			if statusOf(s, pageA) != StatusEscalated || rec.Escalation == nil || rec.Escalation.Seat != tt.want {
				t.Errorf("status %s, escalation %+v, want escalated on %s", statusOf(s, pageA), rec.Escalation, tt.want)
			}
			for _, d := range tt.detail {
				if !strings.Contains(rec.Escalation.Detail, d) {
					t.Errorf("escalation detail %q lacks %q", rec.Escalation.Detail, d)
				}
			}
			if got := statusPage(s, pageA).Demoted; got != 0 && tt.want != "blueprint" {
				t.Errorf("demoted conflicts = %d on an escalated page", got)
			}
		})
	}
}

func TestChangeReadKeepsAlreadyPresentFindingsOnAccept(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"))
	older := map[string]any{"location": "rotate", "finding": "Already present: the step has no timeout.", "rewrite": "add one", "blocking": false}
	h.api.script("s1", "blueprint", okText(readText("ok", older)))
	o := h.options()
	o.Since = h.head()
	o.Scopes = map[string]blueprint.Scope{pageA: scopedFacts([]string{"rotate"})}
	s, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if statusOf(s, pageA) != StatusAccepted {
		t.Fatalf("status %s, want accepted", statusOf(s, pageA))
	}
	rec := h.record(s.RunID, pageA)
	if len(rec.Findings) != 1 || rec.Findings[0].Blocking || !strings.HasPrefix(rec.Findings[0].Text, "Already present:") {
		t.Errorf("findings = %+v, want the non-blocking finding kept on an accepted page", rec.Findings)
	}
	if got := statusPage(s, pageA).AlreadyPresent; got != 1 {
		t.Errorf("already present = %d, want 1", got)
	}
}

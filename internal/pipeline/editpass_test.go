package pipeline

import (
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/edits"
	"github.com/glw907/drafter/internal/prompts"
)

// sequence lists the stage and seat of every request the fake API saw, in order.
func sequence(h *harness) []string {
	var out []string
	for _, r := range h.api.requests {
		stage, seat := stageSeat(r.CustomID)
		out = append(out, stage+"."+seat)
	}
	return out
}

// brokenStep is the edit that turns a step into text the test gate refuses.
const brokenStep = inNew + " BROKEN"

func TestNarrowedPageSendsOnlyTheDraftAndTheEdit(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	s := h.mustDraft(drafting{narrow: true})
	mustAccept(t, h, s)
	if got, want := sequence(h), []string{"s2.draft", "s2e.edit"}; !slices.Equal(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	rec := h.record(s.RunID, pageA)
	for _, seat := range []string{"structure", "editor", "fact", "reader", "coverage", "consistency"} {
		if !slices.Contains(rec.SeatsNotRun, seat) {
			t.Errorf("seats not run = %v, want it to name %s", rec.SeatsNotRun, seat)
		}
	}
	w := h.workOf(s.RunID)
	if !w.Narrow || !w.SkipRead || !w.Edit || !w.Edited {
		t.Errorf("work = narrow %v, skip_read %v, edit %v, edited %v; want all true", w.Narrow, w.SkipRead, w.Edit, w.Edited)
	}
	if acceptedOf(s, pageA) != "" {
		t.Error("a page that took no blueprint read carries an accepted id")
	}
	edit := h.api.requests[1]
	if edit.Model != h.cfg.Seat["edit"].Model || !isEditsRequest(edit) {
		t.Errorf("the edit request = model %q, edits schema %v", edit.Model, isEditsRequest(edit))
	}
	text := requestText(edit)
	for _, want := range []string{"seat edit reads docs/guides/rotate-the-api-key.md", "=== blueprint ===", "The edit pass:", "=== file: " + pageA + " ==="} {
		if !strings.Contains(text, want) {
			t.Errorf("the edit request lacks %q", want)
		}
	}
}

func TestJudgedPageSendsTodaysSequenceAndTheEdit(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	s := h.mustDraft(drafting{})
	mustAccept(t, h, s)
	want := []string{
		"s1.consistency", "s1.blueprint", "s2.draft", "s2e.edit", "s3.coverage",
		"s4.structure", "s4.editor", "s4.fact", "s7.reader", "s7.reader",
	}
	if got := sequence(h); !slices.Equal(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	if rec := h.record(s.RunID, pageA); len(rec.SeatsNotRun) != 0 {
		t.Errorf("seats not run = %v, want none", rec.SeatsNotRun)
	}
}

func TestRedGateAfterTheEditRunsOneGateFixExchange(t *testing.T) {
	for _, tt := range []struct {
		name string
		run  drafting
		want []string
	}{
		{"narrowed", drafting{narrow: true}, []string{"s2.draft", "s2e.edit", "s2e.edit"}},
		{"judged", drafting{}, []string{
			"s1.consistency", "s1.blueprint", "s2.draft", "s2e.edit", "s2e.edit", "s3.coverage",
			"s4.structure", "s4.editor", "s4.fact", "s7.reader", "s7.reader",
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newBlueprintHarness(t, withPages("a"), withDrafting)
			h.api.script(s2e, "edit", okText(answer(inOld, brokenStep, 1)), okText(answer(brokenStep, inNew, 2)))
			s := h.mustDraft(tt.run)
			mustAccept(t, h, s)
			if got := sequence(h); !slices.Equal(got, tt.want) {
				t.Fatalf("requests = %v, want %v", got, tt.want)
			}
			fix := requestsFor(h, s2e, "edit")[1]
			if text := requestText(fix); !strings.Contains(text, "The gate is red") || !strings.Contains(text, "BROKEN") {
				t.Errorf("the second edit turn does not carry the gate's finding:\n%s", text)
			}
			if got := len(fix.Messages); got != 3 {
				t.Errorf("the gate-fix turn has %d messages, want the edit's two and its own", got)
			}
			page := h.read(pageA)
			if strings.Contains(page, "BROKEN") || !strings.Contains(page, inNew) {
				t.Errorf("the page after the gate fix = %q", page)
			}
		})
	}
}

func TestRedGateAfterTheEditEscalatesOnceTheFixBudgetIsSpent(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	h.cfg.Retries.GateFix = 1
	h.api.script(s2e, "edit",
		okText(answer(inOld, brokenStep, 1)),
		okText(answer(brokenStep, brokenStep+"!", 2)),
		okText(answer(brokenStep+"!", brokenStep+"!!", 3)))
	s := h.mustDraft(drafting{narrow: true})
	if got, rule := statusOf(s, pageA), ruleOf(s, pageA); got != StatusEscalated || rule != "red gate" {
		t.Fatalf("status = %s (%s), want escalated on the red gate", got, rule)
	}
	if n := len(requestsFor(h, s2e, "edit")); n != 2 {
		t.Errorf("edit requests = %d, want the edit and one gate-fix turn", n)
	}
}

func TestEditPassOfASectionRedraftIsBoundedAndSkipsTheCoherenceRead(t *testing.T) {
	h := newSectionHarness(t)
	h.api.script(s2, "draft", okText(answer(inOld, inNew, 1)))
	s := h.mustDraft(sectionScope("rotate").narrowed())
	mustAccept(t, h, s)
	want := []string{"s2.draft", "s2e.edit"}
	if got := sequence(h); !slices.Equal(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	text := requestText(requestsFor(h, s2e, "edit")[0])
	for _, want := range []string{"scope [rotate]", edits.BoundInstructions(), "These sections changed: rotate."} {
		if !strings.Contains(text, want) {
			t.Errorf("the section redraft's edit request lacks %q", want)
		}
	}
	if strings.Contains(text, changeIn) {
		t.Error("the edit request carries the draft's change block")
	}
}

func TestEditPassOfASectionRedraftRefusesAnEditOutsideTheSection(t *testing.T) {
	h := newSectionHarness(t)
	h.api.script(s2, "draft", okText(answer(inOld, inNew, 1)))
	h.api.script(s2e, "edit", okText(answer("## Before you begin", "## Before you start", 1)))
	s := h.mustDraft(sectionScope("rotate").narrowed())
	mustAccept(t, h, s)
	if n := len(requestsFor(h, s2e, "edit")); n != 2 {
		t.Fatalf("edit requests = %d, want the refused answer and its retry", n)
	}
	if !strings.Contains(requestText(requestsFor(h, s2e, "edit")[1]), "Your answer was refused") {
		t.Error("the retry does not carry the refusal")
	}
	if strings.Contains(h.read(pageA), "Before you start") {
		t.Error("an edit outside the section was applied")
	}
}

func TestPreferencesReachTheDraftAndTheEditPrompts(t *testing.T) {
	const prefs = "Spell out a flag the first time it appears.\nPrefer \"run\" to \"execute\".\n"
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	writeFile(t, h.root, "docs/editing.md", prefs)
	gitIn(t, h.root, "add", "docs/editing.md")
	gitIn(t, h.root, "commit", "-q", "-m", "preferences")
	h.cfg.Editing.Preferences = "docs/editing.md"
	s := h.mustDraft(drafting{narrow: true})
	mustAccept(t, h, s)
	for _, r := range []batch.Request{requestsFor(h, s2, "draft")[0], requestsFor(h, s2e, "edit")[0]} {
		if !strings.Contains(requestText(r), "preferences ["+prefs+"]") {
			t.Errorf("request %s lacks the preferences verbatim:\n%s", r.CustomID, requestText(r))
		}
	}
	if got := requestText(requestsFor(h, s2e, "edit")[0]); !strings.Contains(got, "markdown [false]") {
		t.Errorf("a YAML blueprint's edit request says it is Markdown:\n%s", got)
	}
}

func TestAMissingPreferencesFileRefusesTheRunBeforeAnyRequest(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	h.cfg.Editing.Preferences = "docs/editing.md"
	_, err := h.runDrafting(drafting{narrow: true})
	if err == nil || !strings.Contains(err.Error(), "editing.preferences") {
		t.Fatalf("Run error = %v, want the editing.preferences refusal", err)
	}
	if len(h.api.calls) != 0 {
		t.Errorf("the API was called before the refusal: %v", h.api.calls)
	}
}

// markdownBlueprint commits a Markdown blueprint for page A and points its handoff at it.
func (h *harness) markdownBlueprint() (path, body string) {
	h.t.Helper()
	path = "blueprints/docs/guides/rotate-the-api-key.blueprint.md"
	body = "---\npage: Rotate the API key\ntype: task-guide\ntrack: operate\n---\n\n# Rotate the API key\n\n" +
		":::reader\nAn operator whose key is about to expire.\n:::\n\n" +
		"## Before you begin\n\n*Tells the reader what to have ready.*\n\n- You have an API key. :fact[lumen-key-ttl]\n\n" +
		"## Rotate the key\n\n*Walks the reader through the rotation.*\n\n1. Create a new key. :fact[lumen-key-ttl]\n"
	writeFile(h.t, h.root, path, body)
	gitIn(h.t, h.root, "add", path)
	gitIn(h.t, h.root, "commit", "-q", "-m", "markdown blueprint")
	h.handoffs[0] = h.blueprintHandoff(pageA, briefA, path, "Rotate a key")
	return path, body
}

func TestMarkdownBlueprintIsABlueprintNotAMarkdownPlan(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	md, _ := h.markdownBlueprint()
	o := h.options()
	o.Draft, o.DryRun, o.Judge = true, true, true
	r, err := newRunner(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	defer r.release()
	p := r.pages[0]
	if !p.blueprint || p.markdown || !p.mdBlueprint() || p.ps.Step != stBpCheck {
		t.Errorf("%s = blueprint %v, markdown plan %v, markdown blueprint %v, step %q", md, p.blueprint, p.markdown, p.mdBlueprint(), p.ps.Step)
	}
	if got := p.w.Rec.PlanFormat; got != "blueprint" {
		t.Errorf("plan format = %q, want blueprint", got)
	}
	if !p.context(prompts.Round1).Markdown {
		t.Error("the seat context does not say the blueprint is Markdown")
	}
	yaml := newBlueprintHarness(t, withPages("a"), withDrafting)
	o = yaml.options()
	o.Draft, o.DryRun = true, true
	ry, err := newRunner(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	defer ry.release()
	if ry.pages[0].context(prompts.Round1).Markdown {
		t.Error("a YAML blueprint's seat context says it is Markdown")
	}
}

// TestMarkdownBlueprintDraftsAndEdits runs a page from a Markdown blueprint on the default path.
func TestMarkdownBlueprintDraftsAndEdits(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	_, body := h.markdownBlueprint()
	s := h.mustDraft(drafting{narrow: true})
	mustAccept(t, h, s)
	if got, want := sequence(h), []string{"s2.draft", "s2e.edit"}; !slices.Equal(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	for _, r := range h.api.requests {
		text := requestText(r)
		if !strings.Contains(text, "markdown [true]") {
			t.Errorf("request %s does not render with the Markdown flag:\n%s", r.CustomID, text)
		}
		if !strings.Contains(text, body) {
			t.Errorf("request %s lacks the blueprint's text verbatim", r.CustomID)
		}
	}
}

func TestEditSeatSettingsComeFromTheEditSeat(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	seat := h.cfg.Seat["edit"]
	seat.Model, seat.MaxTokens = "model-b", 1234
	h.cfg.Seat["edit"] = seat
	h.cfg.Prices.Models["model-b"] = h.cfg.Prices.Models["model-a"]
	s := h.mustDraft(drafting{narrow: true})
	mustAccept(t, h, s)
	edit := requestsFor(h, s2e, "edit")[0]
	if edit.Model != "model-b" || edit.MaxTokens != 1234 {
		t.Errorf("edit request = model %q, max_tokens %d, want model-b and 1234", edit.Model, edit.MaxTokens)
	}
	if d := requestsFor(h, s2, "draft")[0]; d.Model != "model-a" {
		t.Errorf("draft request model = %q, want model-a", d.Model)
	}
}

func TestDryRunOfANarrowedPageEstimatesTheDraft(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	o := h.options()
	o.Draft, o.DryRun = true, true
	s, err := Run(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Estimates) != 1 || s.Estimates[0].Seat != "draft" || s.EstimateStage != s2 {
		t.Errorf("estimates = %+v for stage %q, want the draft alone", s.Estimates, s.EstimateStage)
	}
}

func (d drafting) narrowed() drafting {
	d.narrow = true
	return d
}

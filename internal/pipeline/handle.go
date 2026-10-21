package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/coverage"
	"github.com/glw907/drafter/internal/edits"
	"github.com/glw907/drafter/internal/run"
)

// handle takes the results of the page's open intent through the failures table and, when every
// seat of the step has an accepted answer, completes the step.
func (p *page) handle(ctx context.Context, in *run.Intent, results []batch.Result) error {
	byID := make(map[string]batch.Result, len(results))
	for _, res := range results {
		byID[res.CustomID] = res
	}
	if p.w.Got == nil {
		p.w.Got = map[string]string{}
	}
	for _, rq := range in.Requests {
		res, present := byID[rq.CustomID]
		if err := p.take(in, rq, res, present); err != nil {
			return err
		}
		if p.ps.Status != StatusRunning || p.r.stop != "" {
			return nil
		}
	}
	if len(p.pending(p.stepSeats())) > 0 {
		return nil
	}
	return p.complete(ctx)
}

// stepSeats names the seats whose answers the current step waits for.
func (p *page) stepSeats() []string {
	switch p.ps.Step {
	case stBpRead:
		if p.runs("consistency") {
			return []string{"consistency", "blueprint"}
		}
		return []string{"blueprint"}
	case stConsistency:
		return []string{"consistency"}
	case stCoverage:
		return []string{"coverage"}
	case stDraft, stGateFix, stRedraft, stFinalRedraft, stSection, stCoherenceFix, stSmall:
		return []string{"draft"}
	case stEdit:
		return []string{"edit"}
	case stCoherence:
		return []string{"coherence"}
	case stRound1:
		var out []string
		for _, s := range reviewSeats {
			if p.runs(s) {
				out = append(out, s)
			}
		}
		return out
	case stRound2:
		return p.round2Seats()
	case stScoped:
		var out []string
		for _, s := range []string{"editor", "fact"} {
			if p.runs(s) {
				out = append(out, s)
			}
		}
		return out
	case stReader1, stReader2:
		return []string{"reader"}
	}
	return nil
}

// round2Seats lists the seats that read again in round 2: those that returned fix in round 1,
// or every seat that read when the chain argument bothReviewers is set.
func (p *page) round2Seats() []string {
	var out []string
	for _, s := range reviewSeats {
		raw, ok := p.w.Round1[s]
		if !ok {
			continue
		}
		if a, err := parseRead(raw); err == nil && (a.Verdict == "fix" || p.bothReviewers()) {
			out = append(out, s)
		}
	}
	return out
}

// noRetry is true for the reads whose stop rule allows no second try.
func (p *page) noRetry() bool {
	switch p.ps.Step {
	case stRound2, stScoped, stReader1, stReader2:
		return true
	}
	return false
}

// isRead reports whether the seat's answer is a read, so a failure to produce one is a fault.
func isRead(seat string) bool {
	return seat != "draft" && seat != "consistency" && seat != "edit"
}

// take applies the failures table to one result.
func (p *page) take(in *run.Intent, rq run.IntentRequest, res batch.Result, present bool) error {
	rec := &p.w.Rec
	outcome := "missing"
	if present {
		outcome = res.Kind
		switch {
		case res.StopReason != "":
			outcome += ":" + res.StopReason
		case res.ErrType != "":
			outcome += ":" + res.ErrType
		}
	}
	rec.Stages = append(rec.Stages, stageRecord{Stage: in.Stage, Seat: rq.Seat, CustomID: rq.CustomID, Outcome: outcome})

	act := batch.ActionInvalid
	if present {
		act = batch.Classify(res)
	}
	switch act {
	case batch.ActionAccept:
		if err := p.validate(rq.Seat, res.Text); err != nil {
			return p.answerFailure(rq.Seat, "did not parse: "+err.Error())
		}
		p.w.Got[rq.Seat] = res.Text
		return nil
	case batch.ActionRetryAnswer:
		return p.answerFailure(rq.Seat, "stopped at "+res.StopReason)
	case batch.ActionInvalid:
		return p.answerFailure(rq.Seat, "returned nothing")
	case batch.ActionResubmit:
		if p.w.spend(p.ps.Step+"."+rq.Seat+".resubmit") > 1 {
			return p.escalate("api error", rq.Seat, nil, res.ErrType+": "+res.ErrMessage)
		}
	case batch.ActionResubmitThenStop:
		if p.w.spend(p.ps.Step+"."+rq.Seat+".cancel") > 1 {
			p.r.halt(StopInfrastructure)
		}
	case batch.ActionRefusal:
		return p.escalate("refusal", rq.Seat, nil, "the model refused")
	case batch.ActionSpendLimit:
		p.r.halt(StopSpendLimit)
	default:
		return p.escalate("api error", rq.Seat, nil, res.ErrType+": "+res.ErrMessage)
	}
	return nil
}

// answerFailure handles an answer that is missing, truncated, or unreadable: one retry, then
// the page escalates as unreadable. The reads whose rule allows no retry escalate at once.
func (p *page) answerFailure(seat, why string) error {
	if isRead(seat) {
		p.w.Rec.fault("the %s read %s", seat, why)
	}
	if p.noRetry() || p.w.spend(p.ps.Step+"."+seat+".answer") > 1 {
		return p.escalate("unreadable "+seat, seat, nil, why)
	}
	return nil
}

type draftAnswer struct {
	Page     string   `json:"page"`
	Brief    string   `json:"brief"`
	Friction []string `json:"friction"`
}

type consistencyAnswer struct {
	Conflicts []struct {
		Facts    []string `json:"facts"`
		With     string   `json:"with"`
		Conflict string   `json:"conflict"`
	} `json:"conflicts"`
}

type readerAnswer struct {
	Paraphrase string   `json:"paraphrase"`
	Summary    string   `json:"summary"`
	Friction   []string `json:"friction"`
}

// validate checks that text is an answer of the shape the current step asked of the seat.
func (p *page) validate(seat, text string) error {
	switch p.ps.Step {
	case stBpRead:
		if seat == "consistency" {
			var a consistencyAnswer
			return decodeAnswer(text, &a)
		}
		_, err := parseRead(text)
		return err
	case stConsistency:
		var a consistencyAnswer
		return decodeAnswer(text, &a)
	case stDraft:
		var a draftAnswer
		if err := decodeAnswer(text, &a); err != nil {
			return err
		}
		if strings.TrimSpace(a.Page) == "" {
			return errors.New("the page is empty")
		}
		if !json.Valid([]byte(a.Brief)) {
			return errors.New("the brief is not JSON")
		}
	case stGateFix, stRedraft, stFinalRedraft, stSection, stCoherenceFix, stSmall, stEdit:
		var a edits.Answer
		return decodeAnswer(text, &a)
	case stCoverage:
		var a struct {
			Items []struct {
				ID    string `json:"id"`
				Quote string `json:"quote"`
			} `json:"items"`
		}
		return decodeAnswer(text, &a)
	case stReader1:
		var a readerAnswer
		return decodeAnswer(text, &a)
	default:
		_, err := parseRead(text)
		return err
	}
	return nil
}

// complete runs the step once every seat has an accepted answer. The one-retry budgets of the
// failures table are per step, so they start over for the next one.
func (p *page) complete(ctx context.Context) error {
	for row := range p.w.Budgets {
		if strings.HasSuffix(row, ".answer") || strings.HasSuffix(row, ".resubmit") || strings.HasSuffix(row, ".cancel") {
			delete(p.w.Budgets, row)
		}
	}
	switch p.ps.Step {
	case stBpRead:
		return p.completeBlueprintRead()
	case stConsistency:
		return p.completeConsistency()
	case stDraft:
		return p.completeDraft(ctx)
	case stGateFix, stRedraft, stFinalRedraft, stSection, stCoherenceFix, stSmall, stEdit:
		return p.completeExchange(ctx)
	case stCoherence:
		return p.completeCoherence()
	case stCoverage:
		return p.completeCoverage()
	case stRound1:
		return p.completeRound1()
	case stRound2:
		p.recordReads(s6)
		return p.afterRound2(p.round2Seats())
	case stScoped:
		p.recordReads(s7)
		return p.afterScoped(p.stepSeats())
	case stReader1:
		var a readerAnswer
		_ = decodeAnswer(p.w.Got["reader"], &a)
		p.w.Rec.addFriction("reader", a.Friction)
		p.w.Reader.History = append(slices.Clone(p.w.Sent["reader"]), batch.Message{Role: "assistant", Text: p.w.Got["reader"]})
		p.w.Got = nil
		p.ps.Step = stReader2
		return nil
	case stReader2:
		return p.completeReader()
	}
	return fmt.Errorf("page %s completed unknown step %q", p.h.Page.Path, p.ps.Step)
}

func (p *page) recordReads(stage string) {
	for _, seat := range p.stepSeats() {
		if a, err := parseRead(p.w.Got[seat]); err == nil {
			p.w.Rec.verdict(stage, seat, a.Verdict)
			p.w.Rec.addFriction(seat, a.Friction)
		}
	}
}

func (p *page) completeConsistency() error {
	var a consistencyAnswer
	if err := decodeAnswer(p.w.Got["consistency"], &a); err != nil {
		return err
	}
	if len(a.Conflicts) == 0 {
		p.w.Rec.verdict(s1, "consistency", "ok")
		p.w.Got = nil
		p.ps.Step = stDraft
		return nil
	}
	p.w.Rec.verdict(s1, "consistency", "conflicts")
	var parts []string
	for _, c := range a.Conflicts {
		parts = append(parts, fmt.Sprintf("%s conflicts with %s: %s", strings.Join(c.Facts, ", "), c.With, c.Conflict))
	}
	return p.escalate("plan", "consistency", nil, strings.Join(parts, "; "))
}

func (p *page) completeDraft(ctx context.Context) error {
	var a draftAnswer
	if err := decodeAnswer(p.w.Got["draft"], &a); err != nil {
		return err
	}
	p.w.Rec.addFriction("draft", a.Friction)
	p.w.DraftHist = append(slices.Clone(p.w.Sent["draft"]), batch.Message{Role: "assistant", Text: p.w.Got["draft"]})
	p.w.Got = nil
	p.w.PendingGate = exDraft
	if err := p.writeFiles(a.Page, a.Brief, nil); err != nil {
		return err
	}
	return p.gateStep(ctx, exDraft)
}

// completeCoverage checks the coverage read's quotes against the plan's sections.
func (p *page) completeCoverage() error {
	doc, err := p.planDoc()
	if err != nil {
		return err
	}
	pageText, _, err := p.current()
	if err != nil {
		return err
	}
	misses, faults, err := coverage.Verify(doc, []byte(pageText), []byte(p.w.Got["coverage"]))
	if err != nil {
		return p.answerFailure("coverage", err.Error())
	}
	for _, f := range faults {
		p.w.Rec.fault("%s", f)
	}
	for _, m := range misses {
		p.w.Cov.Misses = append(p.w.Cov.Misses, missRecord{Item: m.Item, Section: m.Section, Kind: m.Kind, Reason: m.Reason})
	}
	p.w.Cov.ReadIn = true
	p.w.Got = nil
	return nil
}

func (p *page) completeRound1() error {
	p.w.Round1 = map[string]string{}
	maps.Copy(p.w.Round1, p.w.Got)
	p.recordReads(s4)
	return p.afterRound1()
}

func (p *page) completeReader() error {
	a, err := parseRead(p.w.Got["reader"])
	if err != nil {
		return err
	}
	p.w.Rec.verdict(s7, "reader", a.Verdict)
	p.w.Rec.addFriction("reader", a.Friction)
	if a.Verdict == "ok" {
		return p.reviewed()
	}
	fs := a.findings("reader", 0)
	p.w.Rec.findings(s7, fs)
	if p.w.Reader.Retest {
		var ns []int
		for _, f := range fs {
			ns = append(ns, f.N)
		}
		return p.escalate("fix after the reader redraft", "reader", ns, "the retest returned fix")
	}
	p.w.Cited = edits.Render(fs)
	p.w.Ex = &exchange{Kind: exFinal, Findings: fs}
	p.w.Got = nil
	p.ps.Step = stFinalRedraft
	return nil
}

// reviewed ends a page whose review has passed. A section redraft goes on to the coherence read,
// which reads the whole page once its changed sections are in; any other page accepts.
func (p *page) reviewed() error {
	if !p.w.Section {
		return p.accept()
	}
	p.w.Got = nil
	p.ps.Step = stCoherence
	return nil
}

// completeCoherence acts on the coherence read. An ok accepts the page. A first fix opens an
// exchange on the read's findings, with no region bound, since a finding may lie anywhere; a fix on
// the retest after that exchange escalates.
func (p *page) completeCoherence() error {
	a, err := parseRead(p.w.Got["coherence"])
	if err != nil {
		return err
	}
	p.w.Rec.verdict(s8, "coherence", a.Verdict)
	p.w.Rec.addFriction("coherence", a.Friction)
	if a.Verdict == "ok" {
		return p.accept()
	}
	fs := a.findings("coherence", 0)
	p.w.Rec.findings(s8, fs)
	if p.w.CoherenceRetest {
		var ns []int
		for _, f := range fs {
			ns = append(ns, f.N)
		}
		return p.escalate("coherence", "coherence", ns, "the coherence retest returned fix")
	}
	p.w.Ex = &exchange{Kind: exCoherence, Findings: fs}
	p.w.Got = nil
	p.ps.Step = stCoherenceFix
	return nil
}

// completeExchange applies an edit-contract answer, or sends its refusal back.
func (p *page) completeExchange(ctx context.Context) error {
	ex := p.w.Ex
	text := p.w.Got[ex.seat()]
	var ans edits.Answer
	if err := decodeAnswer(text, &ans); err != nil {
		return err
	}
	ex.History = append(slices.Clone(p.w.Sent[ex.seat()]), batch.Message{Role: "assistant", Text: text})
	p.w.Got = nil
	pageText, briefText, err := p.current()
	if err != nil {
		return err
	}
	b, err := p.bound(ex)
	if err != nil {
		return err
	}
	ap, err := edits.Apply(pageText, briefText, ex.Findings, ex.Resolutions, ans, b)
	if ref, ok := errors.AsType[*edits.Refusal](err); ok {
		return p.refused(ex, ref, briefText)
	} else if err != nil {
		return err
	}
	ex.Resolutions = ap.Resolutions
	p.w.Rec.resolutions(exchangeStage(ex.Kind), ex.Resolutions)
	if ap.Page != pageText || ap.Brief != briefText {
		p.w.PendingGate = ex.Kind
		if err := p.writeFiles(ap.Page, ap.Brief, ex); err != nil {
			return err
		}
	}
	return p.gateStep(ctx, ex.Kind)
}

// refused answers an edit-contract refusal: the problems go back as the next turn, inside the
// shared budget.
func (p *page) refused(ex *exchange, ref *edits.Refusal, briefBefore string) error {
	for _, prob := range ref.Problems {
		if strings.Contains(prob, "brief is not valid JSON") && !json.Valid([]byte(briefBefore)) {
			p.w.Rec.fault("the brief was not valid JSON before the answer's edits, so the refusal %q is not the answer's doing", prob)
		}
	}
	if p.w.spend(ex.Kind+".contract") > p.r.cfg.Retries.EditContract {
		return p.escalate("contract", ex.seat(), nil, strings.Join(ref.Problems, "; "))
	}
	var b strings.Builder
	b.WriteString("Your answer was refused, and nothing in it was applied. The problems:\n")
	for _, prob := range ref.Problems {
		b.WriteString("- " + prob + "\n")
	}
	b.WriteString("\nAnswer again with exact edits and a resolution for every finding that has none yet.")
	ex.Turn = b.String()
	return nil
}

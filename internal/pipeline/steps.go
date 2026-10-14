package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/glw907/drafter/internal/assemble"
	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/coverage"
	"github.com/glw907/drafter/internal/edits"
	"github.com/glw907/drafter/internal/prompts"
)

// stage is what a page asks of the API next: one or more requests that belong together.
type stage struct {
	page *page
	name string // the stage token in the custom_ids
	reqs []batch.Request
}

// reviewSeats are the seats that read a page in rounds 1 and 2, in the order their findings are
// numbered.
var reviewSeats = []string{"structure", "editor", "fact"}

func (p *page) hash12() string {
	sum := sha256.Sum256([]byte(p.key))
	return hex.EncodeToString(sum[:])[:12]
}

// nextAttempt advances and returns the submission counter for the stage and seat.
func (p *page) nextAttempt(stage, seat string) int {
	if p.ps.Attempts == nil {
		p.ps.Attempts = map[string]int{}
	}
	k := stage + "." + seat
	p.ps.Attempts[k]++
	return p.ps.Attempts[k]
}

func (p *page) customID(stage, seat string, attempt int) string {
	return fmt.Sprintf("%s_%s_%s_%s_%d", p.r.suffix, p.hash12(), stage, seat, attempt)
}

// pageJSON returns the page's resolved entry from the handoff's context, or a minimal entry.
func (p *page) pageJSON() json.RawMessage {
	var c struct {
		Resolved struct {
			Pages []json.RawMessage `json:"pages"`
		} `json:"resolved"`
	}
	if len(p.h.Context) > 0 && json.Unmarshal(p.h.Context, &c) == nil {
		for _, raw := range c.Resolved.Pages {
			var e struct {
				Path string `json:"path"`
			}
			if json.Unmarshal(raw, &e) == nil && e.Path == p.h.Page.Path {
				return raw
			}
		}
	}
	return marshalRaw(map[string]string{
		"path": p.h.Page.Path, "brief": p.h.Page.Brief, "type": p.h.Page.Type, "job": p.h.Inputs.Job,
	})
}

// bothReviewers reports the chain argument that sends every reviewer to round 2.
func (p *page) bothReviewers() bool {
	var c struct {
		Args struct {
			BothReviewers bool `json:"bothReviewers"`
		} `json:"args"`
	}
	return len(p.h.Context) > 0 && json.Unmarshal(p.h.Context, &c) == nil && c.Args.BothReviewers
}

func (p *page) context(round prompts.Round) prompts.Context {
	return prompts.Context{
		Page: p.pageJSON(), Inputs: marshalRaw(p.h.Inputs), Round: round, Facts: p.r.cfg.Facts.Dir != "",
	}
}

func (p *page) render(seat string, c prompts.Context) (string, error) {
	out, err := p.mod.Render(seat, c)
	if err != nil {
		return "", fmt.Errorf("render the %s prompt for %s: %w", seat, p.h.Page.Path, err)
	}
	return out, nil
}

// baselineFor returns the round-1 snapshot's ID for the requests that read a diff from it. Only
// round 2 narrows to the redraft's changes, so no other request carries one.
func (p *page) baselineFor(stage string) string {
	if stage == s6 {
		return p.w.Baseline
	}
	return ""
}

// build assembles one request and remembers its turns, which an answer extends into history.
func (p *page) build(ctx context.Context, stage, seat string, a assemble.Answer, prompt string, hist []batch.Message, declined []edits.DeclinedFinding) (batch.Request, error) {
	attempt := p.nextAttempt(stage, seat)
	req, err := assemble.Build(ctx, assemble.Input{
		Seat: seat, Answer: a, Config: p.r.cfg, Prompt: prompt, History: hist,
		Page: p.h.Page.Path, Brief: p.h.Page.Brief, Slug: p.slug(), Baseline: p.baselineFor(stage),
		Facts: p.r.facts, FactIDs: p.h.Inputs.FactIDs, Declined: declined,
		CustomID: p.customID(stage, seat, attempt),
	})
	if err != nil {
		return batch.Request{}, fmt.Errorf("assemble the %s request for %s: %w", seat, p.h.Page.Path, err)
	}
	if p.w.Sent == nil {
		p.w.Sent = map[string][]batch.Message{}
	}
	p.w.Sent[seat] = req.Messages
	return req, nil
}

// advance does the page's local work and returns the next stage it needs, or nil when the page
// has ended, holds, or the run has stopped.
func (p *page) advance(ctx context.Context) (*stage, error) {
	for {
		if p.ps.Status != StatusRunning || p.r.stop != "" {
			return nil, nil
		}
		var stg *stage
		var err error
		switch p.ps.Step {
		case stPlan:
			err = p.doPlan()
		case stConsistency:
			stg, err = p.stageConsistency(ctx)
		case stDraft:
			stg, err = p.stageDraft(ctx)
		case stGateFix, stRedraft, stFinalRedraft:
			stg, err = p.stageExchange(ctx)
		case stCoverage:
			stg, err = p.doCoverage(ctx)
		case stRound1:
			stg, err = p.stageRound1(ctx)
		case stRound2:
			stg, err = p.stageRound2(ctx)
		case stReader1, stReader2:
			stg, err = p.stageReader(ctx)
		case stScoped:
			stg, err = p.stageScoped(ctx)
		default:
			return nil, fmt.Errorf("page %s is at unknown step %q", p.h.Page.Path, p.ps.Step)
		}
		if err != nil || stg != nil {
			return stg, err
		}
	}
}

// pending returns the seats of seats that have no accepted answer yet.
func (p *page) pending(seats []string) []string {
	var out []string
	for _, s := range seats {
		if _, ok := p.w.Got[s]; !ok {
			out = append(out, s)
		}
	}
	return out
}

func (p *page) stageConsistency(ctx context.Context) (*stage, error) {
	doc, err := p.planDoc()
	if err != nil {
		return nil, err
	}
	prompt, err := assemble.ConsistencyPrompt(doc, p.r.facts, p.r.root)
	if err != nil {
		return nil, err
	}
	req, err := p.build(ctx, s1, "consistency", assemble.AnswerConsistency, prompt, nil, nil)
	if err != nil {
		return nil, err
	}
	return &stage{page: p, name: s1, reqs: []batch.Request{req}}, nil
}

func (p *page) stageDraft(ctx context.Context) (*stage, error) {
	prompt, err := p.render("draft", p.context(prompts.Round1))
	if err != nil {
		return nil, err
	}
	req, err := p.build(ctx, s2, "draft", assemble.AnswerDraft, prompt, nil, nil)
	if err != nil {
		return nil, err
	}
	return &stage{page: p, name: s2, reqs: []batch.Request{req}}, nil
}

// exchangeStage names the stage token of an edit exchange.
func exchangeStage(kind string) string {
	switch kind {
	case exRedraft:
		return s5
	case exFinal:
		return s7
	}
	return s2
}

func (p *page) stageExchange(ctx context.Context) (*stage, error) {
	ex := p.w.Ex
	prompt := ex.Turn
	if prompt == "" {
		c := p.context(prompts.Round2)
		c.Findings = edits.Render(ex.Findings)
		if ex.Kind == exFinal {
			c.Round, c.AfterFinalRead = prompts.RoundFinal, true
		}
		var err error
		if prompt, err = p.render("draft", c); err != nil {
			return nil, err
		}
	}
	name := exchangeStage(ex.Kind)
	req, err := p.build(ctx, name, "draft", assemble.AnswerEdits, prompt, ex.History, nil)
	if err != nil {
		return nil, err
	}
	return &stage{page: p, name: name, reqs: []batch.Request{req}}, nil
}

// doCoverage runs the mechanical checks and, when the plan has claims to read, the coverage read.
func (p *page) doCoverage(ctx context.Context) (*stage, error) {
	cov := &p.w.Cov
	if p.markdown {
		return nil, p.afterCoverage()
	}
	doc, err := p.planDoc()
	if err != nil {
		return nil, err
	}
	pageText, briefText, err := p.current()
	if err != nil {
		return nil, err
	}
	if !cov.MechDone {
		misses, err := coverage.Check(doc, []byte(pageText), []byte(briefText), p.r.facts != nil)
		if err != nil {
			return nil, fmt.Errorf("check coverage of %s: %w", p.h.Page.Path, err)
		}
		for _, m := range misses {
			cov.Misses = append(cov.Misses, missRecord{Item: m.Item, Section: m.Section, Kind: m.Kind, Reason: m.Reason})
		}
		cov.MechDone = true
	}
	if !cov.ReadIn && p.runs("coverage") && hasReadItems(doc) {
		name := coverageStage(cov.Phase)
		req, err := p.build(ctx, name, "coverage", assemble.AnswerCoverage, coverage.Prompt(doc, []byte(pageText)), nil, nil)
		if err != nil {
			return nil, err
		}
		return &stage{page: p, name: name, reqs: []batch.Request{req}}, nil
	}
	return nil, p.afterCoverage()
}

func coverageStage(phase string) string {
	switch phase {
	case covRedraft:
		return s5
	case covFinal:
		return s7
	}
	return s3
}

// The phases of a coverage check.
const (
	covInitial = "initial"
	covRedraft = "redraft"
	covFinal   = "final"
)

// toCoverage moves the page to a fresh coverage check.
func (p *page) toCoverage(phase string) {
	p.w.Cov = coverageState{Phase: phase}
	p.w.Got = nil
	p.ps.Step = stCoverage
}

// afterCoverage acts on a finished coverage check.
func (p *page) afterCoverage() error {
	cov := p.w.Cov
	switch cov.Phase {
	case covInitial:
		p.w.CovFindings = nil
		for _, m := range cov.Misses {
			p.w.CovFindings = append(p.w.CovFindings, missFinding(m))
		}
		p.w.Got = nil
		p.ps.Step = stRound1
		return nil
	case covRedraft:
		if len(cov.Misses) > 0 {
			return p.escalate("second fix verdict", "coverage", nil, missSummary(cov.Misses))
		}
		p.w.Got = nil
		p.ps.Step = stRound2
	default:
		if len(cov.Misses) > 0 {
			return p.escalate("fix after the reader redraft", "coverage", nil, missSummary(cov.Misses))
		}
		p.w.Got = nil
		p.ps.Step = stScoped
	}
	return nil
}

func missFinding(m missRecord) edits.Finding {
	loc := m.Section
	if loc == "" {
		loc = "the brief"
	}
	return edits.Finding{
		Seat: "coverage", Location: loc, Blocking: true,
		Text:    fmt.Sprintf("plan item %s (%s) is not carried: %s", m.Item, m.Kind, m.Reason),
		Rewrite: "Add what the plan item asks for in the section named.",
	}
}

func missSummary(ms []missRecord) string {
	var parts []string
	for _, m := range ms {
		parts = append(parts, m.Item+": "+m.Reason)
	}
	return strings.Join(parts, "; ")
}

// stageRound1 takes the round-1 snapshot, then asks each review seat that has no answer yet.
func (p *page) stageRound1(ctx context.Context) (*stage, error) {
	if p.w.Baseline == "" {
		if err := p.snapshotRound1(); err != nil {
			return nil, err
		}
	}
	var seats []string
	for _, s := range reviewSeats {
		if p.runs(s) {
			seats = append(seats, s)
		}
	}
	if len(seats) == 0 {
		return nil, p.afterRound1()
	}
	todo := p.pending(seats)
	if len(todo) == 0 {
		return nil, p.afterRound1()
	}
	var reqs []batch.Request
	for _, seat := range todo {
		prompt, err := p.render(seat, p.context(prompts.Round1))
		if err != nil {
			return nil, err
		}
		req, err := p.build(ctx, s4, seat, assemble.AnswerRead, prompt, nil, nil)
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, req)
	}
	return &stage{page: p, name: s4, reqs: reqs}, nil
}

// snapshotRound1 keeps the page as round 1 reads it, in the run directory and the object
// database, so a narrowed read can diff against it.
func (p *page) snapshotRound1() error {
	text, _, err := p.readFile(p.h.Page.Path)
	if err != nil {
		return err
	}
	if p.dir == "" {
		return nil
	}
	snap := filepath.Join(p.dir, "round1.page.md")
	if err := os.WriteFile(snap, []byte(text), 0o644); err != nil {
		return fmt.Errorf("write round-1 snapshot: %w", err)
	}
	if p.w.Baseline, err = p.gitHashObject(snap); err != nil {
		return err
	}
	return nil
}

// readAnswer is a review seat's or the final reader's verdict.
type readAnswer struct {
	Verdict  string `json:"verdict"`
	Findings []struct {
		Location string `json:"location"`
		Finding  string `json:"finding"`
		Rewrite  string `json:"rewrite"`
		Blocking bool   `json:"blocking"`
	} `json:"findings"`
	Summary  string   `json:"summary"`
	Friction []string `json:"friction"`
}

func parseRead(text string) (readAnswer, error) {
	var a readAnswer
	if err := decodeAnswer(text, &a); err != nil {
		return a, err
	}
	if a.Verdict != "ok" && a.Verdict != "fix" {
		return a, fmt.Errorf("verdict %q is neither ok nor fix", a.Verdict)
	}
	return a, nil
}

// decodeAnswer decodes an answer strictly: one JSON value with no unknown field.
func decodeAnswer(text string, v any) error {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("data after the JSON value")
	}
	return nil
}

func (a readAnswer) findings(seat string, n int) []edits.Finding {
	var out []edits.Finding
	for _, f := range a.Findings {
		n++
		out = append(out, edits.Finding{N: n, Seat: seat, Location: f.Location, Text: f.Finding, Rewrite: f.Rewrite, Blocking: f.Blocking})
	}
	return out
}

// afterRound1 decides between the redraft and the final reader.
func (p *page) afterRound1() error {
	var fs []edits.Finding
	anyFix := false
	for _, seat := range reviewSeats {
		raw, ok := p.w.Round1[seat]
		if !ok {
			continue
		}
		a, err := parseRead(raw)
		if err != nil {
			return err
		}
		if a.Verdict == "fix" {
			anyFix = true
			fs = append(fs, a.findings(seat, len(fs))...)
		}
	}
	if !anyFix && len(p.w.CovFindings) == 0 {
		p.w.Got = nil
		p.ps.Step = stReader1
		return nil
	}
	for _, f := range p.w.CovFindings {
		f.N = len(fs) + 1
		fs = append(fs, f)
	}
	p.w.Ex = &exchange{Kind: exRedraft, Findings: fs}
	p.w.Rec.findings(s5, fs)
	p.w.Got = nil
	p.ps.Step = stRedraft
	return nil
}

// stageRound2 asks the seats that read in round 1 again, narrowed to the redraft's changes.
func (p *page) stageRound2(ctx context.Context) (*stage, error) {
	var seats []string
	for _, s := range reviewSeats {
		raw, ok := p.w.Round1[s]
		if !ok {
			continue
		}
		a, err := parseRead(raw)
		if err != nil {
			return nil, err
		}
		if a.Verdict == "fix" || p.bothReviewers() {
			seats = append(seats, s)
		}
	}
	todo := p.pending(seats)
	if len(todo) == 0 {
		return nil, p.afterRound2(seats)
	}
	var reqs []batch.Request
	for _, seat := range todo {
		c := p.context(prompts.Round2)
		c.Narrow = marshalRaw(map[string]json.RawMessage{
			"baseline": marshalRaw(p.w.Baseline), "own": json.RawMessage(p.w.Round1[seat]),
		})
		prompt, err := p.render(seat, c)
		if err != nil {
			return nil, err
		}
		req, err := p.build(ctx, s6, seat, assemble.AnswerRead, prompt, nil, p.declinedFor(seat))
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, req)
	}
	return &stage{page: p, name: s6, reqs: reqs}, nil
}

// declinedFor returns the blocking findings of seat that the redraft declined.
func (p *page) declinedFor(seat string) []edits.DeclinedFinding {
	if p.w.Ex == nil {
		return nil
	}
	var out []edits.DeclinedFinding
	for _, d := range edits.Declined(p.w.Ex.Findings, p.w.Ex.Resolutions) {
		if d.Finding.Seat == seat {
			out = append(out, d)
		}
	}
	return out
}

func (p *page) afterRound2(seats []string) error {
	for _, seat := range seats {
		a, err := parseRead(p.w.Got[seat])
		if err != nil {
			return err
		}
		if a.Verdict == "fix" {
			var ns []int
			for i := range a.Findings {
				ns = append(ns, i+1)
			}
			p.w.Rec.findings(s6, a.findings(seat, 0))
			return p.escalate("second fix verdict", seat, ns, "")
		}
	}
	p.w.Got = nil
	p.ps.Step = stReader1
	return nil
}

// stageReader asks the final reader's first turn, or its second turn with the job.
func (p *page) stageReader(ctx context.Context) (*stage, error) {
	if !p.runs("reader") {
		return nil, p.accept()
	}
	round := prompts.Round1
	if p.w.Reader.Retest {
		round = prompts.RoundFinal
	}
	c := p.context(round)
	var req batch.Request
	var err error
	if p.ps.Step == stReader1 {
		c.Turn = 1
		var prompt string
		if prompt, err = p.render("reader", c); err != nil {
			return nil, err
		}
		req, err = p.build(ctx, s7, "reader", assemble.AnswerReader1, prompt, nil, nil)
	} else {
		c.Turn = 2
		var prompt string
		if prompt, err = p.render("reader", c); err != nil {
			return nil, err
		}
		req, err = p.build(ctx, s7, "reader", assemble.AnswerReader2, prompt, p.w.Reader.History, nil)
	}
	if err != nil {
		return nil, err
	}
	return &stage{page: p, name: s7, reqs: []batch.Request{req}}, nil
}

// stageScoped asks the register editor and the fact read to re-read the final redraft's
// changes, each told the findings the redraft answered.
func (p *page) stageScoped(ctx context.Context) (*stage, error) {
	var seats []string
	for _, s := range []string{"editor", "fact"} {
		if p.runs(s) {
			seats = append(seats, s)
		}
	}
	todo := p.pending(seats)
	if len(todo) == 0 {
		return nil, p.afterScoped(seats)
	}
	var reqs []batch.Request
	for _, seat := range todo {
		c := p.context(prompts.RoundFinal)
		c.Scope = p.w.Cited
		prompt, err := p.render(seat, c)
		if err != nil {
			return nil, err
		}
		req, err := p.build(ctx, s7, seat, assemble.AnswerRead, prompt, nil, p.declinedFor(seat))
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, req)
	}
	return &stage{page: p, name: s7, reqs: reqs}, nil
}

func (p *page) afterScoped(seats []string) error {
	for _, seat := range seats {
		a, err := parseRead(p.w.Got[seat])
		if err != nil {
			return err
		}
		if a.Verdict == "fix" {
			var ns []int
			for i := range a.Findings {
				ns = append(ns, i+1)
			}
			p.w.Rec.findings(s7, a.findings(seat, 0))
			return p.escalate("fix after the reader redraft", seat, ns, "")
		}
	}
	p.w.Got = nil
	p.w.Reader = readerState{Retest: true}
	p.ps.Step = stReader1
	return nil
}

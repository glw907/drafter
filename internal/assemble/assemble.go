// Package assemble turns a rendered seat prompt into a complete API request: the seat's model
// and system prompt, the files, command outputs, and source excerpts its inputs name, and the
// schema its answer must match.
package assemble

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/edits"
	"github.com/glw907/drafter/internal/plan"
)

// Answer names which answer a request asks for.
type Answer string

// The answers a request asks for.
const (
	AnswerDraft       Answer = "draft"
	AnswerEdits       Answer = "edits"
	AnswerRead        Answer = "read"
	AnswerReader1     Answer = "reader1"
	AnswerReader2     Answer = "reader2"
	AnswerCoverage    Answer = "coverage"
	AnswerConsistency Answer = "consistency"
)

// Input is what Build assembles a request from.
//
// Prompt is the prompts module's rendering, or drafter's own text for a continuation, and
// History holds the turns before it. Page and Brief are repo-relative paths. Facts is nil for a
// project without a facts store.
type Input struct {
	Seat     string
	Answer   Answer
	Config   *config.Config
	Prompt   string
	History  []batch.Message
	Page     string
	Brief    string
	Slug     string
	Baseline string
	Facts    *plan.Facts
	FactIDs  []string
	Declined []edits.DeclinedFinding
	CustomID string
}

// note opens the first user turn of a request. The delimiters it states are the ones the block
// writers in this package use.
const note = `Files and command outputs your task needs are inlined below, each between a line "=== file: PATH ===" and a line "=== end file: PATH ===" (commands and source excerpts use the same shape with "command" and "excerpt"). A block that reads "not inlined" or "no excerpt" was left out on purpose. Do not run a command whose output is not inlined. If you would need to make an edit, or you could not make a check because something is not inlined, say so as a finding.`

// Build assembles the request for in: the seat's model, effort, token limit, and system prompt
// (the body of its agent definition, empty when it has none), the answer's schema, and a user turn
// holding the prompt, the inlined files, command outputs, and excerpts, any declined findings,
// and the answer's contract. Commands run under ctx with a time limit each and the environment
// gate.Env returns.
func Build(ctx context.Context, in Input) (batch.Request, error) {
	seat, ok := in.Config.Seat[in.Seat]
	if !ok {
		return batch.Request{}, fmt.Errorf("assemble: seat %q is not configured", in.Seat)
	}
	schema, contract, err := answerSchema(in.Answer)
	if err != nil {
		return batch.Request{}, err
	}
	system, err := systemPrompt(seat.Agent)
	if err != nil {
		return batch.Request{}, err
	}

	var parts []string
	first := len(in.History) == 0
	if first && in.Answer != AnswerCoverage && in.Answer != AnswerConsistency {
		parts = append(parts, note)
	}
	parts = append(parts, in.Prompt)
	blocks, err := inputBlocks(ctx, in, seat)
	if err != nil {
		return batch.Request{}, err
	}
	parts = append(parts, blocks...)
	if len(in.Declined) > 0 {
		parts = append(parts, declinedBlock(in.Declined))
	}
	parts = append(parts, contract)

	msgs := append([]batch.Message(nil), in.History...)
	msgs = append(msgs, batch.Message{Role: "user", Text: strings.Join(parts, "\n\n")})
	return batch.Request{
		CustomID:  in.CustomID,
		Model:     seat.Model,
		System:    system,
		Messages:  msgs,
		MaxTokens: seat.MaxTokens,
		Effort:    seat.Effort,
		Schema:    schema,
	}, nil
}

// inputBlocks returns the blocks the answer's request inlines after the prompt.
func inputBlocks(ctx context.Context, in Input, seat config.Seat) ([]string, error) {
	switch in.Answer {
	case AnswerReader2, AnswerCoverage, AnswerConsistency:
		return nil, nil
	}
	r, err := openRepo(ctx, in.Config)
	if err != nil {
		return nil, err
	}
	switch in.Answer {
	case AnswerReader1:
		return r.pageBlocks(in.Page)
	case AnswerEdits:
		return r.pageBlocks(in.Page, in.Brief)
	}

	texts := []string{in.Prompt}
	if in.Facts == nil && in.Seat == "fact" {
		for _, p := range []string{in.Page, in.Brief} {
			data, err := os.ReadFile(filepath.Join(r.root, p))
			if err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("assemble: read %s: %w", p, err)
			}
			texts = append(texts, string(data))
		}
	}
	blocks := r.fileBlocks(texts...)
	cmds, err := commandBlocks(ctx, in, seat)
	if err != nil {
		return nil, err
	}
	blocks = append(blocks, cmds...)
	if in.Facts != nil {
		ex, err := r.excerptBlocks(in.Facts, in.FactIDs)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, ex...)
	}
	return blocks, nil
}

func declinedBlock(ds []edits.DeclinedFinding) string {
	var b strings.Builder
	b.WriteString("Earlier findings you raised as blocking were declined by the drafter, each with a reason. For each one, accept the decline, or raise the finding again.\n")
	for _, d := range ds {
		f := d.Finding
		fmt.Fprintf(&b, "\nDeclined finding %d (%s): %s\nReason declined: %s\n", f.N, f.Location, f.Text, d.Reason)
	}
	return strings.TrimRight(b.String(), "\n")
}

// systemPrompt returns the body of the agent definition at path, without its front matter, or
// the empty string when path is empty.
func systemPrompt(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("assemble: read agent definition: %w", err)
	}
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	if rest, ok := strings.CutPrefix(s, "---\n"); ok {
		switch {
		case strings.HasPrefix(rest, "---\n"):
			s = rest[4:]
		default:
			i := strings.Index(rest, "\n---\n")
			switch {
			case i >= 0:
				s = rest[i+5:]
			case strings.HasSuffix(rest, "\n---"):
				s = ""
			}
		}
	}
	return strings.TrimSpace(s), nil
}

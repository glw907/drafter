// Package assemble turns a rendered seat prompt into a complete API request: the seat's model
// and system prompt, the files, command outputs, and source excerpts its inputs name, and the
// schema its answer must match.
package assemble

import (
	"context"
	"errors"

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

// Build assembles the request for in.
func Build(ctx context.Context, in Input) (batch.Request, error) {
	return batch.Request{}, errors.New("assemble: not implemented")
}

// ConsistencyPrompt builds the consistency read's prompt for p.
func ConsistencyPrompt(p *plan.Plan, facts *plan.Facts, root string) (string, error) {
	return "", errors.New("assemble: not implemented")
}

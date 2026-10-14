// Package pipeline runs each planned page through drafter's stages: the plan check, the draft,
// the coverage check, the review seats, the redraft, and the final reader. It owns the stop
// rules and the money path, and starts no goroutines.
package pipeline

import (
	"context"
	"time"

	"github.com/glw907/drafter/internal/batch"
)

// API is what the pipeline needs from the Claude API client. *batch.Client implements it.
type API interface {
	CheckWorkspace(ctx context.Context) error
	CountTokens(ctx context.Context, r batch.Request) (int64, error)
	Create(ctx context.Context, rs []batch.Request) (batch.Batch, error)
	Get(ctx context.Context, id string) (batch.Batch, error)
	Results(ctx context.Context, id string) ([]batch.Result, error)
	List(ctx context.Context, since time.Time) ([]batch.Batch, error)
	Send(ctx context.Context, r batch.Request) (batch.Result, error)
}

var _ API = (*batch.Client)(nil)

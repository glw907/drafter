package batch

import (
	"context"
	"errors"
	"time"
)

var errTodo = errors.New("not implemented")

// Client talks to the Claude API.
type Client struct {
	// Sleep waits d or until ctx ends; tests replace it.
	Sleep func(ctx context.Context, d time.Duration) error
	// Now is the backoff clock; tests replace it.
	Now func() time.Time

	checked bool
}

// New returns a Client for o.
func New(o Options) *Client { return &Client{} }

// CheckWorkspace verifies the workspace the key resolves to.
func (c *Client) CheckWorkspace(ctx context.Context) error { return errTodo }

// CountTokens counts the input tokens of r.
func (c *Client) CountTokens(ctx context.Context, r Request) (int64, error) { return 0, errTodo }

// Create submits one batch.
func (c *Client) Create(ctx context.Context, rs []Request) (Batch, error) { return Batch{}, errTodo }

// Get returns one batch.
func (c *Client) Get(ctx context.Context, id string) (Batch, error) { return Batch{}, errTodo }

// Results returns the results of an ended batch.
func (c *Client) Results(ctx context.Context, id string) ([]Result, error) { return nil, errTodo }

// List returns the batches created at or after since, less a margin.
func (c *Client) List(ctx context.Context, since time.Time) ([]Batch, error) { return nil, errTodo }

// Send makes one streamed Messages call.
func (c *Client) Send(ctx context.Context, r Request) (Result, error) { return Result{}, errTodo }

// Classify maps a Result to an Action.
func Classify(r Result) Action { return ActionInvalid }

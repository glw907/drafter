package batch

import (
	"context"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// listMargin absorbs a local clock that runs ahead of the API's, so List returns a batch
// created up to this long before its since argument.
const listMargin = 10 * time.Minute

// CountTokens counts the input tokens of r, system prompt, schema, and effort included, so a
// worst case never undercounts what Create or Send would send.
func (c *Client) CountTokens(ctx context.Context, r Request) (int64, error) {
	if err := c.requireCheck(); err != nil {
		return 0, err
	}
	w, err := buildWire(r)
	if err != nil {
		return 0, err
	}
	var n int64
	f, err := c.do(ctx, true, func(ctx context.Context) error {
		res, err := c.sdk.Messages.CountTokens(ctx, w.countParams(), option.WithRequestTimeout(c.o.RequestTimeout))
		if err != nil {
			return err
		}
		n = res.InputTokens
		return nil
	})
	if err != nil {
		return 0, err
	}
	if f != nil {
		return 0, f.plain()
	}
	return n, nil
}

// Create submits rs as one batch. A failure after the request was written is ErrMaybeSent:
// the batch may exist, so the caller adopts through List before resubmitting.
func (c *Client) Create(ctx context.Context, rs []Request) (Batch, error) {
	if err := c.requireCheck(); err != nil {
		return Batch{}, err
	}
	params := anthropic.MessageBatchNewParams{}
	for _, r := range rs {
		w, err := buildWire(r)
		if err != nil {
			return Batch{}, err
		}
		params.Requests = append(params.Requests, w.batchParams(r.CustomID, r.MaxTokens))
	}
	var out Batch
	f, err := c.do(ctx, false, func(ctx context.Context) error {
		b, err := c.sdk.Messages.Batches.New(ctx, params, option.WithRequestTimeout(c.o.RequestTimeout))
		if err != nil {
			return err
		}
		out = batchOf(*b)
		return nil
	})
	if err != nil {
		return Batch{}, err
	}
	if f != nil {
		return Batch{}, f.afterWrite()
	}
	return out, nil
}

// Get returns the state of batch id.
func (c *Client) Get(ctx context.Context, id string) (Batch, error) {
	var out Batch
	f, err := c.do(ctx, true, func(ctx context.Context) error {
		b, err := c.sdk.Messages.Batches.Get(ctx, id, anthropic.MessageBatchGetParams{}, option.WithRequestTimeout(c.o.RequestTimeout))
		if err != nil {
			return err
		}
		out = batchOf(*b)
		return nil
	})
	if err != nil {
		return Batch{}, err
	}
	if f != nil {
		return Batch{}, f.plain()
	}
	return out, nil
}

// Results downloads every result of an ended batch, in the order the API serves them.
func (c *Client) Results(ctx context.Context, id string) ([]Result, error) {
	var out []Result
	f, err := c.do(ctx, true, func(ctx context.Context) error {
		out = nil
		s := c.sdk.Messages.Batches.ResultsStreaming(ctx, id, anthropic.MessageBatchResultsParams{}, option.WithRequestTimeout(c.o.StreamTimeout))
		defer func() { _ = s.Close() }()
		for s.Next() {
			out = append(out, batchResult(s.Current()))
		}
		return s.Err()
	})
	if err != nil {
		return nil, err
	}
	if f != nil {
		return nil, f.plain()
	}
	return out, nil
}

// List returns the batches created at or after since minus ten minutes, newest first. The
// endpoint filters nothing by date and serves newest first, so List pages until a page holds a
// batch older than the cutoff.
func (c *Client) List(ctx context.Context, since time.Time) ([]Batch, error) {
	cutoff := since.Add(-listMargin)
	var out []Batch
	after := ""
	for {
		params := anthropic.MessageBatchListParams{Limit: anthropic.Int(100)}
		if after != "" {
			params.AfterID = anthropic.String(after)
		}
		var data []anthropic.MessageBatch
		var hasMore bool
		var last string
		f, err := c.do(ctx, true, func(ctx context.Context) error {
			p, err := c.sdk.Messages.Batches.List(ctx, params, option.WithRequestTimeout(c.o.RequestTimeout))
			if err != nil {
				return err
			}
			data, hasMore, last = p.Data, p.HasMore, p.LastID
			return nil
		})
		if err != nil {
			return nil, err
		}
		if f != nil {
			return nil, f.plain()
		}
		reachedOlder := false
		for _, b := range data {
			if b.CreatedAt.Before(cutoff) {
				reachedOlder = true
				continue
			}
			out = append(out, batchOf(b))
		}
		if reachedOlder || !hasMore || last == "" || last == after {
			return out, nil
		}
		after = last
	}
}

// Send makes one streamed Messages call and returns its result. An HTTP 400, 500, or 529 comes
// back as an errored Result so both modes classify alike. A failure after the request was
// written, including an SSE error event, is ErrMaybeSent; the caller settles the attempt as
// estimated and resubmits it as a new attempt.
func (c *Client) Send(ctx context.Context, r Request) (Result, error) {
	if err := c.requireCheck(); err != nil {
		return Result{}, err
	}
	w, err := buildWire(r)
	if err != nil {
		return Result{}, err
	}
	var msg anthropic.Message
	f, err := c.do(ctx, false, func(ctx context.Context) error {
		msg = anthropic.Message{}
		stopped := false
		s := c.sdk.Messages.NewStreaming(ctx, w.sendParams(r.MaxTokens), option.WithRequestTimeout(c.o.StreamTimeout))
		defer func() { _ = s.Close() }()
		for s.Next() {
			ev := s.Current()
			if err := msg.Accumulate(ev); err != nil {
				return err
			}
			stopped = stopped || ev.Type == "message_stop"
		}
		if err := s.Err(); err != nil {
			return err
		}
		if !stopped {
			return errIncompleteStream
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	if f != nil {
		switch f.status {
		case http.StatusBadRequest, http.StatusInternalServerError, 529:
			return erroredResult(r.CustomID, *f), nil
		}
		return Result{}, f.afterWrite()
	}
	return succeeded(r.CustomID, msg), nil
}

func erroredResult(customID string, f failure) Result {
	typ, msg := f.typ, f.msg
	if typ == "" {
		switch f.status {
		case http.StatusBadRequest:
			typ = "invalid_request_error"
		case http.StatusInternalServerError:
			typ = "api_error"
		case 529:
			typ = "overloaded_error"
		}
	}
	if msg == "" {
		msg = http.StatusText(f.status)
	}
	return Result{CustomID: customID, Kind: KindErrored, ErrType: typ, ErrMessage: msg}
}

func batchOf(b anthropic.MessageBatch) Batch {
	c := b.RequestCounts
	return Batch{
		ID: b.ID, Ended: b.ProcessingStatus == anthropic.MessageBatchProcessingStatusEnded,
		Status: string(b.ProcessingStatus), CreatedAt: b.CreatedAt,
		Counts: Counts{
			Processing: int(c.Processing), Succeeded: int(c.Succeeded), Errored: int(c.Errored),
			Canceled: int(c.Canceled), Expired: int(c.Expired),
		},
	}
}

func batchResult(r anthropic.MessageBatchIndividualResponse) Result {
	switch r.Result.Type {
	case "succeeded":
		return succeeded(r.CustomID, r.Result.Message)
	case "errored":
		return Result{CustomID: r.CustomID, Kind: KindErrored, ErrType: r.Result.Error.Error.Type, ErrMessage: r.Result.Error.Error.Message}
	default:
		return Result{CustomID: r.CustomID, Kind: r.Result.Type}
	}
}

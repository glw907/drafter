package batch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const (
	defaultRequestTimeout = 2 * time.Minute
	defaultStreamTimeout  = 30 * time.Minute
	defaultWallTime       = 5 * time.Minute

	backoffBase = time.Second
	backoffMax  = 30 * time.Second

	workspaceHeader = "anthropic-workspace-id"
)

// Client talks to the Claude API. It holds no connection state beyond the SDK client and starts
// no goroutines. Use it from one goroutine at a time.
type Client struct {
	// Sleep waits d, or until ctx ends, and reports why it stopped early. Tests replace it so
	// retry rows run in milliseconds.
	Sleep func(ctx context.Context, d time.Duration) error
	// Now is the backoff clock. Tests replace it.
	Now func() time.Time

	sdk     anthropic.Client
	o       Options
	checked bool
}

// New returns a Client for o. A zero RequestTimeout is two minutes, a zero StreamTimeout thirty,
// and a zero WallTime five.
//
// The SDK's own retries and environment credential discovery are off: ANTHROPIC_API_KEY,
// ANTHROPIC_AUTH_TOKEN, ANTHROPIC_PROFILE, ANTHROPIC_BASE_URL, and ANTHROPIC_CUSTOM_HEADERS
// never reach a request. Every request names o.WorkspaceID.
func New(o Options) *Client {
	if o.RequestTimeout == 0 {
		o.RequestTimeout = defaultRequestTimeout
	}
	if o.StreamTimeout == 0 {
		o.StreamTimeout = defaultStreamTimeout
	}
	if o.WallTime == 0 {
		o.WallTime = defaultWallTime
	}
	opts := []option.RequestOption{
		option.WithoutEnvironmentDefaults(),
		option.WithAPIKey(o.Key),
		option.WithMaxRetries(0),
		option.WithHeader(workspaceHeader, o.WorkspaceID),
	}
	if o.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(o.BaseURL))
	}
	return &Client{Sleep: sleep, Now: time.Now, sdk: anthropic.NewClient(opts...), o: o}
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// CheckWorkspace makes one free models call and requires the response's anthropic-workspace-id
// header to equal Options.WorkspaceID. An absent header is a mismatch. CountTokens, Create, and
// Send refuse until it has passed.
func (c *Client) CheckWorkspace(ctx context.Context) error {
	if c.o.WorkspaceID == "" {
		return &CallError{Message: "no workspace ID configured"}
	}
	var resp *http.Response
	f, err := c.do(ctx, true, func(ctx context.Context) error {
		_, err := c.sdk.Models.List(ctx, anthropic.ModelListParams{Limit: anthropic.Int(1)},
			option.WithRequestTimeout(c.o.RequestTimeout), option.WithResponseInto(&resp))
		return err
	})
	if err != nil {
		return err
	}
	if f != nil {
		return f.plain()
	}
	got := resp.Header.Get(workspaceHeader)
	if got == "" {
		return &CallError{Message: fmt.Sprintf("workspace mismatch: configured %s, response named none", c.o.WorkspaceID)}
	}
	if got != c.o.WorkspaceID {
		return &CallError{Message: fmt.Sprintf("workspace mismatch: configured %s, response named %s", c.o.WorkspaceID, got)}
	}
	c.checked = true
	return nil
}

func (c *Client) requireCheck() error {
	if !c.checked {
		return &CallError{Kind: ErrNoWorkspaceCheck, Message: "call CheckWorkspace first"}
	}
	return nil
}

// do runs call, retrying in place within WallTime. It returns (nil, nil) on success and
// (nil, err) when the call is over: the caller's context ended, a spend limit was hit, or
// retries outlasted the wall time. A failure that is not retried comes back as (f, nil) for
// the caller to map. idempotent calls retry any transport error and any 429, 500, or 529; the
// others retry only a failure that provably came before the request was written, and a 429.
func (c *Client) do(ctx context.Context, idempotent bool, call func(context.Context) error) (*failure, error) {
	start := c.Now()
	for attempt := 0; ; attempt++ {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr // nothing was sent
		}
		err := call(ctx)
		if err == nil {
			return nil, nil
		}
		if cerr := ctx.Err(); cerr != nil {
			if idempotent {
				return nil, cerr
			}
			// The context ended while the request may have been written; the caller must adopt or
			// settle, not treat the call as unsent.
			return nil, &CallError{Kind: ErrMaybeSent, Message: "caller context ended during the call", Cause: cerr.Error()}
		}
		f := inspect(err)
		if f.spend {
			return nil, f.callError(ErrSpendLimit)
		}
		delay, retry := f.retryDelay(idempotent, attempt)
		if !retry {
			return &f, nil
		}
		if c.Now().Sub(start)+delay > c.o.WallTime {
			return nil, f.callError(ErrTransport)
		}
		if err := c.Sleep(ctx, delay); err != nil {
			return nil, err
		}
	}
}

func backoff(attempt int) time.Duration {
	d := backoffBase
	for range attempt {
		d *= 2
		if d >= backoffMax {
			return backoffMax
		}
	}
	return d
}

var errIncompleteStream = errors.New("stream ended before message_stop")

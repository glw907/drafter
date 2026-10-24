// Package batch is drafter's client for the Claude API: the Message Batches endpoints for
// stage work and one streamed Messages call for sync work. It starts no goroutines.
//
// No error a call returns holds an SDK error value (whose Request field holds the key header).
// An API failure is a *CallError with the status, the API's error type, and its message. A
// transport failure is a *CallError whose Cause is the underlying error's text without the
// request URL. When retries run out (ErrTransport) or the API refuses at a spend limit
// (ErrSpendLimit), the *CallError carries whatever the last failure had. A client-side refusal,
// from CheckWorkspace or a billable call made before it passed, is a *CallError with a message
// and no status. A caller context that ends comes back as the context's own error, except while a
// Create or Send request is in flight, where it is a *CallError of kind ErrMaybeSent whose Cause is the context
// error's text. A request the client cannot build (a message role that is not "user" or
// "assistant", or a schema that is not a JSON object) is a plain error naming the request,
// returned before anything is sent.
package batch

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Options configures a Client. A zero timeout or WallTime takes the default New documents.
type Options struct {
	Key         string
	BaseURL     string
	WorkspaceID string

	// RequestTimeout bounds each unary call, response body included.
	RequestTimeout time.Duration
	// StreamTimeout bounds a whole Send stream or Results download.
	StreamTimeout time.Duration
	// WallTime bounds the time spent retrying one call in place.
	WallTime time.Duration
}

// Message is one conversation turn. Role is "user" or "assistant". Cached holds leading parts of
// a user turn, each sent as its own text block marked ephemeral for the five-minute prompt cache,
// ahead of the block that holds Text. The cache key covers the bytes up to each marked block, so
// a part must be byte-stable across the requests that share it.
type Message struct {
	Role   string
	Cached []string `json:",omitempty"`
	Text   string
}

// Request is one Messages request, shared by both modes. Schema is the answer's JSON Schema;
// the client sends it as the structured-output format. Effort and Schema are omitted from the
// wire when empty.
type Request struct {
	CustomID  string
	Model     string
	System    string
	Messages  []Message
	MaxTokens int64
	Effort    string
	Schema    json.RawMessage
}

// Result kinds.
const (
	KindSucceeded = "succeeded"
	KindErrored   = "errored"
	KindCanceled  = "canceled"
	KindExpired   = "expired"
)

// Result is the outcome of one request, in either mode. An HTTP 400, 500, or 529 from a sync
// call is an errored Result with the matching ErrType, so both modes classify alike.
type Result struct {
	CustomID   string
	MessageID  string
	Kind       string
	StopReason string
	Text       string
	ErrType    string
	ErrMessage string
	Usage      Usage
}

// Batch is the state of one message batch. Status is the API's processing_status. The API moves
// requests out of Counts.Processing only when the whole batch ends, so mid-batch the status and
// the age are the facts worth reporting.
type Batch struct {
	ID        string
	Ended     bool
	Status    string
	Counts    Counts
	CreatedAt time.Time
}

// Counts is a batch's request counts by state.
type Counts struct {
	Processing, Succeeded, Errored, Canceled, Expired int
}

// String lists the counts, for a status line.
func (c Counts) String() string {
	return fmt.Sprintf("%d succeeded, %d errored, %d canceled, %d expired", c.Succeeded, c.Errored, c.Canceled, c.Expired)
}

// Errors a Client returns, matched with errors.Is.
var (
	// ErrMaybeSent means the request may have reached the API and its outcome is unknown.
	ErrMaybeSent = errors.New("request may have been sent")
	// ErrTransport means in-place retries outlasted the wall time, or a retry-after was
	// longer than the wall time left.
	ErrTransport = errors.New("transport retries exhausted")
	// ErrSpendLimit means the API refused the call at a spend limit.
	ErrSpendLimit = errors.New("spend limit reached")
	// ErrNoWorkspaceCheck means a billable call was made before CheckWorkspace passed.
	ErrNoWorkspaceCheck = errors.New("workspace not checked")
)

// CallError is the error a Client returns for a failed call. Kind is one of the sentinel
// errors above or nil, and errors.Is matches it. Type is the API's error type, which for a
// failure inside a stream is the type of the SSE error event.
type CallError struct {
	Kind    error
	Status  int
	Type    string
	Message string
	// Cause describes a transport failure, without the request URL.
	Cause string
}

// Error implements error.
func (e *CallError) Error() string {
	var parts []string
	if e.Kind != nil {
		parts = append(parts, e.Kind.Error())
	}
	if e.Status != 0 {
		parts = append(parts, fmt.Sprintf("status %d", e.Status))
	}
	if e.Type != "" {
		parts = append(parts, e.Type)
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	if e.Cause != "" {
		parts = append(parts, e.Cause)
	}
	return "batch: " + strings.Join(parts, ": ")
}

// Unwrap returns Kind.
func (e *CallError) Unwrap() error { return e.Kind }

// Action is what the pipeline does with a Result. The zero value, ActionInvalid, is never
// accepted.
type Action int

// The actions Classify returns.
const (
	ActionInvalid Action = iota
	ActionAccept
	ActionResubmit
	ActionResubmitThenStop
	ActionEscalate
	ActionRetryAnswer
	ActionRefusal
	ActionSpendLimit
)

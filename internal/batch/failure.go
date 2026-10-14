package batch

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

const (
	spendCode   = "enforced_spend_limit_reached"
	spendPrefix = "You have reached your specified"
)

// failure is what a failed call leaves after the SDK error is read down to its status, type,
// and message. It never holds the SDK error, whose Request field carries the key header.
type failure struct {
	// transport is true when no HTTP status was read: a connection error, a timeout, or a
	// body that could not be read or parsed.
	transport bool
	// beforeWrite is true when the request provably never reached the API.
	beforeWrite bool
	spend       bool
	status      int
	typ         string
	msg         string
	retryAfter  string
	cause       string
}

func inspect(err error) failure {
	if ae, ok := errors.AsType[*anthropic.Error](err); ok {
		f := failure{status: ae.StatusCode, typ: string(ae.Type())}
		var code string
		f.msg, code = parseBody(ae.RawJSON())
		if ae.Response != nil {
			f.retryAfter = ae.Response.Header.Get("retry-after")
		}
		f.spend = (f.status == http.StatusTooManyRequests && code == spendCode) ||
			(f.status == http.StatusBadRequest && strings.HasPrefix(f.msg, spendPrefix))
		return f
	}
	return failure{transport: true, beforeWrite: provablyUnsent(err), cause: rootCause(err)}
}

// parseBody reads the message and details.error_code out of an API error body.
func parseBody(raw string) (msg, code string) {
	var body struct {
		Error struct {
			Message string `json:"message"`
			Details struct {
				ErrorCode string `json:"error_code"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(raw), &body) != nil {
		return "", ""
	}
	return body.Error.Message, body.Error.Details.ErrorCode
}

// provablyUnsent reports a failure that can only have come before the request was written: a
// DNS error, or a failed dial.
func provablyUnsent(err error) bool {
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return true
	}
	op, ok := errors.AsType[*net.OpError](err)
	return ok && op.Op == "dial"
}

// rootCause is the error's text without the request URL a *url.Error adds.
func rootCause(err error) string {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		err = ue.Err
	}
	return err.Error()
}

func (f failure) retryDelay(idempotent bool, attempt int) (time.Duration, bool) {
	switch {
	case f.transport:
		if idempotent || f.beforeWrite {
			return backoff(attempt), true
		}
	case f.status == http.StatusTooManyRequests:
		if secs, err := strconv.ParseFloat(f.retryAfter, 64); err == nil && secs >= 0 {
			return time.Duration(secs * float64(time.Second)), true
		}
		return backoff(attempt), true
	case idempotent && (f.status == http.StatusInternalServerError || f.status == 529):
		return backoff(attempt), true
	}
	return 0, false
}

func (f failure) callError(kind error) *CallError {
	return &CallError{Kind: kind, Status: f.status, Type: f.typ, Message: f.msg, Cause: f.cause}
}

func (f failure) plain() *CallError { return f.callError(nil) }

// afterWrite maps a failure that may have followed the write of a call that must not be
// repeated: a failure with no status, a status of 500 or above, or an SSE error event (status
// 200) is ErrMaybeSent. A failure before the write was retried already. Any other status is the
// API refusing the request, and stays plain.
func (f failure) afterWrite() *CallError {
	if f.transport || f.status >= 500 || f.status == http.StatusOK {
		return f.callError(ErrMaybeSent)
	}
	return f.plain()
}

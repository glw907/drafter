package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/glw907/drafter/internal/batch"
)

var customIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// outcome is one scripted result for a request.
type outcome struct {
	res batch.Result
	// missing leaves the request out of a batch's results.
	missing bool
}

func okText(text string) outcome {
	return outcome{res: batch.Result{Kind: batch.KindSucceeded, StopReason: "end_turn", Text: text}}
}

func stopped(reason string) outcome {
	return outcome{res: batch.Result{Kind: batch.KindSucceeded, StopReason: reason, Text: "{"}}
}

func errored(errType, msg string) outcome {
	return outcome{res: batch.Result{Kind: batch.KindErrored, ErrType: errType, ErrMessage: msg}}
}

func kind(k string) outcome { return outcome{res: batch.Result{Kind: k}} }

var absent = outcome{missing: true}

// fakeAPI is a scripted pipeline.API. Every fallible method has an error field. A request's
// outcome comes from the queue for its stage and seat, then from the default answer.
type fakeAPI struct {
	t *testing.T

	checkErr, countErr, getErr, resultsErr, listErr, sendErr error
	// createErr fails Create before the fake records anything. createSentErr records the batch
	// and then fails, the way a response lost on the wire does.
	createErr, createSentErr error
	// createErrs and sendErrs fail the nth call (1-based) only.
	createErrs map[int]error
	sendErrs   map[int]error

	// tokens is what CountTokens returns; 1000 when nil.
	tokens func(batch.Request) int64
	// queues maps "stage.seat" to scripted outcomes, consumed in order.
	queues map[string][]outcome
	// answer is the default answer for a request with no scripted outcome.
	answer func(batch.Request) string
	// pollsToEnd is how many Get calls report a batch as not ended.
	pollsToEnd int
	// onSubmit runs at the start of every Create and Send, so a test can check what came before.
	onSubmit func(ids []string)
	// crash is asked at each call that can end a run: "create", "send", "results". It returns
	// "before" to panic before the call does anything, "after" to panic once a create or send
	// has been recorded, or "" to go on. The panic stands for the process dying.
	crash func(call string, ids []string) string

	batches   map[string]*fakeBatch
	order     []string
	calls     []string
	creates   int
	sends     int
	counts    int
	requests  []batch.Request // every request Create or Send saw, in order
	seenIDs   map[string]bool
	allUnique bool
}

type fakeBatch struct {
	id      string
	created time.Time
	reqs    []batch.Request
	results []batch.Result
	polls   int
}

func newFakeAPI(t *testing.T) *fakeAPI {
	return &fakeAPI{
		t: t, queues: map[string][]outcome{}, batches: map[string]*fakeBatch{},
		seenIDs: map[string]bool{}, allUnique: true,
	}
}

// script queues outcomes for the requests of stage and seat.
func (f *fakeAPI) script(stage, seat string, outs ...outcome) {
	k := stage + "." + seat
	f.queues[k] = append(f.queues[k], outs...)
}

// stageSeat reads the stage and seat out of a custom_id.
func stageSeat(id string) (stage, seat string) {
	parts := strings.Split(id, "_")
	if len(parts) < 5 {
		return "", ""
	}
	return parts[2], parts[3]
}

func (f *fakeAPI) outcomeFor(r batch.Request) outcome {
	stage, seat := stageSeat(r.CustomID)
	k := stage + "." + seat
	if q := f.queues[k]; len(q) > 0 {
		f.queues[k] = q[1:]
		return q[0]
	}
	text := ""
	if f.answer != nil {
		text = f.answer(r)
	}
	if text == "" {
		text = defaultAnswer(r)
	}
	return okText(text)
}

func (f *fakeAPI) CheckWorkspace(ctx context.Context) error {
	f.calls = append(f.calls, "check")
	return f.checkErr
}

func (f *fakeAPI) CountTokens(ctx context.Context, r batch.Request) (int64, error) {
	f.counts++
	f.calls = append(f.calls, "count")
	if f.countErr != nil {
		return 0, f.countErr
	}
	if f.tokens != nil {
		return f.tokens(r), nil
	}
	return 1000, nil
}

func (f *fakeAPI) checkRequest(r batch.Request) {
	f.t.Helper()
	if !customIDPattern.MatchString(r.CustomID) {
		f.t.Errorf("custom_id %q does not match the API's pattern", r.CustomID)
	}
	if f.seenIDs[r.CustomID] {
		f.allUnique = false
		f.t.Errorf("custom_id %q was submitted twice in one run", r.CustomID)
	}
	f.seenIDs[r.CustomID] = true
	f.requests = append(f.requests, r)
}

func result(r batch.Request, o outcome) batch.Result {
	res := o.res
	res.CustomID = r.CustomID
	if res.Kind == batch.KindSucceeded {
		res.Usage = batch.Usage{InputTokens: 1000, OutputTokens: 500}
	}
	return res
}

func (f *fakeAPI) record(rs []batch.Request) *fakeBatch {
	f.t.Helper()
	b := &fakeBatch{id: fmt.Sprintf("batch_%d", len(f.batches)+1), created: time.Now(), reqs: rs}
	seen := map[string]bool{}
	for _, r := range rs {
		if seen[r.CustomID] {
			f.t.Errorf("custom_id %q repeats within one batch", r.CustomID)
		}
		seen[r.CustomID] = true
		f.checkRequest(r)
		o := f.outcomeFor(r)
		if !o.missing {
			b.results = append(b.results, result(r, o))
		}
	}
	f.batches[b.id] = b
	f.order = append(f.order, b.id)
	return b
}

func ids(rs []batch.Request) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.CustomID)
	}
	return out
}

// crashed is the panic value that stands for the process dying.
type crashed struct{}

func (f *fakeAPI) maybeCrash(call string, ids []string, phase string) {
	if f.crash != nil && f.crash(call, ids) == phase {
		panic(crashed{})
	}
}

func (f *fakeAPI) Create(ctx context.Context, rs []batch.Request) (batch.Batch, error) {
	f.creates++
	f.calls = append(f.calls, "create")
	if f.onSubmit != nil {
		f.onSubmit(ids(rs))
	}
	f.maybeCrash("create", ids(rs), "before")
	if f.crash != nil && f.crash("create", ids(rs)) == "after" {
		f.record(rs)
		panic(crashed{})
	}
	if err := f.createErrs[f.creates]; err != nil {
		return batch.Batch{}, err
	}
	if f.createErr != nil {
		return batch.Batch{}, f.createErr
	}
	b := f.record(rs)
	if f.createSentErr != nil {
		err := f.createSentErr
		f.createSentErr = nil
		return batch.Batch{}, err
	}
	return batch.Batch{ID: b.id, CreatedAt: b.created}, nil
}

func (f *fakeAPI) Get(ctx context.Context, id string) (batch.Batch, error) {
	f.calls = append(f.calls, "get")
	if f.getErr != nil {
		return batch.Batch{}, f.getErr
	}
	b, ok := f.batches[id]
	if !ok {
		return batch.Batch{}, &batch.CallError{Status: 404, Type: "not_found_error", Message: "no batch " + id}
	}
	b.polls++
	return f.batchOf(b), nil
}

func (f *fakeAPI) Results(ctx context.Context, id string) ([]batch.Result, error) {
	f.calls = append(f.calls, "results")
	f.maybeCrash("results", nil, "before")
	if f.resultsErr != nil {
		return nil, f.resultsErr
	}
	b, ok := f.batches[id]
	if !ok {
		return nil, &batch.CallError{Status: 404, Type: "not_found_error", Message: "no batch " + id}
	}
	return b.results, nil
}

func (f *fakeAPI) List(ctx context.Context, since time.Time) ([]batch.Batch, error) {
	f.calls = append(f.calls, "list")
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []batch.Batch
	for _, id := range slices.Backward(f.order) {
		b := f.batches[id]
		out = append(out, f.batchOf(b))
	}
	return out, nil
}

// batchOf reports b as the API would: in progress until pollsToEnd polls have passed, then ended
// with every request succeeded.
func (f *fakeAPI) batchOf(b *fakeBatch) batch.Batch {
	out := batch.Batch{ID: b.id, CreatedAt: b.created, Status: "in_progress", Counts: batch.Counts{Processing: len(b.reqs)}}
	if b.polls > f.pollsToEnd {
		out.Ended, out.Status = true, "ended"
		out.Counts = batch.Counts{Succeeded: len(b.reqs)}
	}
	return out
}

// addDecoy adds an ended batch of another run, which List returns beside the run's own.
func (f *fakeAPI) addDecoy(id string, created time.Time) {
	f.batches[id] = &fakeBatch{
		id: id, created: created, polls: 1 << 20,
		results: []batch.Result{{CustomID: "other_" + id, Kind: batch.KindSucceeded, StopReason: "end_turn", Text: "{}"}},
	}
	f.order = append(f.order, id)
}

func (f *fakeAPI) Send(ctx context.Context, r batch.Request) (batch.Result, error) {
	f.sends++
	f.calls = append(f.calls, "send")
	if f.onSubmit != nil {
		f.onSubmit([]string{r.CustomID})
	}
	f.maybeCrash("send", []string{r.CustomID}, "before")
	if err := f.sendErrs[f.sends]; err != nil {
		return batch.Result{}, err
	}
	if f.sendErr != nil {
		return batch.Result{}, f.sendErr
	}
	f.checkRequest(r)
	res := result(r, f.outcomeFor(r))
	if res.Kind == batch.KindSucceeded {
		res.MessageID = fmt.Sprintf("msg_%d", f.sends)
	}
	return res, nil
}

// goodPage and goodBrief satisfy plan rotate-the-api-key.plan.yaml.
const goodPage = `# Rotate the API key

Rotate a Lumen API key by publishing the new key before you retire the old one.

## Before you begin

A key lives for 90 days. See [key lifetime](../reference/keys.md#key-lifetime) and [revoking](revoke-a-key.md).

## Rotate the key

1. Create a new key.
2. Publish the site with the new key.
3. Check which key signed the build.

A passing health check shows only that the service is up, so read the publish log.

The new key's ID is the one the create step printed.
`

const goodBrief = `{"sentences":[{"text":"A key lives for 90 days.","id":"lumen-key-ttl"}]}`

const goodPageB = `# Revoke a key

Revoke the old key after the grace period.

## Revoke the key

1. Revoke the old key.
`

const goodBriefB = `{"sentences":[]}`

func jsonText(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func draftText(page, brief string) string {
	return jsonText(map[string]any{"page": page, "brief": brief})
}

func readText(verdict string, findings ...map[string]any) string {
	if findings == nil {
		findings = []map[string]any{}
	}
	return jsonText(map[string]any{"verdict": verdict, "findings": findings, "summary": "read"})
}

func finding(location, text string) map[string]any {
	return map[string]any{"location": location, "finding": text, "rewrite": "rewrite " + text, "blocking": true}
}

// defaultAnswer is the accepting answer for a request, chosen by its schema.
func defaultAnswer(r batch.Request) string {
	schema := string(r.Schema)
	stage, seat := stageSeat(r.CustomID)
	switch {
	case strings.Contains(schema, `"resolutions"`) && seat == "edit":
		return `{"edits":[],"resolutions":[{"finding":1,"status":"declined","edits":[],"reason":"the page needs no edit"}]}`
	case strings.Contains(schema, `"resolutions"`):
		return `{"edits":[],"resolutions":[]}`
	case strings.Contains(schema, `"conflicts"`):
		return `{"conflicts":[]}`
	case strings.Contains(schema, `"paraphrase"`):
		return `{"paraphrase":"p","summary":"s"}`
	case strings.Contains(schema, `"quote"`):
		return jsonText(map[string]any{"items": []map[string]string{
			{"id": "r-publish", "quote": "A passing health check shows only that the service is up, so read the publish log."},
			{"id": "r-verify", "quote": "The new key's ID is the one the create step printed."},
		}})
	case strings.Contains(schema, `"verdict"`):
		return readText("ok")
	case strings.Contains(schema, `"brief"`):
		if strings.Contains(r.Messages[len(r.Messages)-1].Text, "revoke-a-key.md") {
			return draftText(goodPageB, goodBriefB)
		}
		return draftText(goodPage, goodBrief)
	}
	return fmt.Sprintf("unscripted %s %s", stage, seat)
}

package batch

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		in   Result
		want Action
	}{
		{"errored api_error", Result{Kind: KindErrored, ErrType: "api_error"}, ActionResubmit},
		{"errored overloaded_error", Result{Kind: KindErrored, ErrType: "overloaded_error"}, ActionResubmit},
		{"errored invalid_request_error", Result{Kind: KindErrored, ErrType: "invalid_request_error", ErrMessage: "bad"}, ActionEscalate},
		{"errored billing_error", Result{Kind: KindErrored, ErrType: "billing_error"}, ActionEscalate},
		{"errored empty type", Result{Kind: KindErrored}, ActionEscalate},
		{"errored unknown type", Result{Kind: KindErrored, ErrType: "mystery_error"}, ActionEscalate},
		{"errored org spend limit", Result{Kind: KindErrored, ErrType: "invalid_request_error", ErrMessage: orgLimitMsg}, ActionSpendLimit},
		{"errored workspace spend limit", Result{Kind: KindErrored, ErrType: "invalid_request_error", ErrMessage: wsLimitMsg}, ActionSpendLimit},
		{"canceled", Result{Kind: KindCanceled}, ActionResubmitThenStop},
		{"expired", Result{Kind: KindExpired}, ActionResubmitThenStop},
		{"end_turn", Result{Kind: KindSucceeded, StopReason: "end_turn"}, ActionAccept},
		{"max_tokens", Result{Kind: KindSucceeded, StopReason: "max_tokens"}, ActionRetryAnswer},
		{"context window", Result{Kind: KindSucceeded, StopReason: "model_context_window_exceeded"}, ActionRetryAnswer},
		{"refusal", Result{Kind: KindSucceeded, StopReason: "refusal"}, ActionRefusal},
		{"pause_turn", Result{Kind: KindSucceeded, StopReason: "pause_turn"}, ActionEscalate},
		{"empty stop reason", Result{Kind: KindSucceeded}, ActionEscalate},
		{"unknown kind", Result{Kind: "mystery"}, ActionEscalate},
		{"zero result", Result{}, ActionInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.in); got != tt.want {
				t.Errorf("Classify(%+v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestActionZeroIsInvalid(t *testing.T) {
	var a Action
	if a != ActionInvalid {
		t.Errorf("zero Action = %v, want ActionInvalid", a)
	}
}

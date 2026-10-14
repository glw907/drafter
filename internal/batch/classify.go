package batch

import "strings"

// Classify maps a Result to what the pipeline does with it. Only a zero Result is
// ActionInvalid; anything unrecognized escalates.
func Classify(r Result) Action {
	switch r.Kind {
	case "":
		return ActionInvalid
	case KindCanceled, KindExpired:
		return ActionResubmitThenStop
	case KindErrored:
		if strings.HasPrefix(r.ErrMessage, spendPrefix) {
			return ActionSpendLimit
		}
		switch r.ErrType {
		case "api_error", "overloaded_error":
			return ActionResubmit
		}
		return ActionEscalate
	case KindSucceeded:
		switch r.StopReason {
		case "end_turn":
			return ActionAccept
		case "max_tokens", "model_context_window_exceeded":
			return ActionRetryAnswer
		case "refusal":
			return ActionRefusal
		}
	}
	return ActionEscalate
}

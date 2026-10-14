package batch

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// wire is the parts of a Request that the batch, count_tokens, and messages endpoints share.
type wire struct {
	model    anthropic.Model
	messages []anthropic.MessageParam
	system   []anthropic.TextBlockParam
	output   anthropic.OutputConfigParam
}

func buildWire(r Request) (wire, error) {
	w := wire{model: anthropic.Model(r.Model)}
	for _, m := range r.Messages {
		block := anthropic.NewTextBlock(m.Text)
		switch m.Role {
		case "user":
			w.messages = append(w.messages, anthropic.NewUserMessage(block))
		case "assistant":
			w.messages = append(w.messages, anthropic.NewAssistantMessage(block))
		default:
			return wire{}, fmt.Errorf("batch: request %q message role %q is not user or assistant", r.CustomID, m.Role)
		}
	}
	if r.System != "" {
		w.system = []anthropic.TextBlockParam{{Text: r.System}}
	}
	if r.Effort != "" {
		w.output.Effort = anthropic.OutputConfigEffort(r.Effort)
	}
	if len(r.Schema) > 0 {
		var schema map[string]any
		if err := json.Unmarshal(r.Schema, &schema); err != nil {
			return wire{}, fmt.Errorf("batch: request %q schema is not a JSON object: %w", r.CustomID, err)
		}
		w.output.Format = anthropic.JSONOutputFormatParam{Schema: schema}
	}
	return w, nil
}

func (w wire) batchParams(customID string, maxTokens int64) anthropic.MessageBatchNewParamsRequest {
	return anthropic.MessageBatchNewParamsRequest{
		CustomID: customID,
		Params: anthropic.MessageBatchNewParamsRequestParams{
			MaxTokens:    maxTokens,
			Messages:     w.messages,
			Model:        w.model,
			System:       w.system,
			OutputConfig: w.output,
		},
	}
}

func (w wire) countParams() anthropic.MessageCountTokensParams {
	return anthropic.MessageCountTokensParams{
		Messages:     w.messages,
		Model:        w.model,
		System:       anthropic.MessageCountTokensParamsSystemUnion{OfTextBlockArray: w.system},
		OutputConfig: w.output,
	}
}

func (w wire) sendParams(maxTokens int64) anthropic.MessageNewParams {
	return anthropic.MessageNewParams{
		MaxTokens:    maxTokens,
		Messages:     w.messages,
		Model:        w.model,
		System:       w.system,
		OutputConfig: w.output,
	}
}

func usageOf(u anthropic.Usage) Usage {
	out := Usage{
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		CacheWrite5m: u.CacheCreation.Ephemeral5mInputTokens,
		CacheWrite1h: u.CacheCreation.Ephemeral1hInputTokens,
		CacheRead:    u.CacheReadInputTokens,
	}
	// A response without the per-TTL breakdown prices its writes as five-minute writes.
	if out.CacheWrite5m == 0 && out.CacheWrite1h == 0 {
		out.CacheWrite5m = u.CacheCreationInputTokens
	}
	return out
}

func succeeded(customID string, m anthropic.Message) Result {
	var text strings.Builder
	for _, b := range m.Content {
		if b.Type == "text" {
			text.WriteString(b.Text)
		}
	}
	return Result{
		CustomID:   customID,
		MessageID:  m.ID,
		Kind:       KindSucceeded,
		StopReason: string(m.StopReason),
		Text:       text.String(),
		Usage:      usageOf(m.Usage),
	}
}

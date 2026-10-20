package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/glw907/drafter/internal/assemble"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/gate"
)

// Shape is one seat's request: its answer schema, effort, system prompt, and token limit.
// MaxTokens is 4096 because Haiku 5.5's thinking counts toward max_tokens.
type Shape struct {
	Seat      string
	System    string
	Prompt    string
	Effort    string
	MaxTokens int64
	Schema    json.RawMessage
}

// Model is Haiku 5.5's model ID, from the claude-api skill's model table. Every smoke request
// runs on it, so a live run costs cents.
const Model = "claude-haiku-5-5"

// seatAnswers lists the ten seats, the blueprint, coherence, and facttruth seats last, each with the answer its production
// request asks for. The reader's second turn shares the read schema, so one shape per seat
// covers every schema production sends.
var seatAnswers = []struct {
	seat   string
	answer assemble.Answer
	effort string
}{
	{"draft", assemble.AnswerDraft, "high"},
	{"structure", assemble.AnswerRead, "medium"},
	{"editor", assemble.AnswerRead, "medium"},
	{"fact", assemble.AnswerRead, "medium"},
	{"reader", assemble.AnswerReader1, "low"},
	{"coverage", assemble.AnswerCoverage, "low"},
	{"consistency", assemble.AnswerConsistency, "low"},
	{"blueprint", assemble.AnswerBlueprint, "medium"},
	{"coherence", assemble.AnswerRead, "medium"},
	{"facttruth", assemble.AnswerFactTruth, "medium"},
}

const (
	smokePage  = "docs/page.md"
	smokeBrief = "docs/page.brief.json"
)

// Project writes a synthetic project under dir, which must be an empty directory: a git
// repository holding a page, its brief, and one agent definition per seat. It returns the
// configuration the shapes build from, with every seat on Model at its production effort and
// max_tokens 4096.
func Project(dir string) (*config.Config, error) {
	files := map[string]string{
		smokePage:  "# Smoke\n\nThis page exists only to give each seat's request a page to inline.\n",
		smokeBrief: "{}\n",
	}
	c := &config.Config{Root: dir, KeyEnv: "DRAFTER_API_KEY", Seat: map[string]config.Seat{}}
	for _, s := range seatAnswers {
		agent := filepath.Join("agents", s.seat+".md")
		files[agent] = fmt.Sprintf("---\nname: %s\n---\nYou are the %s seat of a documentation pipeline. Answer only with the JSON the schema describes.\n", s.seat, s.seat)
		c.Seat[s.seat] = config.Seat{Agent: filepath.Join(dir, agent), Model: Model, Effort: s.effort, MaxTokens: 4096}
	}
	for name, text := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create project: %w", err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return nil, fmt.Errorf("write project file: %w", err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(gate.Env(c), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("git %s: %w: %s", args[0], err, bytes.TrimSpace(out))
		}
	}
	return c, nil
}

// Shapes builds one request per seat through assemble.Build, so each shape carries the answer
// schema, effort, system prompt, and contract text production sends. c is a configuration
// Project returned.
func Shapes(ctx context.Context, c *config.Config) ([]Shape, error) {
	out := make([]Shape, 0, len(seatAnswers))
	for _, s := range seatAnswers {
		req, err := assemble.Build(ctx, assemble.Input{
			Seat:   s.seat,
			Answer: s.answer,
			Config: c,
			Prompt: fmt.Sprintf("Smoke test for the %s seat. Read %s and answer in the schema's shape, keeping each list empty.", s.seat, smokePage),
			Page:   smokePage,
			Brief:  smokeBrief,
			Slug:   "smoke",
		})
		if err != nil {
			return nil, fmt.Errorf("build %s request: %w", s.seat, err)
		}
		out = append(out, Shape{
			Seat:      s.seat,
			System:    req.System,
			Prompt:    req.Messages[len(req.Messages)-1].Text,
			Effort:    req.Effort,
			MaxTokens: req.MaxTokens,
			Schema:    req.Schema,
		})
	}
	return out, nil
}

// Check reports whether text is JSON that satisfies s's schema. It understands the keywords the
// production schemas use: type, enum, required, properties, additionalProperties false, items, and
// description, which it ignores.
func Check(s Shape, text string) error {
	var schema map[string]any
	if err := json.Unmarshal(s.Schema, &schema); err != nil {
		return fmt.Errorf("schema of %s: %w", s.Seat, err)
	}
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return fmt.Errorf("%s answer is not JSON: %w", s.Seat, err)
	}
	if err := validate(schema, v, "$"); err != nil {
		return fmt.Errorf("%s answer: %w", s.Seat, err)
	}
	return nil
}

func validate(schema map[string]any, v any, path string) error {
	if enum, ok := schema["enum"].([]any); ok {
		if slices.Contains(enum, v) {
			return nil
		}
		return fmt.Errorf("%s is %v, not one of %v", path, v, enum)
	}
	switch schema["type"] {
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("%s is not an object", path)
		}
		props, _ := schema["properties"].(map[string]any)
		if schema["additionalProperties"] == false {
			for k := range obj {
				if _, ok := props[k]; !ok {
					return fmt.Errorf("%s has unexpected property %q", path, k)
				}
			}
		}
		req, _ := schema["required"].([]any)
		for _, r := range req {
			if k, _ := r.(string); obj[k] == nil {
				return fmt.Errorf("%s lacks required property %q", path, k)
			}
		}
		for k, sub := range props {
			val, present := obj[k]
			if !present {
				continue
			}
			ss, _ := sub.(map[string]any)
			if err := validate(ss, val, path+"."+k); err != nil {
				return err
			}
		}
	case "array":
		arr, ok := v.([]any)
		if !ok {
			return fmt.Errorf("%s is not an array", path)
		}
		items, _ := schema["items"].(map[string]any)
		for i, el := range arr {
			if err := validate(items, el, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case "string":
		if _, ok := v.(string); !ok {
			return fmt.Errorf("%s is not a string", path)
		}
	case "integer", "number":
		if _, ok := v.(float64); !ok {
			return fmt.Errorf("%s is not a number", path)
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("%s is not a boolean", path)
		}
	default:
		return fmt.Errorf("%s: schema type %v is not supported", path, strings.TrimSpace(fmt.Sprint(schema["type"])))
	}
	return nil
}

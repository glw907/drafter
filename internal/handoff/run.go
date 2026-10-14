package handoff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// chain is the handoff the chain's handoffFor returns, in its own versioned shape.
type chain struct {
	Version  int             `json:"version"`
	Worktree string          `json:"worktree"`
	Args     json.RawMessage `json:"args"`
	Resolved json.RawMessage `json:"resolved"`
	Pages    []chainPage     `json:"pages"`
}

type chainPage struct {
	Page    Page   `json:"page"`
	Plan    string `json:"plan"`
	Framing string `json:"framing"`
	Inputs  Inputs `json:"inputs"`
}

// ReadWorkflowRun reads the handoff from a Workflow run file. Only .result.handoff is read, so
// the rest of the run file, which is Claude Code's own format, may change freely; the handoff
// itself is decoded strictly. Each page converts to a Handoff with the worktree as its project
// and the chain's args and resolved pages as its context, and its checksum is computed from the
// files and recorded. A run file with no result, no handoff, or no pages is refused, and so is
// a resolved page that carries a figure or rework, which the drafter does not handle.
func ReadWorkflowRun(path string) ([]Handoff, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workflow run %s: %w", path, err)
	}
	var run struct {
		Result *struct {
			Handoff json.RawMessage `json:"handoff"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, fmt.Errorf("decode workflow run %s: %w", path, err)
	}
	if run.Result == nil {
		return nil, fmt.Errorf("workflow run %s: no result; the run has not finished", path)
	}
	raw := bytes.TrimSpace(run.Result.Handoff)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, fmt.Errorf("workflow run %s: result.handoff is missing or null; run the chain with backHalf set to drafter", path)
	}
	var c chain
	if err := decodeStrict(raw, &c); err != nil {
		return nil, fmt.Errorf("decode workflow run %s: result.handoff: %w", path, err)
	}
	if c.Version != Version {
		return nil, fmt.Errorf("workflow run %s: result.handoff has unknown version %d, want %d", path, c.Version, Version)
	}
	if len(c.Pages) == 0 {
		return nil, fmt.Errorf("workflow run %s: result.handoff has no pages", path)
	}
	if err := refuseUnsupported(path, c.Resolved); err != nil {
		return nil, err
	}
	ctx, err := json.Marshal(struct {
		Args     json.RawMessage `json:"args"`
		Resolved json.RawMessage `json:"resolved"`
	}{c.Args, c.Resolved})
	if err != nil {
		return nil, fmt.Errorf("workflow run %s: encode context: %w", path, err)
	}
	var out []Handoff
	for _, p := range c.Pages {
		h := Handoff{
			Version: Version, Project: c.Worktree, Page: p.Page, Plan: p.Plan,
			Framing: p.Framing, Inputs: p.Inputs, Context: ctx, File: path,
		}
		if err := h.check(); err != nil {
			return nil, err
		}
		if h.Checksum, err = Checksum(h.Project, h.Plan, h.Framing); err != nil {
			return nil, fmt.Errorf("workflow run %s: %w", path, err)
		}
		out = append(out, h)
	}
	return out, nil
}

// refuseUnsupported refuses a resolved page with figure: true or a non-empty rework, naming it.
func refuseUnsupported(path string, resolved json.RawMessage) error {
	var r struct {
		Pages []struct {
			Path   string          `json:"path"`
			Figure bool            `json:"figure"`
			Rework json.RawMessage `json:"rework"`
		} `json:"pages"`
	}
	if len(resolved) == 0 {
		return nil
	}
	if err := json.Unmarshal(resolved, &r); err != nil {
		return fmt.Errorf("workflow run %s: decode resolved pages: %w", path, err)
	}
	for _, p := range r.Pages {
		if p.Figure {
			return fmt.Errorf("workflow run %s: page %s carries a figure, which drafter does not draft", path, p.Path)
		}
		switch string(bytes.TrimSpace(p.Rework)) {
		case "", "null", "false", `""`, "[]", "{}":
		default:
			return fmt.Errorf("workflow run %s: page %s carries rework, which drafter does not draft", path, p.Path)
		}
	}
	return nil
}

package assemble

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/gate"
)

// commandTimeout bounds each seat input command; tests shorten it.
var commandTimeout = 5 * time.Minute

// commandWaitDelay is how long a timed-out command's output pipes may stay open after the shell
// is killed, since a child it started can hold them.
var commandWaitDelay = 2 * time.Second

// commandBlocks runs each command in the seat's inputs from the project root and returns its
// output as a block under the command text, whatever its exit status. A command naming
// {baseline} runs only when the input has a baseline. Only draft and read requests, the blueprint
// read's included, run commands.
func commandBlocks(ctx context.Context, in Input, seat config.Seat) ([]string, error) {
	switch in.Answer {
	case AnswerDraft, AnswerRead, AnswerBlueprint:
	default:
		return nil, nil
	}
	var blocks []string
	for _, tmpl := range seat.Inputs {
		if strings.Contains(tmpl, "{baseline}") && in.Baseline == "" {
			continue
		}
		text := gate.Command(tmpl, in.Page, in.Brief, in.Slug, in.Baseline)
		out, err := runCommand(ctx, in.Config, text)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block("command", text, out))
	}
	return blocks, nil
}

// runCommand returns the combined output of text, followed by a note when it exited nonzero or
// ran past commandTimeout. An output over the size limit is replaced by a not-inlined line.
func runCommand(ctx context.Context, c *config.Config, text string) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(tctx, "/bin/sh", "-c", text)
	cmd.Dir = c.Root
	cmd.Env = gate.Env(c)
	cmd.WaitDelay = commandWaitDelay
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()

	var ee *exec.ExitError
	var note string
	switch {
	case ctx.Err() != nil:
		return "", fmt.Errorf("assemble: run input command: %w", ctx.Err())
	case errors.Is(tctx.Err(), context.DeadlineExceeded):
		note = fmt.Sprintf("[timed out after %s; output cut off]", commandTimeout)
	case errors.As(err, &ee):
		note = fmt.Sprintf("[exit status %d]", ee.ExitCode())
	case err != nil:
		return "", fmt.Errorf("assemble: run input command: %w", err)
	}
	out := buf.String()
	if buf.Len() > sizeLimit {
		out = fmt.Sprintf("not inlined: %d bytes", buf.Len())
	}
	if note != "" {
		out = strings.TrimSuffix(out, "\n") + "\n" + note
	}
	return out, nil
}

// Package key resolves the Claude API key from the one source the configuration names.
package key

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"

	"github.com/glw907/drafter/internal/config"
)

// timeout bounds a key_command. It is a variable so tests can shorten it.
var timeout = 30 * time.Second

// Resolve returns the API key. When c.KeyCommand is set it runs the command through /bin/sh -c
// under a 30-second timeout, discards its standard error, and returns its trimmed standard
// output; otherwise it reads the variable c.KeyEnv names. No other variable is consulted.
//
// Every failure names the source and never carries what the command printed.
func Resolve(c *config.Config) (string, error) {
	if c.KeyCommand != "" {
		return fromCommand(c.KeyCommand)
	}
	v, ok := os.LookupEnv(c.KeyEnv)
	if !ok {
		return "", fmt.Errorf("key_env variable %s is unset", c.KeyEnv)
	}
	return check(v, "key_env variable "+c.KeyEnv)
}

func fromCommand(command string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil // discarded: the null device
	// A killed shell can leave a child holding the pipe; stop waiting for it.
	cmd.WaitDelay = 500 * time.Millisecond
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("key_command timed out after %s", timeout)
		}
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			return "", fmt.Errorf("key_command exited with status %d", ee.ExitCode())
		}
		return "", errors.New("key_command did not run")
	}
	return check(out.String(), "key_command")
}

func check(v, source string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("%s is empty", source)
	}
	if strings.IndexFunc(v, unicode.IsSpace) >= 0 {
		return "", fmt.Errorf("%s holds whitespace inside the key", source)
	}
	return v, nil
}

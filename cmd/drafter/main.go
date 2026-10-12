package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Exit statuses: 0 is success, so only the failures are named.
const (
	exitFindings  = 1 // a check reported an error finding
	exitCannotRun = 2 // bad usage, or an input the command could not read
)

// exitError carries a nonzero exit status. A message prints to stderr, then the usage text when
// one is set; a findings failure has already printed its findings, so its message is empty.
type exitError struct {
	code  int
	msg   string
	usage string
}

func (e exitError) Error() string { return e.msg }

func usageError(cmd *cobra.Command, msg string) error {
	return exitError{code: exitCannotRun, msg: msg, usage: cmd.UsageString()}
}

func main() {
	cmd := newRootCmd()
	err := cmd.Execute()
	if err == nil {
		return
	}
	code := 1
	if ee, ok := errors.AsType[exitError](err); ok {
		code = ee.code
		if ee.msg != "" {
			fmt.Fprintln(os.Stderr, ee.msg)
		}
		if ee.usage != "" {
			fmt.Fprint(os.Stderr, ee.usage)
		}
	} else {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/key"
	"github.com/glw907/drafter/internal/plan"
	runstate "github.com/glw907/drafter/internal/run"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status <run-id> [flags]",
		Short: "Print the status of each open batch of a run",
		Long: `Print the status, age, and request counts of each batch the run has submitted and not yet
read. The command reads the run's state without taking its lock, so it works while the run is
live, and it asks the API about each open batch. The API moves requests out of processing only
when the whole batch ends, so a batch in progress shows its status and age. A run with no open
batch prints "no open batch". With --json each batch prints as one object. The exit status is 0
on success and 2 when the run's state cannot be read or the API cannot answer.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return runStatus(cmd, args[0], asJSON) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().BoolVar(&asJSON, "json", false, "print each batch as one JSON object")
	return cmd
}

// statusRow is the JSON shape of one open batch.
type statusRow struct {
	Batch      string `json:"batch"`
	Page       string `json:"page"`
	Status     string `json:"status"`
	Created    string `json:"created"`
	Processing int    `json:"processing"`
	Succeeded  int    `json:"succeeded"`
	Errored    int    `json:"errored"`
	Canceled   int    `json:"canceled"`
	Expired    int    `json:"expired"`
}

func runStatus(cmd *cobra.Command, id string, asJSON bool) error {
	cannot := func(err error) error { return exitError{code: exitCannotRun, msg: err.Error()} }
	if id != filepath.Base(id) {
		return usageError(cmd, fmt.Sprintf("%q is not a run ID", id))
	}
	root, err := plan.GitRoot(".")
	if err != nil {
		return cannot(err)
	}
	c, err := config.Load(root, config.Flags{JSON: asJSON})
	if err != nil {
		return cannot(err)
	}
	repo, err := runstate.Repo(root)
	if err != nil {
		return cannot(err)
	}
	st, err := runstate.LoadState(filepath.Join(c.StateDir, "runs", repo, id))
	if err != nil {
		return cannot(err)
	}
	type open struct{ page, batchID string }
	var opens []open
	for _, pk := range slices.Sorted(maps.Keys(st.Pages)) {
		for _, in := range st.Pages[pk].Intents {
			if in.BatchID != "" && !in.Saved {
				opens = append(opens, open{pk, in.BatchID})
			}
		}
	}
	out := cmd.OutOrStdout()
	if len(opens) == 0 {
		if !asJSON {
			_, err = fmt.Fprintln(out, "no open batch")
		}
		return err
	}
	k, err := key.Resolve(c)
	if err != nil {
		return cannot(err)
	}
	client := batch.New(batch.Options{Key: k, BaseURL: c.APIBaseURL, WorkspaceID: c.WorkspaceID})
	enc := json.NewEncoder(out)
	for _, o := range opens {
		b, err := client.Get(cmd.Context(), o.batchID)
		if err != nil {
			return cannot(err)
		}
		if asJSON {
			n := b.Counts
			err = enc.Encode(statusRow{
				Batch: b.ID, Page: o.page, Status: b.Status, Created: b.CreatedAt.UTC().Format(time.RFC3339),
				Processing: n.Processing, Succeeded: n.Succeeded, Errored: n.Errored, Canceled: n.Canceled, Expired: n.Expired,
			})
		} else {
			err = writeStatusLine(out, o.page, b)
		}
		if err != nil {
			return err
		}
	}
	if !asJSON {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "a batch can take hours; --sync runs attended without a batch wait")
	}
	return nil
}

func writeStatusLine(w io.Writer, page string, b batch.Batch) error {
	age := strings.TrimSuffix(time.Since(b.CreatedAt).Truncate(time.Minute).String(), "0s")
	if age == "" {
		age = "under a minute"
	}
	line := fmt.Sprintf("batch %s (%s): %s, %s old", b.ID, page, b.Status, age)
	if b.Ended {
		line += ", " + b.Counts.String()
	}
	_, err := fmt.Fprintln(w, line)
	return err
}

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/glw907/drafter/internal/plan"
	"github.com/spf13/cobra"
)

func newAnchorsCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "anchors [flags] <file>...",
		Short: "List the anchors a Markdown file offers",
		Long: `Print every anchor each Markdown file offers, one per line, in document order: the
slug of each heading, as check-plan computes it, and the id of each <a id="..."> element. With
several files, each anchor is written as file#anchor. The exit status is 0 on success, 1 when a
file cannot be read, and 2 on bad usage.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usageError(cmd, "anchors needs at least one Markdown file")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error { return runAnchors(cmd, args, asJSON) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the anchors as one JSON array")
	return cmd
}

func runAnchors(cmd *cobra.Command, files []string, asJSON bool) error {
	var all []string
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		for _, a := range plan.Anchors(data) {
			if len(files) > 1 {
				a = file + "#" + a
			}
			all = append(all, a)
		}
	}
	out := cmd.OutOrStdout()
	if asJSON {
		if all == nil {
			all = []string{} // JSON wants [] for no anchors, not null
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(all)
	}
	for _, a := range all {
		if _, err := fmt.Fprintln(out, a); err != nil {
			return err
		}
	}
	return nil
}

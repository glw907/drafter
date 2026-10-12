package main

import (
	"errors"

	"github.com/spf13/cobra"
)

func newCheckPlanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check-plan <plan>...",
		Short: "Validate page plans",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("check-plan: not implemented")
		},
	}
}

package main

import "github.com/spf13/cobra"

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "drafter",
		Short:         "Plan checks and drafting for documentation pages",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version(),
	}
	cmd.AddCommand(newCheckPlanCmd(), newAnchorsCmd(), newConfigCmd(), newSpendCmd(), newCheckPromptsCmd(), newPageCmd(),
		newCheckCmd(), newRefsCmd(), newOutlineCmd(), newExcerptCmd(), newConvertCmd(), newReadCmd(), newSeedCmd(), newGateCmd(), newInitCmd())
	return cmd
}

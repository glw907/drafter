package main

import (
	"fmt"
	"path"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/spf13/cobra"
)

type seedFlags struct {
	root string
}

func newSeedCmd() *cobra.Command {
	var f seedFlags
	cmd := &cobra.Command{
		Use:   "seed [flags] <blueprint>...",
		Short: "Give an existing page its drafted.lock entry",
		Long: `Write the drafted.lock entry that conversion would have written for each named blueprint's
page, so the first run on a page that predates its blueprint keeps the page's prose.

Run drafter read on the blueprint first, and commit the page and the blueprint. A blueprint
qualifies when its lock entry holds an accepted id equal to the blueprint as committed at HEAD and no
page id. Seed checks every blueprint before it writes, and refuses the whole call at the first
one that does not qualify, naming the page and the fix.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usageError(cmd, "seed needs at least one blueprint")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error { return runSeed(cmd, args, &f) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.root, "root", "", "project root (default: the git top level of the working directory)")
	return cmd
}

func runSeed(cmd *cobra.Command, args []string, f *seedFlags) error {
	p, err := projectFor(f.root)
	if err != nil {
		return err
	}
	named := make([]string, 0, len(args))
	for _, a := range args {
		rel, err := repoRelative(p.Root, a)
		if err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
		named = append(named, rel)
	}
	layout := blueprint.Layout{BlueprintRoot: p.BlueprintRoot, LockPath: path.Join(path.Dir(p.Outline), blueprint.LockFile)}
	pages, err := blueprint.Seed(p.Root, layout, named)
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	for _, page := range pages {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "seeded %s\n", page); err != nil {
			return err
		}
	}
	return nil
}

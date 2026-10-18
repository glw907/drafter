package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/internal/scaffold"
	"github.com/spf13/cobra"
)

type initFlags struct {
	root string
}

func newInitCmd() *cobra.Command {
	var f initFlags
	cmd := &cobra.Command{
		Use:   "init [flags]",
		Short: "Start a drafter project in a git repository",
		Long: `Write the files a new drafter project needs: drafter.toml, a blueprint outline, a facts
store with one verified seed fact, a .vale.ini that pins the Google style package, and the
.gitignore that keeps the synced style out of the work tree. An existing .vale.ini is kept.

init refuses, writing nothing, outside a git work tree and when drafter.toml or the blueprints
directory already exists. It prints each path it wrote on standard output.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runInit(cmd, &f) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.root, "root", "", "project root (default: the git top level of the working directory)")
	return cmd
}

func runInit(cmd *cobra.Command, f *initFlags) error {
	root := f.root
	if root == "" {
		top, err := plan.GitRoot(".")
		if err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
		root = top
	} else if _, err := plan.GitRoot(root); err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	res, err := scaffold.Write(root)
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	out, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	for _, p := range res.Written {
		if _, err := fmt.Fprintln(out, p); err != nil {
			return err
		}
	}
	if res.ValeKept {
		if _, err := io.WriteString(stderr, "kept the existing .vale.ini\n"); err != nil {
			return err
		}
	}
	_, err = io.WriteString(stderr, "next: run vale sync, then drafter check\n")
	return err
}

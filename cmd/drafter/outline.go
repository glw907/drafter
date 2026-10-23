package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/plan"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

func newOutlineCmd() *cobra.Command {
	var f checkFlags
	cmd := &cobra.Command{
		Use:   "outline [flags] <slug | page path>",
		Short: "Print a page's outline entry",
		Long: `Print the outline entry of one page, found by its slug or its path, as YAML. With --json
it prints the entry's fields as one JSON object. The exit status is 0 when the page is
listed, 1 when it is not, and 2 when the command could not run.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(cmd, "outline needs exactly one slug or page path")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error { return runOutline(cmd, args[0], &f) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.root, "root", "", "project root (default: the git top level of the working directory)")
	cmd.Flags().BoolVar(&f.json, "json", false, "print the entry as one JSON object")
	return cmd
}

func runOutline(cmd *cobra.Command, arg string, f *checkFlags) error {
	p, err := projectFor(f.root)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(p.Outline)))
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	o, findings := blueprint.DecodeOutline(p.Outline, data)
	if o == nil || plan.HasError(findings) {
		return exitError{code: exitCannotRun, msg: fmt.Sprintf("outline %s is malformed", p.Outline)}
	}
	for _, pg := range o.Pages {
		if pg.Slug != arg && path.Clean(pg.Path) != path.Clean(arg) {
			continue
		}
		raw, err := yaml.Marshal(pg)
		if err != nil {
			return err
		}
		if !f.json {
			_, err = cmd.OutOrStdout().Write(raw)
			return err
		}
		var fields map[string]any
		if err := yaml.Unmarshal(raw, &fields); err != nil {
			return err
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(fields)
	}
	return exitError{code: exitFindings, msg: fmt.Sprintf("%s is not a slug or path the outline lists", arg)}
}

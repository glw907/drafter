package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/plan"
	"github.com/spf13/cobra"
)

func newConvertCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert the chain's outline and plans to the blueprint format",
		Long: `Convert files from the chain's formats to the blueprint format. The outline subcommand
writes outline.yaml from JSON outlines; the plan subcommand writes a page's blueprint from its v1
plan and framing record. Neither overwrites a file.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return usageError(cmd, "convert needs a subcommand: outline or plan")
		},
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.AddCommand(newConvertOutlineCmd(), newConvertPlanCmd())
	return cmd
}

func newConvertOutlineCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "outline [flags] <outline.json>...",
		Short: "Write outline.yaml from JSON outlines",
		Long: `Merge the JSON outlines into one outline.yaml: pages in file order, cross-links in file
order, and one type declaration for each page type used. The type's anatomy is left for a person to
name. The result goes to --out, or to standard output.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usageError(cmd, "convert outline needs at least one JSON outline")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error { return runConvertOutline(cmd, args, out) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&out, "out", "", "write the outline to this file instead of standard output")
	return cmd
}

func runConvertOutline(cmd *cobra.Command, files []string, out string) error {
	in, err := readNamed(files)
	if err != nil {
		return err
	}
	data, err := blueprint.ConvertOutline(in)
	if err != nil {
		return err
	}
	if out == "" {
		_, err = cmd.OutOrStdout().Write(data)
		return err
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("write outline: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write outline: %w", err)
	}
	return f.Close()
}

func readNamed(files []string) ([]blueprint.NamedData, error) {
	var in []blueprint.NamedData
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read outline: %w", err)
		}
		in = append(in, blueprint.NamedData{Name: f, Data: data})
	}
	return in, nil
}

type convertPlanFlags struct {
	framing string
	outline []string
	root    string
}

func newConvertPlanCmd() *cobra.Command {
	var f convertPlanFlags
	cmd := &cobra.Command{
		Use:   "plan [flags] <plan.yaml>",
		Short: "Write a page's blueprint from its v1 plan and framing record",
		Long: `Convert a v1 plan, with its framing record, to a blueprint at the page's place under
the blueprint root. The page's path comes from the plan, and its exemplars from its entry in the JSON
outline. Standard output gets the blueprint's path. Standard error gets what the conversion could
not decide, the plan's friction lines for the project's friction log, and the framing record's
departures for the commit message.

When the page file exists, convert also seeds the page's drafted.lock entry from the page as
committed at HEAD. It refuses a page with uncommitted changes and a page the lock already has.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(cmd, "convert plan needs exactly one plan file")
			}
			if len(f.outline) == 0 {
				return usageError(cmd, "convert plan needs --outline")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error { return runConvertPlan(cmd, args[0], &f) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.framing, "framing", "", "the page's framing record")
	cmd.Flags().StringArrayVar(&f.outline, "outline", nil, "the chain's JSON outline that lists the page (repeatable)")
	cmd.Flags().StringVar(&f.root, "root", "", "the repository root (default: the git top level of the plan)")
	return cmd
}

func runConvertPlan(cmd *cobra.Command, planFile string, f *convertPlanFlags) error {
	data, err := os.ReadFile(planFile)
	if err != nil {
		return fmt.Errorf("read plan: %w", err)
	}
	p, findings := plan.Decode(planFile, data)
	if plan.HasError(findings) || p == nil {
		var w strings.Builder
		for _, x := range findings {
			fmt.Fprintf(&w, "%s:%d: %s: %s\n", x.File, x.Line, x.Severity, x.Message)
		}
		if _, err := io.WriteString(cmd.ErrOrStderr(), w.String()); err != nil {
			return err
		}
		return errors.New("the plan does not decode; fix it before converting")
	}
	pagePath := path.Clean(filepath.ToSlash(p.Page.Path))
	if !filepath.IsLocal(pagePath) {
		return fmt.Errorf("the plan's page path %q is not inside the repository", p.Page.Path)
	}

	in := blueprint.PlanInput{Plan: p}
	if f.framing != "" {
		if in.Framing, err = os.ReadFile(f.framing); err != nil {
			return fmt.Errorf("read framing record: %w", err)
		}
	}
	files, err := readNamed(f.outline)
	if err != nil {
		return err
	}
	jo, err := blueprint.ParseJSONOutlines(files)
	if err != nil {
		return err
	}
	var listed bool
	if in.Exemplars, listed = jo.Exemplars(pagePath); !listed {
		return fmt.Errorf("the outline does not list the page %s", pagePath)
	}

	root := f.root
	if root == "" {
		if root, err = blueprint.RepoRoot(filepath.Dir(planFile)); err != nil {
			return err
		}
	}
	conv, err := blueprint.ConvertPlan(in)
	if err != nil {
		return err
	}
	proj, err := projectFor(root)
	if err != nil {
		return err
	}
	layout := blueprint.Layout{BlueprintRoot: proj.BlueprintRoot, LockPath: path.Join(path.Dir(proj.Outline), blueprint.LockFile)}
	res, err := blueprint.InstallConverted(root, layout, pagePath, conv.Data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), res.Blueprint); err != nil {
		return err
	}
	_, err = io.WriteString(cmd.ErrOrStderr(), report(conv, res, pagePath))
	return err
}

// report returns what the conversion left to a person. It is status text, so the caller sends it
// to stderr and leaves standard output to the blueprint's path.
func report(conv *blueprint.Converted, res blueprint.Installed, pagePath string) string {
	var sb strings.Builder
	w := &sb
	if res.Seeded {
		fmt.Fprintf(w, "seeded the drafted.lock entry for %s\n", pagePath)
	} else {
		fmt.Fprintf(w, "no page file at %s, so no drafted.lock entry was seeded\n", pagePath)
	}
	if len(conv.Friction) > 0 {
		fmt.Fprintln(w, "friction (dropped from the blueprint; add to the project's friction log):")
		for _, l := range conv.Friction {
			fmt.Fprintf(w, "  - %s\n", l)
		}
	}
	if conv.Preamble != "" {
		fmt.Fprintln(w, "framing preamble (dropped from the blueprint; put in the conversion's commit message):")
		fmt.Fprintln(w, conv.Preamble)
	}
	if conv.Departures != "" {
		fmt.Fprintln(w, "departures (dropped from the blueprint; put in the conversion's commit message):")
		fmt.Fprintln(w, conv.Departures)
	}
	fmt.Fprintln(w, "needs judgment:")
	for _, l := range conv.Judgment {
		fmt.Fprintf(w, "  - %s\n", l)
	}
	return sb.String()
}

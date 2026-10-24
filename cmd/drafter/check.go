package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/plan"
	"github.com/spf13/cobra"
)

type checkFlags struct {
	root  string
	json  bool
	facts []string
}

func newCheckCmd() *cobra.Command {
	var f checkFlags
	cmd := &cobra.Command{
		Use:   "check [flags] [<blueprint>...]",
		Short: "Validate the project's blueprints",
		Long: `Validate every blueprint under the project's blueprint root, or the ones named, offline.
It decodes each blueprint, runs the plan checks on its view (ids, reader state, recovery,
links, cross-links, and fact citability), and adds the blueprint checks: each managed page
has one blueprint, every [[id]] reference resolves, every section says why, every use opens
with Only or Omit, page types declare an anatomy, drafted.lock agrees with the disk and
the outline. The handler check warns when a failure is handled by a fact, a verbatim sentence,
or a link or handoff back to the section of the failing step, since none gives the reader a
next action. With no blueprint named it also verifies the facts store: each fact's id, tag, and
source pointers against the repository. Findings print as file:line: severity: check: message
(fix). The exit status is 0 when no finding is an error, 1 when any is, and 2 when the command
could not run.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runCheck(cmd, args, &f) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.root, "root", "", "project root (default: the git top level of the working directory)")
	cmd.Flags().BoolVar(&f.json, "json", false, "print the findings as one JSON array")
	cmd.Flags().StringArrayVar(&f.facts, "fact", nil, "check only this fact's source pointers; repeat for several, and no blueprint is checked")
	return cmd
}

// projectFor resolves the project root and the blueprint settings for a command. Every error is
// one the command cannot run past, so it comes back as exit status 2.
func projectFor(rootFlag string) (blueprint.Project, error) {
	root := rootFlag
	if root == "" {
		var err error
		if root, err = plan.GitRoot("."); err != nil {
			return blueprint.Project{}, exitError{code: exitCannotRun, msg: err.Error()}
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return blueprint.Project{}, exitError{code: exitCannotRun, msg: err.Error()}
	}
	cfg, err := config.Load(root, config.Flags{})
	if err != nil {
		return blueprint.Project{}, exitError{code: exitCannotRun, msg: err.Error()}
	}
	return blueprint.Project{
		Root:          root,
		BlueprintRoot: path.Clean(cfg.Blueprints.Root),
		Outline:       path.Clean(cfg.Blueprints.Outline),
		FactsDir:      cfg.Facts.Dir,
		Required:      cfg.Blueprints.Required,
		BriefPath:     cfg.Blueprints.BriefPath,
		ProseSources:  cfg.Facts.ProseSources,
		SentenceWords: cfg.Check.SentenceWords,
	}, nil
}

// repoRelative turns a file argument, relative to the working directory, into a slash path
// relative to root.
func repoRelative(root, arg string) (string, error) {
	abs, err := filepath.Abs(arg)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%s is outside the project root %s", arg, root)
	}
	return filepath.ToSlash(rel), nil
}

// checkNoExcerpt names the warning for a verified fact whose pointers show a read no source lines.
const checkNoExcerpt = "no-excerpt"

// noExcerptFindings warns on each verified fact that a checked blueprint cites and whose
// pointers yield no excerpt, once per fact however many blueprints cite it. It asks the
// assemble package the question a read's request asks, so the warning and the read agree. A
// project with no facts store has no such facts.
func noExcerptFindings(ctx context.Context, p blueprint.Project, named []string) ([]plan.Finding, error) {
	if p.FactsDir == "" {
		return nil, nil
	}
	facts, err := plan.LoadFacts(p.FactsDir)
	if err != nil {
		return nil, err
	}
	files := named
	if len(named) == 0 {
		if files, err = blueprintFiles(p); err != nil {
			return nil, err
		}
	}
	cited := map[string]bool{}
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(rel)))
		if err != nil {
			continue // Check already reported a file it could not read
		}
		if src, _ := blueprint.Load(rel, data); src != nil {
			var ids []string
			switch b := src.(type) {
			case *blueprint.Blueprint:
				ids = blueprint.CitedFacts(b, facts)
			case *blueprint.Markdown:
				ids = b.FactIDs(facts)
			}
			for _, id := range ids {
				cited[id] = true
			}
		}
	}
	if len(cited) == 0 {
		return nil, nil
	}
	ex, err := excerpterFor(ctx, p)
	if err != nil {
		// Without tracked files, which only a git work tree has, no pointer can yield an
		// excerpt, and warning on every fact would only repeat that.
		return nil, nil
	}
	var out []plan.Finding
	for _, id := range slices.Sorted(maps.Keys(cited)) {
		f, ok := facts.Get(id)
		if !ok || f.Tag != "verified" {
			continue
		}
		_, real, err := ex.Fact(facts, id)
		if err != nil {
			return nil, err
		}
		if real {
			continue
		}
		out = append(out, plan.Finding{
			File: path.Join(filepath.ToSlash(mustRel(p.Root, p.FactsDir)), f.File), Line: f.Line,
			Severity: plan.SeverityWarning, Check: checkNoExcerpt,
			Message: "verified fact " + id + " has no pointer that yields an excerpt" + commentaryNote(f.Commentary) + ", so a read cannot check it against source",
			Fix:     "point it at the code that shows the claim; `drafter excerpt " + id + "` shows what a read sees",
		})
	}
	return out, nil
}

// mustRel returns dir relative to root, or dir itself when it lies elsewhere.
func mustRel(root, dir string) string {
	if rel, err := filepath.Rel(root, dir); err == nil {
		return rel
	}
	return dir
}

// blueprintFiles lists the repo-relative slash paths of every blueprint under the blueprint root.
func blueprintFiles(p blueprint.Project) ([]string, error) {
	var files []string
	err := filepath.WalkDir(filepath.Join(p.Root, filepath.FromSlash(p.BlueprintRoot)), func(file string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(p.Root, file)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if _, ok := blueprint.PagePath(p.BlueprintRoot, rel); ok {
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}

func runCheck(cmd *cobra.Command, args []string, f *checkFlags) error {
	p, err := projectFor(f.root)
	if err != nil {
		return err
	}
	if len(f.facts) > 0 {
		if len(args) > 0 {
			return usageError(cmd, "--fact checks the facts store and takes no blueprint")
		}
		return runCheckFacts(cmd, p, f)
	}
	named := make([]string, 0, len(args))
	for _, a := range args {
		rel, err := repoRelative(p.Root, a)
		if err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
		named = append(named, rel)
	}
	findings, err := p.Check(named)
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	noExcerpts, err := noExcerptFindings(cmd.Context(), p, named)
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	findings = append(findings, noExcerpts...)
	storeLine := ""
	if len(named) == 0 && p.FactsDir != "" {
		store, checked, skipped, err := plan.VerifyFacts(p.FactsDir, p.Root, nil)
		if err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
		findings = append(findings, store...)
		storeLine = fmt.Sprintf("facts store: %d pointers checked, %d skipped\n", checked, skipped)
	}
	return reportFindings(cmd, f, findings, storeLine)
}

// runCheckFacts verifies the source pointers of the facts f.facts names and nothing else. An id
// the store does not hold is an error finding.
func runCheckFacts(cmd *cobra.Command, p blueprint.Project, f *checkFlags) error {
	if p.FactsDir == "" {
		return exitError{code: exitCannotRun, msg: "no facts store; set facts.dir, or create the blueprints/facts directory"}
	}
	facts, err := plan.LoadFacts(p.FactsDir)
	if err != nil {
		return exitError{code: exitCannotRun, msg: err.Error()}
	}
	var findings []plan.Finding
	var ids []string
	for _, id := range f.facts {
		switch {
		case facts.Count(id) > 0:
			ids = append(ids, id)
		case facts.Count("f:"+id) > 0:
			ids = append(ids, "f:"+id)
		default:
			findings = append(findings, plan.Finding{
				File: filepath.ToSlash(mustRel(p.Root, p.FactsDir)), Line: 1, Severity: plan.SeverityError, Check: "store",
				Message: fmt.Sprintf("fact %s is not in the facts store", id), Fix: "name an id from the facts store",
			})
		}
	}
	var checked, skipped int
	if len(ids) > 0 { // VerifyFacts reads a nil id list as the whole store
		store, c, s, err := plan.VerifyFacts(p.FactsDir, p.Root, ids)
		if err != nil {
			return exitError{code: exitCannotRun, msg: err.Error()}
		}
		findings, checked, skipped = append(findings, store...), c, s
	}
	return reportFindings(cmd, f, findings, fmt.Sprintf("facts store: %d pointers checked, %d skipped\n", checked, skipped))
}

// reportFindings prints the findings and their count, and returns the exit status they call for.
func reportFindings(cmd *cobra.Command, f *checkFlags, findings []plan.Finding, storeLine string) error {
	out, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	if f.json {
		if err := writeJSON(out, findings); err != nil {
			return err
		}
	} else {
		var b strings.Builder
		for _, x := range findings {
			fmt.Fprintf(&b, "%s:%d: %s: %s: %s (%s)\n", x.File, x.Line, x.Severity, x.Check, x.Message, x.Fix)
		}
		if _, err := io.WriteString(out, b.String()); err != nil {
			return err
		}
	}
	errs, warns := count(findings)
	if _, err := io.WriteString(stderr, storeLine+plural(errs, "error")+", "+plural(warns, "warning")+"\n"); err != nil {
		return err
	}
	if errs > 0 {
		return exitError{code: exitFindings}
	}
	return nil
}

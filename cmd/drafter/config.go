package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/plan"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	var flags config.Flags
	cmd := &cobra.Command{
		Use:   "config [flags]",
		Short: "Print every resolved setting and where it came from",
		Long: `Print each setting of the project the command runs in as key = value (source), where
the source is the layer, variable, or flag that set it. A cap also lists every source that set
it. The project is the git top level when it holds a drafter.toml; otherwise only the user
layer applies. key_command and key_env print by value, never a key. With --json the rows print
as one array. The exit status is 0 on success and 1 when a layer is refused or a setting the
command needs is missing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runConfig(cmd, flags) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().BoolVar(&flags.JSON, "json", false, "print the settings as one JSON array")
	cmd.Flags().BoolVar(&flags.Sync, "sync", false, "resolve mode as sync")
	return cmd
}

// configRow is one setting in the JSON output.
type configRow struct {
	Key     string   `json:"key"`
	Value   string   `json:"value"`
	Source  string   `json:"source"`
	Sources []string `json:"sources,omitempty"`
}

func runConfig(cmd *cobra.Command, flags config.Flags) error {
	root, err := plan.GitRoot(".")
	if err != nil {
		root = "" // outside a git tree only the user layer applies
	}
	c, err := config.Load(root, flags)
	if err != nil {
		return err
	}
	rows := c.Sources()
	out := cmd.OutOrStdout()
	if c.Output == "json" {
		err = writeConfigJSON(out, rows)
	} else {
		err = writeConfigText(out, rows)
	}
	if err != nil {
		return err
	}
	scope := config.ScopeMoney
	if c.Root != "" {
		scope = config.ScopeRun
	}
	return c.Validate(scope)
}

func writeConfigJSON(w io.Writer, rows []config.Source) error {
	out := make([]configRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, configRow{Key: r.Key, Value: r.Value, Source: r.From, Sources: r.Sets})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func writeConfigText(w io.Writer, rows []config.Source) error {
	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b, "%s = %s  (%s", r.Key, r.Value, r.From)
		if len(r.Sets) > 0 {
			fmt.Fprintf(&b, "; set by %s", strings.Join(r.Sets, ", "))
		}
		b.WriteString(")\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

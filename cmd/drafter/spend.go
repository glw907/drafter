package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/ledger"
	"github.com/glw907/drafter/internal/plan"
	"github.com/spf13/cobra"
)

const dateLayout = "2006-01-02"

type spendFlags struct {
	since, until string
	json         bool
}

func newSpendCmd() *cobra.Command {
	var f spendFlags
	cmd := &cobra.Command{
		Use:   "spend [flags]",
		Short: "Print the cost recorded in the ledger by repo and seat",
		Long: `Print what the ledger recorded as spent, by repo and seat, with a total. The window is the
current billing cycle unless --since or --until name UTC dates; --until is inclusive. Lines
settled at an estimate are marked. Open reservations print on a line of their own at their
worst case and are never part of the cost. With --json the report prints as one object with
rows, total, and open. The exit status is 0 on success and 1 when the ledger is unreadable.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runSpend(cmd, f) },
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(cmd, err.Error()) })
	cmd.Flags().StringVar(&f.since, "since", "", "first UTC date to count, YYYY-MM-DD (default: the cycle start)")
	cmd.Flags().StringVar(&f.until, "until", "", "last UTC date to count, YYYY-MM-DD, inclusive (default: now)")
	cmd.Flags().BoolVar(&f.json, "json", false, "print the report as one JSON object")
	return cmd
}

// spendRow and spendReport are the JSON shape of the report.
type spendRow struct {
	Repo      string  `json:"repo"`
	Seat      string  `json:"seat"`
	Cost      float64 `json:"cost"`
	Requests  int     `json:"requests"`
	Estimated int     `json:"estimated"`
}

type spendReport struct {
	Rows  []spendRow `json:"rows"`
	Total float64    `json:"total"`
	Open  struct {
		Count int     `json:"count"`
		Worst float64 `json:"worst"`
	} `json:"open"`
}

func runSpend(cmd *cobra.Command, f spendFlags) error {
	var since, until time.Time
	var err error
	if f.since != "" {
		if since, err = time.Parse(dateLayout, f.since); err != nil {
			return usageError(cmd, fmt.Sprintf("--since %q is not a YYYY-MM-DD date", f.since))
		}
	}
	if f.until != "" {
		if until, err = time.Parse(dateLayout, f.until); err != nil {
			return usageError(cmd, fmt.Sprintf("--until %q is not a YYYY-MM-DD date", f.until))
		}
		until = until.AddDate(0, 0, 1)
	}
	root, err := plan.GitRoot(".")
	if err != nil {
		root = "" // outside a git tree only the user layer applies
	}
	c, err := config.Load(root, config.Flags{JSON: f.json})
	if err != nil {
		return err
	}
	if f.since == "" {
		since = ledger.CycleStart(time.Now(), c.BillingCycleDay)
	}
	rep, err := ledger.Open(c.StateDir).Spend(since, until)
	if err != nil {
		return err
	}
	if c.Output == "json" {
		return writeSpendJSON(cmd.OutOrStdout(), rep)
	}
	return writeSpendText(cmd.OutOrStdout(), rep)
}

func writeSpendJSON(w io.Writer, rep ledger.Report) error {
	out := spendReport{Rows: make([]spendRow, 0, len(rep.Rows)), Total: rep.Total}
	for _, r := range rep.Rows {
		out.Rows = append(out.Rows, spendRow{r.Repo, r.Seat, r.Cost, r.Requests, r.Estimated})
	}
	out.Open.Count, out.Open.Worst = rep.Open.Count, rep.Open.Worst
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func writeSpendText(w io.Writer, rep ledger.Report) error {
	repoW, seatW := len("repo"), len("seat")
	for _, r := range rep.Rows {
		repoW, seatW = max(repoW, len(r.Repo)), max(seatW, len(r.Seat))
	}
	var b strings.Builder
	if len(rep.Rows) > 0 {
		fmt.Fprintf(&b, "%-*s  %-*s  %10s  %8s\n", repoW, "repo", seatW, "seat", "cost", "requests")
	}
	for _, r := range rep.Rows {
		fmt.Fprintf(&b, "%-*s  %-*s  %10.4f  %8d", repoW, r.Repo, seatW, r.Seat, r.Cost, r.Requests)
		if r.Estimated > 0 {
			fmt.Fprintf(&b, "  (%d estimated)", r.Estimated)
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "total %.4f\n", rep.Total)
	if n := rep.Open.Count; n > 0 {
		noun := "reservations"
		if n == 1 {
			noun = "reservation"
		}
		fmt.Fprintf(&b, "open %d %s, worst %.4f\n", n, noun, rep.Open.Worst)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

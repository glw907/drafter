//go:build smoke

package smoke

import (
	"context"
	"testing"
	"time"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/key"
	"github.com/glw907/drafter/internal/ledger"
)

// TestLive runs the smoke against the real API with the user's configuration and key. It spends
// money: one batch of eight requests and one sync request on Haiku 5.5, priced from the user's
// price table.
func TestLive(t *testing.T) {
	cfg, err := config.Load("", config.Flags{})
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}
	if err := cfg.Validate(config.ScopeMoney); err != nil {
		t.Fatalf("configuration: %v", err)
	}
	if _, ok := cfg.Prices.Models[Model]; !ok {
		t.Fatalf("prices.models has no entry for %s", Model)
	}
	apiKey, err := key.Resolve(cfg)
	if err != nil {
		t.Fatalf("resolve key: %v", err)
	}
	p := Params{
		Options:  batch.Options{Key: apiKey, BaseURL: cfg.APIBaseURL, WorkspaceID: cfg.WorkspaceID},
		StateDir: cfg.StateDir,
		Model:    Model,
		Prices:   cfg.Prices,
		Caps: ledger.Caps{
			Page:       cfg.PageCap,
			Cycle:      cfg.CycleCeiling,
			CycleStart: ledger.CycleStart(time.Now(), cfg.BillingCycleDay),
		},
	}
	proj, err := Project(t.TempDir())
	if err != nil {
		t.Fatalf("synthetic project: %v", err)
	}
	shapes, err := Shapes(context.Background(), proj)
	if err != nil {
		t.Fatalf("shapes: %v", err)
	}
	rep, err := Run(context.Background(), p, shapes)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, problem := range rep.Problems {
		t.Error(problem)
	}
	t.Logf("run %s, batch %s: %d results, resumed %v", rep.RunID, rep.BatchID, len(rep.Results), rep.Resumed)
}

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

// haikuModel is Haiku 5.5's model ID, from the claude-api skill's model table.
const haikuModel = "claude-haiku-5-5"

// TestLive runs the smoke against the real API with the user's configuration and key. It spends
// money: one batch of seven requests and one sync request on Haiku 5.5, priced from the user's
// price table.
func TestLive(t *testing.T) {
	cfg, err := config.Load("", config.Flags{})
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}
	if err := cfg.Validate(config.ScopeMoney); err != nil {
		t.Fatalf("configuration: %v", err)
	}
	if _, ok := cfg.Prices.Models[haikuModel]; !ok {
		t.Fatalf("prices.models has no entry for %s", haikuModel)
	}
	apiKey, err := key.Resolve(cfg)
	if err != nil {
		t.Fatalf("resolve key: %v", err)
	}
	p := Params{
		Options:  batch.Options{Key: apiKey, BaseURL: cfg.APIBaseURL, WorkspaceID: cfg.WorkspaceID},
		StateDir: cfg.StateDir,
		Model:    haikuModel,
		Prices:   cfg.Prices,
		Caps: ledger.Caps{
			Page:       cfg.PageCap,
			Cycle:      cfg.CycleCeiling,
			CycleStart: ledger.CycleStart(time.Now(), cfg.BillingCycleDay),
		},
	}
	rep, err := Run(context.Background(), p, Shapes())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, problem := range rep.Problems {
		t.Error(problem)
	}
	t.Logf("run %s, batch %s: %d results, resumed %v", rep.RunID, rep.BatchID, len(rep.Results), rep.Resumed)
}

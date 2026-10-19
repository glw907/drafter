package pipeline

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden request under testdata")

// TestBlueprintReadRequestGolden pins the stage-1 blueprint read request of the blueprint harness.
// The golden was captured before the drafting pass changed the pipeline, so the read's bytes stay
// what they were. The custom_id carries the run id and is blanked.
func TestBlueprintReadRequestGolden(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"))
	h.mustRun()
	reqs := requestsFor(h, "s1", "blueprint")
	if len(reqs) != 1 {
		t.Fatalf("blueprint requests = %d, want 1", len(reqs))
	}
	r := reqs[0]
	r.CustomID = ""
	got, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	golden := filepath.Join("testdata", "blueprint-read-request.golden.json")
	if *updateGolden {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("the blueprint read request differs from %s\n got: %s\nwant: %s", golden, got, want)
	}
}

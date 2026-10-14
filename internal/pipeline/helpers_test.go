package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/handoff"
	"github.com/glw907/drafter/internal/run"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func checksum(root, plan string) (string, error) { return handoff.Checksum(root, plan, "") }

func hash12Of(pagePath string) string {
	sum := sha256.Sum256([]byte(run.PageKey(pagePath)))
	return hex.EncodeToString(sum[:])[:12]
}

func pageKey(p string) string { return run.PageKey(p) }

// stateOnDisk loads the state of the one run in the state directory.
func (h *harness) stateOnDisk() *run.State { return h.state(h.runID()) }

// normalizeRecord returns a page's record.json with the parts that differ between runs
// replaced: the custom_id prefix and the input hashes.
func normalizeRecord(t *testing.T, runDir, pagePath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runDir, run.PageKey(pagePath), "record.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if hashes, ok := m["input_hashes"].(map[string]any); ok {
		for k := range hashes {
			hashes[k] = "sha256:<hash>"
		}
	}
	if stages, ok := m["stages"].([]any); ok {
		for _, s := range stages {
			st, _ := s.(map[string]any)
			id, _ := st["custom_id"].(string)
			parts := strings.SplitN(id, "_", 3)
			st["custom_id"] = "<run>_<page>_" + parts[2]
		}
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(out) + "\n"
}

// record loads a page's record.json from the run directory.
func (h *harness) record(runID, pagePath string) record {
	h.t.Helper()
	data, err := os.ReadFile(filepath.Join(h.runDir(runID), run.PageKey(pagePath), "record.json"))
	if err != nil {
		h.t.Fatal(err)
	}
	var rec record
	if err := json.Unmarshal(data, &rec); err != nil {
		h.t.Fatal(err)
	}
	return rec
}

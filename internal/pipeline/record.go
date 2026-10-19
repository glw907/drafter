package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/glw907/drafter/internal/atomicfile"
	"github.com/glw907/drafter/internal/edits"
)

// record is a page's record.json: what the run did for it and why it ended as it did.
type record struct {
	Page         string            `json:"page"`
	Brief        string            `json:"brief"`
	Plan         string            `json:"plan"`
	Mode         string            `json:"mode"`
	PlanFormat   string            `json:"plan_format"`
	Facts        string            `json:"facts"`
	SeatsNotRun  []string          `json:"seats_not_run"`
	InputHashes  map[string]string `json:"input_hashes"`
	Stages       []stageRecord     `json:"stages"`
	Verdicts     []verdictRecord   `json:"verdicts"`
	Findings     []findingRecord   `json:"findings"`
	Resolutions  []resolutionEntry `json:"resolutions"`
	Faults       []string          `json:"faults"`
	Friction     []string          `json:"friction"`
	Warnings     []string          `json:"warnings"`
	Escalation   *escalation       `json:"escalation"`
	Cost         float64           `json:"cost"`
	FinalStatus  string            `json:"final_status"`
	FinalHashes  map[string]string `json:"final_hashes"`
	GateExcerpts []string          `json:"gate_excerpts"`
}

// stageRecord is one submitted request.
type stageRecord struct {
	Stage    string `json:"stage"`
	Seat     string `json:"seat"`
	CustomID string `json:"custom_id"`
	Outcome  string `json:"outcome"`
}

// verdictRecord is one read's verdict.
type verdictRecord struct {
	Stage   string `json:"stage"`
	Seat    string `json:"seat"`
	Verdict string `json:"verdict"`
}

// findingRecord is one numbered finding.
type findingRecord struct {
	Stage    string `json:"stage"`
	Seat     string `json:"seat"`
	N        int    `json:"n"`
	Location string `json:"location"`
	Text     string `json:"text"`
	Blocking bool   `json:"blocking"`
}

// resolutionEntry is a resolution with the stage that produced it.
type resolutionEntry struct {
	Stage   string `json:"stage"`
	Finding int    `json:"finding"`
	Status  string `json:"status"`
	Edits   []int  `json:"edits"`
	Reason  string `json:"reason"`
}

// escalation names the stop rule that ended a page, the seat that triggered it, and the
// numbers of the findings that did.
type escalation struct {
	Rule     string `json:"rule"`
	Seat     string `json:"seat"`
	Findings []int  `json:"findings"`
	Detail   string `json:"detail,omitempty"`
}

func newRecord() record {
	return record{
		SeatsNotRun: []string{}, InputHashes: map[string]string{}, Stages: []stageRecord{},
		Verdicts: []verdictRecord{}, Findings: []findingRecord{}, Resolutions: []resolutionEntry{},
		Faults: []string{}, Friction: []string{}, Warnings: []string{},
		FinalHashes: map[string]string{}, GateExcerpts: []string{},
	}
}

func (r *record) fault(format string, a ...any) {
	r.Faults = append(r.Faults, fmt.Sprintf(format, a...))
}

func (r *record) warn(s string) { r.Warnings = append(r.Warnings, s) }

// alreadyPresentPrefix starts a finding, or a warning for a conflict, about a defect the change
// being read did not introduce.
const alreadyPresentPrefix = "Already present:"

// unplacedPrefix starts the warning for a change read's consistency conflict that names neither a
// fact nor a with.
const unplacedPrefix = "Unplaced:"

// alreadyPresent counts the blueprint read's findings that report an older defect.
func (r *record) alreadyPresent() int {
	n := 0
	for _, f := range r.Findings {
		if f.Seat == "blueprint" && strings.HasPrefix(strings.TrimSpace(f.Text), alreadyPresentPrefix) {
			n++
		}
	}
	return n
}

// blueprintFindings lists the blueprint read's findings for the summary, nil when it made none.
func (r *record) blueprintFindings() []FindingResult {
	var out []FindingResult
	for _, f := range r.Findings {
		if f.Seat == "blueprint" {
			out = append(out, FindingResult{Location: f.Location, Text: f.Text, Blocking: f.Blocking})
		}
	}
	return out
}

func (r *record) verdict(stage, seat, v string) {
	r.Verdicts = append(r.Verdicts, verdictRecord{Stage: stage, Seat: seat, Verdict: v})
}

func (r *record) findings(stage string, fs []edits.Finding) {
	for _, f := range fs {
		r.Findings = append(r.Findings, findingRecord{
			Stage: stage, Seat: f.Seat, N: f.N, Location: f.Location, Text: f.Text, Blocking: f.Blocking,
		})
	}
}

func (r *record) resolutions(stage string, rs []edits.Resolution) {
	kept := r.Resolutions[:0:0]
	for _, old := range r.Resolutions {
		if old.Stage != stage {
			kept = append(kept, old)
		}
	}
	r.Resolutions = kept
	for _, x := range rs {
		r.Resolutions = append(r.Resolutions, resolutionEntry{
			Stage: stage, Finding: x.Finding, Status: x.Status, Edits: x.Edits, Reason: x.Reason,
		})
	}
}

// friction adds notes a seat reported, once each.
func (r *record) addFriction(seat string, notes []string) {
	for _, n := range notes {
		line := seat + ": " + n
		dup := false
		for _, f := range r.Friction {
			dup = dup || f == line
		}
		if !dup {
			r.Friction = append(r.Friction, line)
		}
	}
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func hashFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashBytes(b), nil
}

// writeJSON writes v to path as indented JSON.
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	if err := atomicfile.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}

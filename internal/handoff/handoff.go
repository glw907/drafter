// Package handoff reads the handoff that tells drafter everything about one planned page.
// A handoff arrives as a JSON file or inside a Workflow run file, and both decode to the one
// Handoff struct, each refused when it names a path outside its project or a plan and framing
// that differ from the ones the planner checked.
package handoff

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Version is the handoff format version this package reads.
const Version = 1

// Handoff is one page's handoff.
type Handoff struct {
	Version  int             `json:"version"`
	Project  string          `json:"project"`
	Page     Page            `json:"page"`
	Plan     string          `json:"plan"`
	Framing  string          `json:"framing,omitempty"`
	Inputs   Inputs          `json:"inputs"`
	Context  json.RawMessage `json:"context,omitempty"`
	Checksum string          `json:"checksum"`

	// File is where the handoff was read from, for errors. It is not decoded.
	File string `json:"-"`
}

// Page names the page and its brief, both relative to the project, and its type:
// task-guide, concept, or reference.
type Page struct {
	Path  string `json:"path"`
	Brief string `json:"brief"`
	Type  string `json:"type"`
}

// Inputs are the page inputs with the plan's and the framing's dispositions applied.
type Inputs struct {
	Job              string   `json:"job"`
	PageType         string   `json:"pageType,omitempty"`
	RegisterSections []string `json:"registerSections"`
	ExemplarSources  []string `json:"exemplarSources"`
	ClaimInventory   []Claim  `json:"claimInventory"`
	FactIDs          []string `json:"factIds,omitempty"`
}

// Claim is one entry of the claim inventory.
type Claim struct {
	Claim       string `json:"claim"`
	Disposition string `json:"disposition"`
	FactID      string `json:"factId,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Section     string `json:"section,omitempty"`
}

// ReadFiles decodes each handoff file strictly and refuses a file that carries an unknown
// version or field, a page, brief, plan, or framing path resolving outside its project, or a
// checksum that does not match the files on disk. It also refuses a set that names more than
// one project. Every refusal names the file.
func ReadFiles(paths ...string) ([]Handoff, error) {
	var out []Handoff
	for _, path := range paths {
		h, err := readFile(path)
		if err != nil {
			return nil, err
		}
		if err := h.check(); err != nil {
			return nil, err
		}
		if err := h.Verify(); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, sameProject(out)
}

func readFile(path string) (Handoff, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Handoff{}, fmt.Errorf("read handoff %s: %w", path, err)
	}
	var h Handoff
	if err := decodeStrict(data, &h); err != nil {
		return Handoff{}, fmt.Errorf("decode handoff %s: %w", path, err)
	}
	if h.Version != Version {
		return Handoff{}, fmt.Errorf("handoff %s: unknown version %d, want %d", path, h.Version, Version)
	}
	h.File = path
	return h, nil
}

func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing data after the JSON object")
	}
	return nil
}

// check refuses a path that resolves outside the project. The exemplar sources are exempt:
// the chain's are absolute paths under its exemplar root.
func (h Handoff) check() error {
	if !filepath.IsAbs(h.Project) {
		return fmt.Errorf("handoff %s: project %q is not an absolute path", h.File, h.Project)
	}
	for _, f := range []struct{ name, path string }{
		{"page.path", h.Page.Path},
		{"page.brief", h.Page.Brief},
		{"plan", h.Plan},
		{"framing", h.Framing},
	} {
		if f.name == "framing" && f.path == "" {
			continue
		}
		if f.path == "" {
			return fmt.Errorf("handoff %s: %s is empty", h.File, f.name)
		}
		if (f.name == "plan" || f.name == "framing") && filepath.IsAbs(f.path) {
			return fmt.Errorf("handoff %s: %s %q must be relative to the project", h.File, f.name, f.path)
		}
		if outside(h.Project, f.path) {
			return fmt.Errorf("handoff %s: %s %q resolves outside project %s", h.File, f.name, f.path, h.Project)
		}
	}
	return nil
}

func outside(project, path string) bool {
	if !filepath.IsAbs(path) {
		path = filepath.Join(project, path)
	}
	rel, err := filepath.Rel(project, path)
	return err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func sameProject(hs []Handoff) error {
	for _, h := range hs[min(1, len(hs)):] {
		if h.Project != hs[0].Project {
			return fmt.Errorf("handoffs %s and %s name different projects (%s, %s); one run works one worktree",
				hs[0].File, h.File, hs[0].Project, h.Project)
		}
	}
	return nil
}

// Verify recomputes the checksum from the plan and framing on disk and reports a mismatch.
func (h Handoff) Verify() error {
	got, err := Checksum(h.Project, h.Plan, h.Framing)
	if err != nil {
		return fmt.Errorf("handoff %s: %w", h.File, err)
	}
	if got != h.Checksum {
		return fmt.Errorf("handoff %s: checksum %s does not match the plan and framing on disk (%s)", h.File, h.Checksum, got)
	}
	return nil
}

// Checksum returns "sha256:" and the hex SHA-256 of the plan's bytes followed by the framing's,
// the same as `cat plan framing | sha256sum`. The plan alone is hashed when framing is empty.
// Both paths are relative to project.
func Checksum(project, plan, framing string) (string, error) {
	h := sha256.New()
	for _, p := range []string{plan, framing} {
		if p == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(project, p))
		if err != nil {
			return "", fmt.Errorf("read %s for checksum: %w", p, err)
		}
		h.Write(data)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

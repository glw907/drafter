package run

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Prune deletes the run directories under stateDir older than retentionDays
// whose state has no Stopped reason and whose every page is accepted or
// escalated, and returns the paths it deleted. A run's age comes from its
// ID. A directory that isn't a run ID, or whose state is absent, malformed,
// or of another schema, is kept. Prune never touches the ledger files.
func Prune(stateDir string, retentionDays int, now time.Time) ([]string, error) {
	repos, err := os.ReadDir(filepath.Join(stateDir, "runs"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read runs: %w", err)
	}
	cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
	var deleted []string
	for _, repo := range repos {
		if !repo.IsDir() {
			continue
		}
		repoDir := filepath.Join(stateDir, "runs", repo.Name())
		runs, err := os.ReadDir(repoDir)
		if err != nil {
			return deleted, fmt.Errorf("read %s: %w", repoDir, err)
		}
		for _, r := range runs {
			ts, ok := idTime(r.Name())
			if !r.IsDir() || !ok || !ts.Before(cutoff) {
				continue
			}
			dir := filepath.Join(repoDir, r.Name())
			s, err := LoadState(dir)
			if err != nil || !finished(s) {
				continue
			}
			if err := os.RemoveAll(dir); err != nil {
				return deleted, fmt.Errorf("delete run %s: %w", dir, err)
			}
			deleted = append(deleted, dir)
		}
	}
	return deleted, nil
}

func finished(s *State) bool {
	if s.Stopped != "" {
		return false
	}
	for _, p := range s.Pages {
		if p == nil || (p.Status != "accepted" && p.Status != "escalated") {
			return false
		}
	}
	return true
}

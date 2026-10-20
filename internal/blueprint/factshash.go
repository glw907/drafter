package blueprint

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
	"strconv"

	"github.com/glw907/drafter/internal/plan"
)

// factHashLen is the number of hex digits of a fact's SHA-256 that the lock keeps.
const factHashLen = 12

// FactsHashes maps each of ids to the first 12 hex digits of the SHA-256 of its bullet's Raw text,
// which keeps the tag qualifier and every pointer span, so a moved pointer changes the hash. It is
// the only place the lock's facts map is computed. An id the store lacks is left out, and so is
// every id when there is no store; the result is nil when it holds nothing.
//
// An id that more than one bullet holds is a facts error that drafter check reports before any
// read, and the store's accessors do not expose its bullets, so the hash covers the count alone.
func FactsHashes(ids []string, facts *plan.Facts) map[string]string {
	if facts == nil {
		return nil
	}
	var out map[string]string
	for _, id := range ids {
		var raw string
		switch n := facts.Count(id); n {
		case 0:
			continue
		case 1:
			f, _ := facts.Get(id)
			raw = f.Raw
		default:
			raw = "duplicate x" + strconv.Itoa(n)
		}
		sum := sha256.Sum256([]byte(raw))
		if out == nil {
			out = map[string]string{}
		}
		out[id] = hex.EncodeToString(sum[:])[:factHashLen]
	}
	return out
}

// ChangedFacts lists, sorted, the ids whose hash differs between a lock entry's recorded facts and
// the current ones: ids in both with different hashes, ids only the current set has, and ids only
// the recorded set holds, which the blueprint no longer cites.
func ChangedFacts(recorded, current map[string]string) []string {
	var out []string
	for id, h := range current {
		if r, ok := recorded[id]; !ok || r != h {
			out = append(out, id)
		}
	}
	for id := range recorded {
		if _, ok := current[id]; !ok {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// SectionsCiting lists, in blueprint order, the ids of the sections whose items place or cite any
// of ids.
func SectionsCiting(b *Blueprint, ids []string) []string {
	var out []string
	for i := range b.Sections {
		s := &b.Sections[i]
		if sectionCites(s, func(id string) bool { return slices.Contains(ids, id) }) {
			out = append(out, s.ID)
		}
	}
	return out
}

// FactsMatch reports whether a lock entry's recorded facts are the current ones. A missing map
// matches an empty current set, which no read can record past omitempty.
func FactsMatch(recorded, current map[string]string) bool {
	return maps.Equal(recorded, current) || len(recorded) == 0 && len(current) == 0
}

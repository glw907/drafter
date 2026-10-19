package config

import (
	"maps"
	"math"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// seatNames are the nine seats a configuration may name. A default run lists the first seven;
// the blueprint seat runs under drafter read and drafter page on a blueprint, and the coherence
// seat runs on every section redraft. Neither is governed by seats, so each command validates the
// seat itself.
var seatNames = []string{"draft", "structure", "editor", "fact", "reader", "coverage", "consistency", "blueprint", "coherence"}

// defaultSeats are the seats a run lists when no layer sets seats.
var defaultSeats = seatNames[:7]

// placeholders are the names a command template may use.
var placeholders = []string{"page", "brief", "slug", "baseline"}

// checkValues refuses the first setting in the layer whose value the spec does not allow. It
// checks what one layer can settle on its own; gate.lane against the resolved protocol is
// checked after the layers merge.
func (l *layer) checkValues() error {
	f := l.f
	if err := firstErr(
		l.oneOf("mode", f.Mode, "batch", "sync"),
		l.oneOf("output", f.Output, "human", "json"),
		l.atLeast("billing_cycle_day", f.BillingCycleDay, 1),
		l.atMost("billing_cycle_day", f.BillingCycleDay, 31),
		l.positive("page_cap", f.PageCap),
		l.positive("cycle_ceiling", f.CycleCeiling),
		l.atLeast("pages_in_flight", f.PagesInFlight, 1),
		l.atLeast("retention_days", f.RetentionDays, 1),
	); err != nil {
		return err
	}
	if r := f.Retries; r != nil {
		if err := firstErr(
			l.atLeast("retries.gate_fix", r.GateFix, 0),
			l.atLeast("retries.edit_contract", r.EditContract, 0),
		); err != nil {
			return err
		}
	}
	if p := f.Prices; p != nil && p.BatchDiscount != nil {
		if d := *p.BatchDiscount; !(d >= 0 && d < 1) {
			return l.fail("prices.batch_discount", "must be from 0 up to but not including 1, not %v", d)
		}
	}
	if f.Seats != nil {
		if !slices.Contains(*f.Seats, "draft") {
			return l.fail("seats", "must include draft, which always runs")
		}
		for _, name := range *f.Seats {
			if !slices.Contains(seatNames, name) {
				return l.fail("seats", "names %q, which is not a seat; the seats are %s",
					name, strings.Join(seatNames, ", "))
			}
		}
	}
	if err := l.checkGate(); err != nil {
		return err
	}
	if b := f.Facts; b != nil && b.BulletFormat != nil && *b.BulletFormat != "v1" {
		return l.fail("facts.bullet_format", "must be v1, the only bullet format, not %q", *b.BulletFormat)
	}
	if err := l.checkBlueprints(); err != nil {
		return err
	}
	return l.checkSeats()
}

// checkBlueprints refuses a blueprint path that is not a clean path inside the project. The
// settings are project-relative, never resolved against the layer's directory, because the path
// rule joins them to page paths.
func (l *layer) checkBlueprints() error {
	b := l.f.Blueprints
	if b == nil {
		return nil
	}
	for _, p := range []struct {
		key string
		v   *string
	}{{"blueprints.root", b.Root}, {"blueprints.outline", b.Outline}} {
		if p.v != nil && !filepath.IsLocal(*p.v) {
			return l.fail(p.key, "must be a path inside the project, relative to its root, not %q", *p.v)
		}
	}
	return nil
}

func (l *layer) checkGate() error {
	g := l.f.Gate
	if g == nil {
		return nil
	}
	if err := l.oneOf("gate.protocol", g.Protocol, "plain", "cairn-run-gate"); err != nil {
		return err
	}
	if g.SectionPattern != nil {
		re, err := regexp.Compile(*g.SectionPattern)
		if err != nil {
			return l.fail("gate.section_pattern", "is not a regular expression: %v", err)
		}
		if re.NumSubexp() < 1 {
			return l.fail("gate.section_pattern", "needs a capture group for the section name")
		}
	}
	if g.FailurePattern != nil {
		if _, err := regexp.Compile(*g.FailurePattern); err != nil {
			return l.fail("gate.failure_pattern", "is not a regular expression: %v", err)
		}
	}
	return firstErr(
		l.template("gate.command", g.Command),
		l.template("gate.index_link_command", g.IndexLinkCommand),
	)
}

func (l *layer) checkSeats() error {
	for _, name := range slices.Sorted(maps.Keys(l.f.Seat)) {
		if !slices.Contains(seatNames, name) {
			return l.fail("seat."+name, "is not a seat; the seats are %s", strings.Join(seatNames, ", "))
		}
		s := l.f.Seat[name]
		if s.MaxTokens != nil && *s.MaxTokens < 1 {
			return l.fail("seat."+name+".max_tokens", "must be at least 1, not %d", *s.MaxTokens)
		}
		if s.Inputs != nil {
			for _, in := range *s.Inputs {
				if err := l.template("seat."+name+".inputs", &in); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// checkProjectPaths refuses a drafter.toml path that resolves outside the project at root.
func (l *layer) checkProjectPaths(root string) error {
	inside := func(key string, p *string) error {
		if p == nil {
			return nil
		}
		rel, err := filepath.Rel(root, l.resolve(*p))
		if err != nil || !filepath.IsLocal(rel) {
			return l.fail(key, "must resolve inside the project, but %q does not; "+
				"a machine-specific path belongs in drafter.local.toml or the user config", *p)
		}
		return nil
	}
	f := l.f
	if err := inside("prompts", f.Prompts); err != nil {
		return err
	}
	if f.Facts != nil {
		if err := inside("facts.dir", f.Facts.Dir); err != nil {
			return err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(f.Seat)) {
		if err := inside("seat."+name+".agent", f.Seat[name].Agent); err != nil {
			return err
		}
	}
	return nil
}

// template refuses a command template that uses a placeholder other than {page}, {brief},
// {slug}, and {baseline}. A placeholder is a brace pair around an identifier with no $ before
// it, so a shell ${arm} and an awk '{print $1}' pass.
func (l *layer) template(key string, s *string) error {
	if s == nil {
		return nil
	}
	t := *s
	for i := 0; i < len(t); i++ {
		if t[i] != '{' || (i > 0 && t[i-1] == '$') {
			continue
		}
		end := strings.IndexByte(t[i:], '}')
		if end < 0 {
			break
		}
		name := t[i+1 : i+end]
		if isIdentifier(name) && !slices.Contains(placeholders, name) {
			return l.fail(key, "uses the placeholder {%s}; the placeholders are {%s}",
				name, strings.Join(placeholders, "}, {"))
		}
	}
	return nil
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		letter := r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		if !letter && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func (l *layer) oneOf(key string, v *string, allowed ...string) error {
	if v == nil || slices.Contains(allowed, *v) {
		return nil
	}
	return l.fail(key, "must be %s, not %q", strings.Join(allowed, " or "), *v)
}

func (l *layer) atLeast(key string, v *int, min int) error {
	if v == nil || *v >= min {
		return nil
	}
	return l.fail(key, "must be at least %d, not %d", min, *v)
}

func (l *layer) atMost(key string, v *int, max int) error {
	if v == nil || *v <= max {
		return nil
	}
	return l.fail(key, "must be at most %d, not %d", max, *v)
}

// positive refuses a cap that is not above 0, including NaN, and one that is infinite.
func (l *layer) positive(key string, v *float64) error {
	if v == nil || (*v > 0 && !math.IsInf(*v, 0)) {
		return nil
	}
	return l.fail(key, "must be above 0, not %v", *v)
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

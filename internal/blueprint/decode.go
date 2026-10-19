package blueprint

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/glw907/drafter/internal/plan"
)

// kinds lists, per item kind, the fields it requires and the fields it may carry besides id, kind,
// and origin. An item carrying any other field is refused. The kinds are plan format v1's plus
// example.
var kinds = map[string]struct{ required, optional []string }{
	"fact":     {required: []string{"fact"}, optional: []string{"use"}},
	"sentence": {required: []string{"mode", "text"}, optional: []string{"facts"}},
	"example":  {required: []string{"text"}, optional: []string{"facts"}},
	"link":     {required: []string{"target"}, optional: []string{"text"}},
	"handoff":  {required: []string{"text"}},
	"reason":   {required: []string{"text"}, optional: []string{"facts"}},
	"step":     {required: []string{"text"}, optional: []string{"requires", "produces", "canFail", "optional"}},
	"failure":  {required: []string{"symptom", "handledBy"}, optional: []string{"from", "cause", "facts"}},
	"code":     {required: []string{"text"}, optional: []string{"source", "facts"}},
	"figure":   {required: []string{"text"}, optional: []string{"source"}},
}

// Decode parses data as a blueprint and reports every schema fault it finds, each at its line, by
// the walk plan.Decode uses. The returned blueprint is nil when the file is not one YAML mapping;
// otherwise it is as far decoded as the faults allow, and a caller runs the checks only when
// plan.HasError reports false.
func Decode(file string, data []byte) (*Blueprint, []plan.Finding) {
	b := new(Blueprint)
	ok, findings := plan.DecodeDocument(file, data, b, func(r *plan.Reporter) { validateBlueprint(r, b) })
	if !ok {
		return nil, findings
	}
	return b, findings
}

func validateBlueprint(r *plan.Reporter, b *Blueprint) {
	if line, ok := b.Lines["version"]; ok && b.Version != Version && !r.Faulted(line) {
		r.Add(line, fmt.Sprintf("unsupported version %d", b.Version), "set version: 1")
	}
	if line, ok := b.Lines["review"]; ok && b.Review != "human" && !r.Faulted(line) {
		r.Add(line, fmt.Sprintf("unknown review %q", b.Review), "use review: human, or remove the field")
	}
	ids := map[string]int{}
	claim := func(line int, id, where string) {
		if first, dup := ids[id]; dup {
			r.Add(line, fmt.Sprintf("%s: id %q repeats the id at line %d; section and item ids share one namespace", where, id, first),
				"give each section and item its own id")
			return
		}
		ids[id] = line
	}
	for i := range b.Sections {
		s := &b.Sections[i]
		where := fmt.Sprintf("sections[%d]", i)
		if line, ok := s.Lines["id"]; ok {
			if !plan.ValidID(s.ID) {
				r.Add(line, fmt.Sprintf("%s: id %q is not a lowercase token", where, s.ID), "use lowercase letters, digits, - and _")
			} else {
				claim(line, s.ID, where)
			}
		}
		if line, ok := s.Lines["level"]; !ok {
			s.Level = 2
		} else if s.Level != 2 && s.Level != 3 && !r.Faulted(line) {
			r.Add(line, fmt.Sprintf("%s: level %d is not a heading level the blueprint supports", where, s.Level), "use level: 2 or level: 3, or omit it for 2")
		}
	}
	for i := range b.Sections {
		s := &b.Sections[i]
		parentOfSubsections := s.Level == 2 && i+1 < len(b.Sections) && b.Sections[i+1].Level == 3
		if line, ok := s.Lines["items"]; ok && len(s.Items) == 0 && !parentOfSubsections && !r.Faulted(line) {
			r.Add(line, fmt.Sprintf("sections[%d].items: must hold at least one entry unless the next section is level 3", i), "add an entry, or leave items empty only above a level-3 section")
		}
		for j := range s.Items {
			it := &s.Items[j]
			where := fmt.Sprintf("sections[%d].items[%d]", i, j)
			if line, ok := it.Lines["id"]; ok && plan.ValidID(it.ID) {
				claim(line, it.ID, where)
			}
			validateItem(r, it, where)
		}
	}
	for i := range b.Dispositions {
		x := &b.Dispositions[i]
		where := fmt.Sprintf("dispositions[%d]", i)
		switch x.Disposition {
		case "", "cut":
		case "subordinated":
			if _, ok := x.Lines["reference"]; !ok {
				r.Add(x.Line, where+": a subordinated fact needs a reference", "add reference: with the link target that now carries the fact")
			}
		default:
			r.Add(x.Lines["disposition"], where+fmt.Sprintf(": unknown disposition %q", x.Disposition), "use subordinated or cut")
		}
	}
}

func validateItem(r *plan.Reporter, it *Item, where string) {
	if line, ok := it.Lines["id"]; ok && !plan.ValidID(it.ID) {
		r.Add(line, fmt.Sprintf("%s: id %q is not a lowercase token", where, it.ID), "use lowercase letters, digits, - and _")
	}
	if line, ok := it.Lines["origin"]; ok && it.Origin != "human" && !r.Faulted(line) {
		r.Add(line, fmt.Sprintf("%s: unknown origin %q", where, it.Origin), "use origin: human, or remove the field")
	}
	kind, ok := it.Lines["kind"]
	if !ok {
		return
	}
	spec, known := kinds[it.Kind]
	if !known {
		names := slices.Sorted(maps.Keys(kinds))
		r.Add(kind, fmt.Sprintf("%s: unknown kind %q", where, it.Kind), "use one of "+strings.Join(names, ", "))
		return
	}
	allowed := slices.Concat([]string{"id", "kind", "origin"}, spec.required, spec.optional)
	for name, line := range it.Lines {
		if !slices.Contains(allowed, name) {
			r.Add(line, fmt.Sprintf("%s: field %q does not belong to kind %q", where, name, it.Kind), "remove it; a "+it.Kind+" item takes "+strings.Join(allowed, ", "))
		}
	}
	for _, name := range spec.required {
		if _, ok := it.Lines[name]; !ok {
			r.Add(it.Line, fmt.Sprintf("%s: kind %q needs the field %q", where, it.Kind, name), fmt.Sprintf("add %s:", name))
		}
	}
	if line, ok := it.Lines["mode"]; ok && it.Mode != "verbatim" && it.Mode != "claim" {
		r.Add(line, fmt.Sprintf("%s: unknown mode %q", where, it.Mode), "use verbatim or claim")
	}
}

// DecodeOutline parses data as an outline.yaml, as Decode parses a blueprint.
func DecodeOutline(file string, data []byte) (*Outline, []plan.Finding) {
	o := new(Outline)
	ok, findings := plan.DecodeDocument(file, data, o, func(r *plan.Reporter) { validateOutline(r, o) })
	if !ok {
		return nil, findings
	}
	return o, findings
}

func validateOutline(r *plan.Reporter, o *Outline) {
	if line, ok := o.Lines["version"]; ok && o.Version != Version && !r.Faulted(line) {
		r.Add(line, fmt.Sprintf("unsupported version %d", o.Version), "set version: 1")
	}
	names := map[string]bool{}
	for i := range o.Types {
		t := &o.Types[i]
		if names[t.Name] {
			r.Add(t.Lines["name"], fmt.Sprintf("types[%d]: type %q is declared twice", i, t.Name), "declare each type once")
		}
		names[t.Name] = true
	}
	trackNames := map[string]bool{}
	for i := range o.Tracks {
		t := &o.Tracks[i]
		if line, ok := t.Lines["name"]; ok && trackNames[t.Name] && !r.Faulted(line) {
			r.Add(line, fmt.Sprintf("tracks[%d]: track %q is declared twice", i, t.Name), "declare each track once")
		}
		trackNames[t.Name] = true
	}
	paths, slugs := map[string]bool{}, map[string]bool{}
	for i := range o.Pages {
		p := &o.Pages[i]
		where := fmt.Sprintf("pages[%d]", i)
		if _, set := p.Lines["managed"]; !set {
			p.Managed = true
		}
		if line, ok := p.Lines["path"]; ok && !r.Faulted(line) {
			switch {
			case !filepath.IsLocal(p.Path):
				r.Add(line, fmt.Sprintf("%s: path %q is not a path inside the repository", where, p.Path),
					"write the path relative to the repository root, with no leading / and no .. that climbs out")
			case paths[p.Path]:
				r.Add(line, fmt.Sprintf("%s: path %q is listed twice", where, p.Path), "list each page once")
			}
			paths[p.Path] = true
		}
		if p.Slug != "" {
			if slugs[p.Slug] {
				r.Add(p.Lines["slug"], fmt.Sprintf("%s: slug %q is used twice", where, p.Slug), "give each page its own slug")
			}
			slugs[p.Slug] = true
		}
		if !p.Managed {
			continue
		}
		for _, f := range []struct{ name, value string }{{"slug", p.Slug}, {"title", p.Title}, {"type", p.Type}, {"job", p.Job}} {
			if f.value == "" {
				r.Add(p.Line, fmt.Sprintf("%s: a managed page needs %q", where, f.name), fmt.Sprintf("add %s:, or mark the page managed: false", f.name))
			}
		}
	}
}

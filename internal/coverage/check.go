package coverage

import (
	"path"
	"regexp"
	"strings"

	"github.com/glw907/drafter/internal/plan"
)

// Miss is a plan item the page doesn't carry. Item is the plan item's id, or the sentence's text
// for a brief sentence the page lacks; Kind is the item's kind, its mode for a sentence, or
// "brief" for a brief sentence; Section is the plan section's heading, empty for a brief sentence.
type Miss struct {
	Item    string
	Section string
	Kind    string
	Reason  string
}

const reasonNoHeading = "heading not found"

var (
	inlineLink = regexp.MustCompile(`\]\(\s*<?([^)\s>]+)`)
	autoLink   = regexp.MustCompile(`<(https?://[^>\s]+)>`)
	refLink    = regexp.MustCompile(`(?m)^ {0,3}\[[^\]]+\]:[ \t]*<?(\S+?)>?[ \t]*$`)
)

// Check runs the mechanical checks of p against page and its sentence-to-fact brief: each brief
// sentence must be on the page, each verbatim sentence must occur in its section, each link item's
// target must be linked inside its section, and each fact item must be cited by a brief sentence
// located in its section. Fact items are skipped when facts is false. A section whose heading is
// not on the page makes each checked item in it a miss. The error is a brief that isn't valid JSON.
func Check(p *plan.Plan, page, brief []byte, facts bool) ([]Miss, error) {
	b, err := parseBrief(brief)
	if err != nil {
		return nil, err
	}
	preamble, regions := split(p, page)

	var misses []Miss
	cited := make([]map[string]bool, len(regions))
	for i := range cited {
		cited[i] = map[string]bool{}
	}
	for _, s := range b.Sentences {
		want := Normalize(s.Text)
		located := want != "" && strings.Contains(preamble.norm, want)
		for i, r := range regions {
			if want == "" || !r.found || !strings.Contains(r.norm, want) {
				continue
			}
			located = true
			for _, id := range s.ID {
				cited[i][id] = true
			}
		}
		if !located {
			misses = append(misses, Miss{Item: s.Text, Kind: "brief", Reason: "brief sentence not found on the page"})
		}
	}

	dir := path.Dir(p.Page.Path)
	for i, s := range p.Sections {
		r := regions[i]
		for _, it := range s.Items {
			kind, ok := checked(it, facts)
			if !ok {
				continue
			}
			miss := Miss{Item: it.ID, Section: s.Heading, Kind: kind}
			switch {
			case !r.found:
				miss.Reason = reasonNoHeading
			case kind == "fact" && !cited[i][it.Fact]:
				miss.Reason = "no brief sentence in this section cites fact " + it.Fact
			case kind == "verbatim" && !strings.Contains(r.norm, Normalize(it.Text)):
				miss.Reason = "verbatim sentence not found in this section"
			case kind == "link" && !linked(r.raw, dir, it.Target):
				miss.Reason = "no link to " + it.Target + " in this section"
			default:
				continue
			}
			misses = append(misses, miss)
		}
	}
	return misses, nil
}

// checked reports whether the mechanical checks cover it, and its Miss kind.
func checked(it plan.Item, facts bool) (string, bool) {
	switch {
	case it.Kind == "fact":
		return "fact", facts
	case it.Kind == "link":
		return "link", true
	case it.Kind == "sentence" && it.Mode == "verbatim":
		return "verbatim", true
	}
	return "", false
}

// linked reports whether the Markdown in raw links target. Paths resolve from the page's
// directory dir, anchor included; a same-page anchor and a URL match as written.
func linked(raw, dir, target string) bool {
	want := resolve(dir, target)
	for _, re := range []*regexp.Regexp{inlineLink, autoLink, refLink} {
		for _, m := range re.FindAllStringSubmatch(raw, -1) {
			if resolve(dir, m[1]) == want {
				return true
			}
		}
	}
	return false
}

func resolve(dir, target string) string {
	if strings.HasPrefix(target, "#") || strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return target
	}
	file, anchor, hasAnchor := strings.Cut(target, "#")
	out := path.Join(dir, file)
	if hasAnchor {
		out += "#" + anchor
	}
	return out
}

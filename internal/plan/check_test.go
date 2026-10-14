package plan

import (
	"strings"
	"testing"
)

const (
	testRoot  = "testdata/repo"
	testFacts = "testdata/repo/docs/facts"
)

type want struct {
	line            int
	severity, check string
	contains        string
}

// runCheck decodes src, which must be free of decode errors, and checks it against the fixture
// repo with its facts directory and outline unless opts changes them.
func runCheck(t *testing.T, src string, opts func(*CheckOptions)) []Finding {
	t.Helper()
	p, fs := Decode("p.plan.yaml", []byte(src))
	if len(fs) != 0 {
		t.Fatalf("the mutated plan no longer decodes: %+v", fs)
	}
	o, err := LoadOutline(testRoot + "/docs/outline.json")
	if err != nil {
		t.Fatal(err)
	}
	co := CheckOptions{Root: testRoot, FactsDir: testFacts, Outline: o}
	if opts != nil {
		opts(&co)
	}
	return Check("p.plan.yaml", p, co)
}

func assertFindings(t *testing.T, got []Finding, wants []want) {
	t.Helper()
	if len(got) != len(wants) {
		t.Fatalf("findings = %+v, want %d", got, len(wants))
	}
	for i, w := range wants {
		f := got[i]
		if f.Line != w.line || f.Severity != w.severity || f.Check != w.check || !strings.Contains(f.Message, w.contains) || f.File != "p.plan.yaml" || f.Fix == "" {
			t.Errorf("finding %d = %+v, want line %d, %s, check %s, message containing %q, and a fix", i, f, w.line, w.severity, w.check, w.contains)
		}
	}
}

func TestCheckValid(t *testing.T) {
	assertFindings(t, runCheck(t, readValid(t), nil), nil)
}

func TestCheckIDs(t *testing.T) {
	tests := []struct {
		name, old, repl string
		wants           []want
	}{
		{"duplicate id", "id: l-keys", "id: f-key-ttl", []want{{28, SeverityError, "ids", "repeats the id of the item at line 25"}}},
		{"dangling handledBy", "handledBy: r-quota", "handledBy: r-gone", []want{{47, SeverityError, "ids", `handledBy names "r-gone"`}}},
		{"from names a non-step", "from: s-create", "from: r-quota", []want{
			{41, SeverityError, "recovery", `step "s-create" can fail`},
			{44, SeverityError, "ids", `"r-quota", a reason item, not a step`},
		}},
		{"from names no item", "from: s-create", "from: s-nope", []want{
			{41, SeverityError, "recovery", `step "s-create" can fail`},
			{44, SeverityError, "ids", `from names "s-nope", which is no item's id`},
		}},
		{"fact both placed and disposed", "fact: legacy-format", "fact: signing-key-ttl", []want{
			{80, SeverityError, "ids", `placed in the page by the item at line 25 and also disposed`},
		}},
		{"handledBy names a step", "handledBy: r-quota", "handledBy: s-deploy", nil},
		{"a failure with no from is free-standing", "        canFail: true\n      - id: x-create\n        kind: failure\n        from: s-create\n", "      - id: x-create\n        kind: failure\n", nil},
		{"a free-standing failure still needs its handler", "        canFail: true\n      - id: x-create\n        kind: failure\n        from: s-create\n        symptom: |\n          The command prints a quota error.\n        handledBy: r-quota\n", "      - id: x-create\n        kind: failure\n        symptom: |\n          The command prints a quota error.\n        handledBy: r-gone\n", []want{
			{45, SeverityError, "ids", `handledBy names "r-gone"`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFindings(t, runCheck(t, mutate(t, tt.old, tt.repl), nil), tt.wants)
		})
	}
}

func TestCheckReaderState(t *testing.T) {
	tests := []struct {
		name, old, repl string
		wants           []want
	}{
		{"met only by a later step", "requires: [signing-key-exists]", "requires: [deployed]", []want{
			{39, SeverityError, "reader-state", `requires "deployed", which no earlier step produces and no precondition lists (the step at line 56 produces it`},
		}},
		{"met by nothing", "  - signing-key-exists\n", "  - other-state\n", []want{
			{39, SeverityError, "reader-state", `requires "signing-key-exists", which no earlier step produces and no precondition lists`},
		}},
		{"a step cannot meet its own requirement", "requires: [new-key]", "requires: [deployed]", []want{
			{59, SeverityError, "reader-state", "(the step at line 56 produces it, but not before this one)"},
		}},
		{"met by an earlier step", "requires: [new-key]", "requires: [new-key, signing-key-exists]", nil},
		{"two unmet requirements sort by message", "requires: [signing-key-exists]", "requires: [zeta, alpha]", []want{
			{39, SeverityError, "reader-state", `requires "alpha"`},
			{39, SeverityError, "reader-state", `requires "zeta"`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFindings(t, runCheck(t, mutate(t, tt.old, tt.repl), nil), tt.wants)
		})
	}
}

func TestCheckRecovery(t *testing.T) {
	tests := []struct {
		name, old, repl string
		wants           []want
	}{
		{"failing step with no failure item", "        produces: [deployed]\n", "        produces: [deployed]\n        canFail: true\n", []want{
			{61, SeverityError, "recovery", `step "s-deploy" can fail, but no failure item names it`},
		}},
		{"canFail false needs no failure item", "canFail: true", "canFail: false", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFindings(t, runCheck(t, mutate(t, tt.old, tt.repl), nil), tt.wants)
		})
	}
}

func TestCheckLinks(t *testing.T) {
	const target = "../reference/keys.md#key-lifetime"
	noOutline := func(o *CheckOptions) { o.Outline = nil }
	tests := []struct {
		name, old, repl string
		opts            func(*CheckOptions)
		wants           []want
	}{
		{"missing target", target, "../reference/gone.md#key-lifetime", nil, []want{
			{30, SeverityError, "links", "target docs/reference/gone.md does not exist"},
		}},
		{"missing anchor", target, "../reference/keys.md#nope", nil, []want{
			{30, SeverityError, "links", "anchor #nope matches no heading in docs/reference/keys.md"},
		}},
		{"anchor on a directory", target, "../reference#key-lifetime", nil, []want{
			{30, SeverityError, "links", "docs/reference is a directory"},
		}},
		{"a directory passes", target, "../reference/", nil, nil},
		{"a file without an anchor passes", target, "../reference/keys.md", nil, nil},
		{"anchor in fenced code", target, "../reference/keys.md#not-a-heading", nil, []want{
			{30, SeverityError, "links", "anchor #not-a-heading matches no heading"},
		}},
		{"repeated heading anchor", target, "../reference/keys.md#key-lifetime-1", nil, nil},
		{"setext heading anchor", target, "../reference/keys.md#revocation-steps", nil, nil},
		{"heading with code and a link", target, "../reference/keys.md#the-key-command-and-a-link--more", nil, nil},
		{"front matter is no anchor", target, "../reference/keys.md#keys-reference", nil, []want{
			{30, SeverityError, "links", "anchor #keys-reference matches no heading"},
		}},
		{"same-page anchor on a section heading", target, `"#create-the-new-key"`, nil, nil},
		{"same-page anchor on the title", target, `"#rotate-the-signing-key"`, nil, nil},
		{"a bare # names nothing", target, `"#"`, nil, []want{
			{30, SeverityError, "links", `target "#" names neither an anchor nor a path`},
		}},
		{"same-page anchor with no heading", target, `"#nope"`, nil, []want{
			{30, SeverityError, "links", "anchor #nope matches no heading on this page"},
		}},
		{"outline-listed target not on disk", target, "../reference/planned.md#anything", nil, []want{
			{30, SeverityWarning, "links", "target docs/reference/planned.md is in the outline but not on disk yet"},
		}},
		{"the same target without an outline", target, "../reference/planned.md#anything", noOutline, []want{
			{30, SeverityError, "links", "target docs/reference/planned.md does not exist"},
		}},
		{"an unlisted target next to a listed one", target, "../reference/other.md", nil, []want{
			{30, SeverityError, "links", "target docs/reference/other.md does not exist"},
		}},
		{"code source that exists", handoff, codeItem("../reference/keys.md") + handoff, nil, nil},
		{"code source with a symbol", handoff, codeItem("../reference/keys.md#NoSuchSymbol") + handoff, nil, nil},
		{"code source that is missing", handoff, codeItem("../reference/gone.go#Symbol") + handoff, nil, []want{
			{72, SeverityError, "links", "source docs/reference/gone.go does not exist"},
		}},
		{"code source outside the repository", handoff, codeItem("../../../x.go") + handoff, nil, []want{
			{72, SeverityError, "links", "target ../../../x.go leaves the repository"},
		}},
		{"https target is not checked", target, "https://example.com/a#nowhere", nil, nil},
		{"http target is not checked", target, "http://example.com/a", nil, nil},
		{"root-relative target", target, "/docs/reference/keys.md#grace-period", nil, nil},
		{"target outside the repository", target, "../../../x.md", nil, []want{
			{30, SeverityError, "links", "target ../../../x.md leaves the repository"},
		}},
		{"subordinated reference with a missing anchor", "../reference/keys.md#grace-period", "../reference/keys.md#nope", nil, []want{
			{77, SeverityError, "links", "anchor #nope matches no heading in docs/reference/keys.md"},
		}},
		{"subordinated reference with a missing target", "../reference/keys.md#grace-period", "../reference/gone.md", nil, []want{
			{77, SeverityError, "links", "target docs/reference/gone.md does not exist"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFindings(t, runCheck(t, mutate(t, tt.old, tt.repl), tt.opts), tt.wants)
		})
	}
}

func TestCheckSentenceFacts(t *testing.T) {
	const withFacts = sentenceText + "        facts: [%s]\n"
	row := func(ids string) string { return strings.Replace(withFacts, "%s", ids, 1) }
	noFacts := func(o *CheckOptions) { o.FactsDir = "" }
	tests := []struct {
		name, repl string
		opts       func(*CheckOptions)
		wants      []want
	}{
		{"citable facts", row("signing-key-ttl, vendor-fact"), nil, nil},
		{"a rejected fact", row("signing-key-ttl, rejected-fact"), nil, []want{
			{65, SeverityError, "facts", `fact "rejected-fact" is tagged [rejected], which a page cannot cite`},
		}},
		{"an id in no facts file", row("no-such-fact"), nil, []want{
			{65, SeverityError, "facts", `fact "no-such-fact" is in no facts file`},
		}},
		{"an id on two bullets", row("dup-id"), nil, []want{
			{65, SeverityError, "facts", `fact "dup-id" is on 2 bullets`},
		}},
		{"no facts directory", row("signing-key-ttl"), noFacts, []want{
			{27, SeverityWarning, "facts", "the fact item at line 27 is not checked"},
			{65, SeverityWarning, "facts", "the facts list at line 65 is not checked because no facts directory is set"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFindings(t, runCheck(t, mutate(t, sentenceText, tt.repl), tt.opts), tt.wants)
		})
	}
}

func TestCheckFacts(t *testing.T) {
	const placed = "fact: signing-key-ttl"
	tests := []struct {
		name, old, repl string
		opts            func(*CheckOptions)
		wants           []want
	}{
		{"a bullet whose tag wraps", placed, "fact: wrapped-tag", nil, nil},
		{"a bracket in a code span", placed, "fact: code-bracket", nil, nil},
		{"a bracket in a quoted span", placed, "fact: quoted-bracket", nil, nil},
		{"a vendor fact", placed, "fact: vendor-fact", nil, nil},
		{"an id in no facts file", placed, "fact: no-such-fact", nil, []want{
			{27, SeverityError, "facts", `fact "no-such-fact" is in no facts file`},
		}},
		{"an id only in the README", placed, "fact: readme-only", nil, []want{
			{27, SeverityError, "facts", `fact "readme-only" is in no facts file`},
		}},
		{"a bullet under Provenance", placed, "fact: provenance-only", nil, []want{
			{27, SeverityError, "facts", `fact "provenance-only" is in no facts file`},
		}},
		{"an id on two bullets", placed, "fact: dup-id", nil, []want{
			{27, SeverityError, "facts", `fact "dup-id" is on 2 bullets (edge.md:21, edge.md:22)`},
		}},
		{"a rejected fact", placed, "fact: rejected-fact", nil, []want{
			{27, SeverityError, "facts", `tagged [rejected], which a page cannot cite`},
		}},
		{"a candidate fact marked excluded", placed, "fact: cand-excluded", nil, []want{
			{27, SeverityError, "facts", `tagged [candidate], which a page cannot cite`},
		}},
		{"a candidate fact with no qualifier", placed, "fact: cand-plain", nil, []want{
			{27, SeverityError, "facts", `tagged [candidate], which a page cannot cite`},
		}},
		{"a docs-drift fact", placed, "fact: drift-fact", nil, []want{
			{27, SeverityError, "facts", `tagged [docs-drift], which a page cannot cite`},
		}},
		{"a bullet with no tag", placed, "fact: no-tag", nil, []want{
			{27, SeverityError, "facts", "cannot be cited: it has no status tag"},
		}},
		{"a tag outside the vocabulary", placed, "fact: bad-tag", nil, []want{
			{27, SeverityError, "facts", "cannot be cited: tag outside the vocabulary"},
		}},
		{"a tag with a space qualifier", placed, "fact: space-tag", nil, []want{
			{27, SeverityError, "facts", "cannot be cited: tag [verified...] uses a space qualifier"},
		}},
		{"a bullet with two tags", placed, "fact: two-tags", nil, []want{
			{27, SeverityError, "facts", "cannot be cited: it has 2 status tags"},
		}},
		{"a disposition naming an unknown fact", "fact: old-key-grace", "fact: no-such-fact", nil, []want{
			{75, SeverityError, "facts", `fact "no-such-fact" is in no facts file`},
		}},
		{"a disposition may name an uncitable fact", "fact: old-key-grace", "fact: rejected-fact", nil, nil},
		{"a disposition naming a duplicated id", "fact: old-key-grace", "fact: dup-id", nil, []want{
			{75, SeverityError, "facts", `fact "dup-id" is on 2 bullets`},
		}},
		{"no facts directory", placed, placed, func(o *CheckOptions) { o.FactsDir = "" }, []want{
			{27, SeverityWarning, "facts", "the fact item at line 27 is not checked because no facts directory is set"},
		}},
		{"an unreadable facts directory", placed, placed, func(o *CheckOptions) { o.FactsDir = testRoot + "/docs/absent" }, []want{
			{1, SeverityError, "facts", "the facts directory cannot be read"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFindings(t, runCheck(t, mutate(t, tt.old, tt.repl), tt.opts), tt.wants)
		})
	}
}

// Facts lists on the introduction, a code item, and a reason item resolve like a sentence's, and
// every facts list is held against the dispositions.
func TestCheckFactsLists(t *testing.T) {
	const (
		intro  = "    Rotating the database credentials.\n"
		reason = "          A quota error means the account already holds the maximum number of keys.\n"
	)
	noFacts := func(o *CheckOptions) { o.FactsDir = "" }
	introFacts := func(ids string) string { return intro + "  facts: [" + ids + "]\n" }
	reasonFacts := func(ids string) string { return reason + "        facts: [" + ids + "]\n" }
	failure := "        handledBy: r-quota\n"
	failureFacts := func(ids string) string { return failure + "        facts: [" + ids + "]\n" }
	codeFacts := func(ids string) string {
		return "      - id: c-demo\n        kind: code\n        text: |\n          The key command.\n        facts: [" + ids + "]\n" + handoff
	}
	tests := []struct {
		name, old, repl string
		opts            func(*CheckOptions)
		wants           []want
	}{
		{"introduction facts that are citable", intro, introFacts("signing-key-ttl"), nil, nil},
		{"introduction fact that is rejected", intro, introFacts("rejected-fact"), nil, []want{
			{16, SeverityError, "facts", `fact "rejected-fact" is tagged [rejected]`},
		}},
		{"introduction facts with no facts directory", intro, introFacts("signing-key-ttl"), noFacts, []want{
			{16, SeverityWarning, "facts", "the facts list at line 16 is not checked"},
			{28, SeverityWarning, "facts", "the fact item at line 28 is not checked"},
		}},
		{"code facts that are citable", handoff, codeFacts("signing-key-ttl"), nil, nil},
		{"code fact that is in no facts file", handoff, codeFacts("no-such-fact"), nil, []want{
			{74, SeverityError, "facts", `fact "no-such-fact" is in no facts file`},
		}},
		{"code facts with no facts directory", handoff, codeFacts("signing-key-ttl"), noFacts, []want{
			{27, SeverityWarning, "facts", "the fact item at line 27 is not checked"},
			{74, SeverityWarning, "facts", "the facts list at line 74 is not checked"},
		}},
		{"failure facts that are citable", failure, failureFacts("signing-key-ttl"), nil, nil},
		{"failure fact that is rejected", failure, failureFacts("rejected-fact"), nil, []want{
			{48, SeverityError, "facts", `fact "rejected-fact" is tagged [rejected]`},
		}},
		{"failure facts with no facts directory", failure, failureFacts("signing-key-ttl"), noFacts, []want{
			{27, SeverityWarning, "facts", "the fact item at line 27 is not checked"},
			{48, SeverityWarning, "facts", "the facts list at line 48 is not checked"},
		}},
		{"a failure cites a disposed fact", failure, failureFacts("old-key-grace"), nil, []want{
			{48, SeverityError, "ids", `fact "old-key-grace" is cited here but disposed at line 76`},
		}},
		{"reason facts that are citable", reason, reasonFacts("signing-key-ttl"), nil, nil},
		{"reason fact that is rejected", reason, reasonFacts("rejected-fact"), nil, []want{
			{52, SeverityError, "facts", `fact "rejected-fact" is tagged [rejected]`},
		}},
		{"reason facts with no facts directory", reason, reasonFacts("signing-key-ttl"), noFacts, []want{
			{27, SeverityWarning, "facts", "the fact item at line 27 is not checked"},
			{52, SeverityWarning, "facts", "the facts list at line 52 is not checked"},
		}},
		{"an empty facts list is not warned about", reason, reasonFacts(""), noFacts, []want{
			{27, SeverityWarning, "facts", "the fact item at line 27 is not checked"},
		}},
		{"a sentence cites a disposed fact", sentenceText, sentenceText + "        facts: [old-key-grace]\n", nil, []want{
			{65, SeverityError, "ids", `fact "old-key-grace" is cited here but disposed at line 76`},
		}},
		{"the same check with no facts directory", sentenceText, sentenceText + "        facts: [old-key-grace]\n", noFacts, []want{
			{27, SeverityWarning, "facts", "the fact item at line 27 is not checked"},
			{65, SeverityError, "ids", `fact "old-key-grace" is cited here but disposed at line 76`},
			{65, SeverityWarning, "facts", "the facts list at line 65 is not checked"},
		}},
		{"the introduction cites a disposed fact", intro, introFacts("old-key-grace"), nil, []want{
			{16, SeverityError, "ids", `fact "old-key-grace" is cited here but disposed at line 76`},
		}},
		{"a code item cites a disposed fact", handoff, codeFacts("old-key-grace"), nil, []want{
			{74, SeverityError, "ids", `fact "old-key-grace" is cited here but disposed at line 80`},
		}},
		{"a reason cites a disposed fact", reason, reasonFacts("old-key-grace"), nil, []want{
			{52, SeverityError, "ids", `fact "old-key-grace" is cited here but disposed at line 76`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFindings(t, runCheck(t, mutate(t, tt.old, tt.repl), tt.opts), tt.wants)
		})
	}
}

func TestCheckSectionAnchors(t *testing.T) {
	const heading = "  - heading: Create the new key\n"
	const deploy = "  - heading: Deploy the new key\n"
	tests := []struct {
		name, old, repl string
		wants           []want
	}{
		{"an anchor equal to the slug", heading, heading + "    anchor: create-the-new-key\n", nil},
		{"an anchor that differs from the slug", heading, heading + "    anchor: create-key\n", []want{
			{32, SeverityError, "links", `anchor "create-key" does not match "create-the-new-key", the slug of the heading "Create the new key"`},
		}},
		{"a repeated heading takes the suffix", deploy, heading + "    anchor: create-the-new-key-1\n", nil},
		{"a repeated heading without the suffix", deploy, heading + "    anchor: create-the-new-key\n", []want{
			{53, SeverityError, "links", `does not match "create-the-new-key-1"`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFindings(t, runCheck(t, mutate(t, tt.old, tt.repl), nil), tt.wants)
		})
	}
}

// An optional step's products do not satisfy a later required step.
func TestCheckOptionalSteps(t *testing.T) {
	const (
		canFail  = "        canFail: true\n"
		deployed = "        produces: [deployed]\n"
		deploy   = "      - id: s-deploy\n"
	)
	optionalCreate := func() string { return mutate(t, canFail, canFail+"        optional: true\n") }
	tests := []struct {
		name, src string
		wants     []want
	}{
		{"a required step needs a token only an optional step produces", optionalCreate(), []want{
			{60, SeverityWarning, "reader-state", `step "s-deploy" requires "new-key", which only optional steps produce before it`},
		}},
		{"a required step also produces the token", strings.Replace(optionalCreate(), deploy,
			"      - id: s-alt\n        kind: step\n        text: Make the key another way.\n        produces: [new-key]\n"+deploy, 1), nil},
		{"an optional step may need an optional token", strings.Replace(optionalCreate(), deployed, deployed+"        optional: true\n", 1), nil},
		{"a precondition supplies the token", strings.Replace(optionalCreate(), "  - signing-key-exists\n", "  - signing-key-exists\n  - new-key\n", 1), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFindings(t, runCheck(t, tt.src, nil), tt.wants)
		})
	}
}

// A code source that is a URL, or absent, is not checked.
func TestCheckCodeSourceUnchecked(t *testing.T) {
	text := "        text: |\n          The key command.\n"
	tests := []struct{ name, item string }{
		{"a URL", "      - id: c-demo\n        kind: code\n        source: https://example.com/missing.go\n" + text},
		{"no source", "      - id: c-demo\n        kind: code\n" + text},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFindings(t, runCheck(t, mutate(t, handoff, tt.item+handoff), nil), nil)
		})
	}
}

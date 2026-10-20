package blueprint

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

var testLayout = Layout{BlueprintRoot: DefaultRoot, LockPath: DefaultRoot + "/" + LockFile}

const convertPage = "docs/guides/rotate-the-widget-key.md"

func convertFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "convert", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func fixturePlan(t *testing.T) *plan.Plan {
	t.Helper()
	p, findings := plan.Decode("widget.plan.yaml", convertFixture(t, "widget.plan.yaml"))
	if len(findings) > 0 {
		t.Fatalf("the fixture plan has findings: %+v", findings)
	}
	return p
}

func fixtureInput(t *testing.T) PlanInput {
	t.Helper()
	jo, err := ParseJSONOutlines([]NamedData{{Name: "widget.outline.json", Data: convertFixture(t, "widget.outline.json")}})
	if err != nil {
		t.Fatal(err)
	}
	ex, ok := jo.Exemplars(convertPage)
	if !ok {
		t.Fatal("the fixture outline does not list the page")
	}
	return PlanInput{Plan: fixturePlan(t), Framing: convertFixture(t, "widget.framing.md"), Exemplars: ex}
}

func convertedFixture(t *testing.T) (*Converted, *Blueprint) {
	t.Helper()
	conv, err := ConvertPlan(fixtureInput(t))
	if err != nil {
		t.Fatal(err)
	}
	b, findings := Decode("out.blueprint.yaml", conv.Data)
	if len(findings) > 0 {
		t.Fatalf("the converted blueprint has findings: %+v\n%s", findings, conv.Data)
	}
	return conv, b
}

func bare(it Item) plan.Item {
	return plan.Item{ID: it.ID, Kind: it.Kind, Fact: it.Fact, Use: it.Use, Mode: it.Mode, Text: it.Text, Facts: it.Facts,
		Source: it.Source, Target: it.Target, Requires: it.Requires, Produces: it.Produces, CanFail: it.CanFail,
		Optional: it.Optional, From: it.From, Symptom: it.Symptom, Cause: it.Cause, HandledBy: it.HandledBy}
}

func bareItem(it plan.Item) plan.Item {
	it.Line, it.Lines = 0, nil
	return it
}

func TestConvertPlanKeepsEveryField(t *testing.T) {
	p := fixturePlan(t)
	conv, b := convertedFixture(t)

	if b.Argument != p.Argument || b.OrderReason != p.OrderReason {
		t.Errorf("argument or orderReason changed: %q, %q", b.Argument, b.OrderReason)
	}
	if b.Introduction.Covers != p.Introduction.Covers || b.Introduction.PriorKnowledge != p.Introduction.PriorKnowledge ||
		b.Introduction.NotCovered != p.Introduction.NotCovered || !reflect.DeepEqual(b.Introduction.Facts, p.Introduction.Facts) {
		t.Errorf("introduction changed: %+v", b.Introduction)
	}
	if !reflect.DeepEqual(b.Preconditions, p.Preconditions) || !reflect.DeepEqual(b.Constraints, p.Constraints) {
		t.Errorf("preconditions %v or constraints %v changed", b.Preconditions, b.Constraints)
	}
	if len(b.Dispositions) != 1 || b.Dispositions[0].Fact != "key-store" || b.Dispositions[0].Reference != "../reference/keys.md" ||
		b.Dispositions[0].Reason != p.Dispositions[0].Reason {
		t.Errorf("dispositions changed: %+v", b.Dispositions)
	}

	if len(b.Sections) != len(p.Sections) {
		t.Fatalf("%d sections, want %d", len(b.Sections), len(p.Sections))
	}
	wantIDs := []string{"before-you-begin", "create-the-new-key", "choose-a-size", "verify-the-key", "delete-the-old-key"}
	for i, ps := range p.Sections {
		s := b.Sections[i]
		if s.ID != wantIDs[i] || s.Heading != ps.Heading || s.Level != ps.Level || s.Anchor != ps.Anchor || s.Opens != ps.Opens {
			t.Errorf("section %d = {%s %q %d %q}, want id %s, heading %q, level %d, anchor %q", i, s.ID, s.Heading, s.Level, s.Anchor, wantIDs[i], ps.Heading, ps.Level, ps.Anchor)
		}
		if s.Why != "" {
			t.Errorf("section %d got a why: %q", i, s.Why)
		}
		if len(s.Items) != len(ps.Items) {
			t.Errorf("section %d has %d items, want %d", i, len(s.Items), len(ps.Items))
			continue
		}
		for j, it := range ps.Items {
			if got, want := bare(s.Items[j]), bareItem(it); !reflect.DeepEqual(got, want) {
				t.Errorf("section %d item %d = %+v, want %+v", i, j, got, want)
			}
		}
	}
	if len(b.Sections[1].Items) != 0 || b.Sections[1].Lines["items"] == 0 {
		t.Errorf("the level-2 section above a level-3 section lost its explicit empty items")
	}

	want := Framing{
		Readers:       "Operators arrive from the alert that the key expires soon. They rotate the key `key-ttl`\nbefore it lapses.\n",
		Background:    "The widget reads its key at start.\n\nThe old key stays valid for one day.\n",
		PlaceInDocSet: "Sits after the install guide and before the reference.\n",
		IntroPlan:     "Open with the one command, then the check.\n",
	}
	got := b.Framing
	got.Line, got.Lines = 0, nil
	if !reflect.DeepEqual(got, want) {
		t.Errorf("framing = %+v, want %+v", got, want)
	}

	if len(b.Exemplars) != 2 || b.Exemplars[0].Source != "docs/guides/install-the-widget.md" || b.Exemplars[0].Slice != "guides" ||
		b.Exemplars[0].ManifestType != "task-guide" || b.Exemplars[0].Take != "The step shape; leave the intro." ||
		b.Exemplars[1].Source != "docs/guides/other.md" || b.Exemplars[1].Slice != "" {
		t.Errorf("exemplars = %+v", b.Exemplars)
	}

	text := string(conv.Data)
	for _, banned := range []string{"friction", "page:", "why rotate", "Departures", "registerSections", "The register has no entry"} {
		if strings.Contains(text, banned) {
			t.Errorf("the blueprint holds %q:\n%s", banned, text)
		}
	}
	if !slices.Equal(conv.Friction, p.Friction) {
		t.Errorf("friction = %v", conv.Friction)
	}
	if !strings.Contains(conv.Departures, "why rotate") {
		t.Errorf("departures = %q", conv.Departures)
	}
}

func TestConvertPlanJudgmentList(t *testing.T) {
	conv, _ := convertedFixture(t)
	all := strings.Join(conv.Judgment, "\n")
	for _, want := range []string{
		`section "Before you begin" (before-you-begin): write its why`,
		`section "Delete the old key" (delete-the-old-key): write its why`,
		`item "f-renew": its use does not start with Only or Omit`,
		`hint: section before-you-begin opens names the heading "Create the new key"; [[create-the-new-key]]`,
		`hint: item h-next text names the heading "Delete the old key"`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("the judgment list lacks %q:\n%s", want, all)
		}
	}
	for _, not := range []string{`item "f-ttl"`, `item "f-plain"`} {
		if strings.Contains(all, not) {
			t.Errorf("the judgment list holds %q:\n%s", not, all)
		}
	}
	if n := strings.Count(all, ": write its why"); n != 5 {
		t.Errorf("%d sections need a why, want 5", n)
	}
}

func TestConvertPlanFraming(t *testing.T) {
	tests := []struct {
		name, framing, wantErr string
	}{
		{"unknown heading is refused by name", "# Framing record: X\n\n## Who arrives, from where, and why\n\nA.\n\n## Surprise section\n\nB.\n", `"Surprise section"`},
		{"a repeated heading is refused", "## Intro plan\nA.\n## Intro plan\nB.\n", "twice"},
		{"departures alone is known", "## Departures from the plan's introduction\nMoves.\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := fixtureInput(t)
			in.Framing = []byte(tt.framing)
			conv, err := ConvertPlan(in)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(conv.Data), "Moves") || conv.Departures != "Moves." {
					t.Errorf("departures leaked or were lost: %q", conv.Departures)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want one naming %s", err, tt.wantErr)
			}
		})
	}
}

func TestConvertPlanFramingPreamble(t *testing.T) {
	in := fixtureInput(t)
	in.Framing = []byte("# Framing record: X\n\nWritten by the framing step; inputs read: the outline.\n\n## Intro plan\nA.\n")
	conv, err := ConvertPlan(in)
	if err != nil {
		t.Fatal(err)
	}
	if conv.Preamble != "Written by the framing step; inputs read: the outline." {
		t.Errorf("preamble = %q", conv.Preamble)
	}
	if strings.Contains(string(conv.Data), "Written by the framing step") {
		t.Error("the preamble reached the blueprint")
	}
}

func TestConvertPlanSectionIDs(t *testing.T) {
	in := fixtureInput(t)
	p := in.Plan
	p.Sections = []plan.Section{
		{Heading: "Set up", Level: 2, Opens: "a\n", Items: []plan.Item{{ID: "check", Kind: "handoff", Text: "x\n"}}},
		{Heading: "Set-up", Level: 2, Opens: "b\n", Items: []plan.Item{{ID: "z", Kind: "handoff", Text: "x\n"}}},
		{Heading: "Check", Level: 2, Opens: "c\n", Items: []plan.Item{{ID: "y", Kind: "handoff", Text: "x\n"}}},
		{Heading: "set up", Level: 2, Opens: "d\n", Items: []plan.Item{{ID: "w", Kind: "handoff", Text: "x\n"}}},
		{Heading: "???", Level: 2, Opens: "e\n", Items: []plan.Item{{ID: "v", Kind: "handoff", Text: "x\n"}}},
		{Heading: "Ignored", Anchor: "Custom_Anchor", Level: 2, Opens: "f\n", Items: []plan.Item{{ID: "u", Kind: "handoff", Text: "x\n"}}},
	}
	conv, err := ConvertPlan(in)
	if err != nil {
		t.Fatal(err)
	}
	b, findings := Decode("x", conv.Data)
	if len(findings) > 0 {
		t.Fatalf("findings: %+v", findings)
	}
	var ids []string
	for _, s := range b.Sections {
		ids = append(ids, s.ID)
		if !plan.ValidID(s.ID) {
			t.Errorf("id %q is not valid", s.ID)
		}
	}
	if want := []string{"set-up", "set-up-2", "check-2", "set-up-3", "section", "custom-anchor"}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	all := strings.Join(conv.Judgment, "\n")
	for _, want := range []string{`"Set-up": the id "set-up" was taken, so it is "set-up-2"`, `"Check": the id "check" was taken, so it is "check-2"`,
		`"set up": the id "set-up" was taken, so it is "set-up-3"`} {
		if !strings.Contains(all, want) {
			t.Errorf("judgment lacks %q:\n%s", want, all)
		}
	}
}

func TestConvertOutline(t *testing.T) {
	data, err := ConvertOutline([]NamedData{
		{Name: "a", Data: convertFixture(t, "outline-a.json")},
		{Name: "b", Data: convertFixture(t, "outline-b.json")},
	})
	if err != nil {
		t.Fatal(err)
	}
	o, findings := DecodeOutline("outline.yaml", data)
	if len(findings) > 0 {
		t.Fatalf("findings %+v:\n%s", findings, data)
	}
	if len(o.Pages) != 2 || o.Pages[0].Path != "docs/a.md" || o.Pages[1].Path != "docs/b.md" {
		t.Fatalf("pages = %+v", o.Pages)
	}
	if len(o.Types) != 2 || o.Types[0].Name != "task-guide" || o.Types[1].Name != "reference" || o.Types[0].Anatomy != "" {
		t.Errorf("types = %+v", o.Types)
	}
	if !o.Pages[0].Figure || o.Pages[1].Figure || !o.Pages[0].Managed {
		t.Errorf("figure or managed lost: %+v", o.Pages)
	}
	if b := o.Pages[1]; b.Title != "[[Page]] B: the sequel # two" || !slices.Equal(CoverStrings(b.Covers), []string{"x: y"}) || b.OutOfScope != nil {
		t.Errorf("page b = %+v", b)
	}
	if len(o.CrossLinks) != 1 || o.CrossLinks[0].Why != "A leads to B." {
		t.Errorf("crossLinks = %+v", o.CrossLinks)
	}
	if strings.Contains(string(data), "anatomy") || strings.Contains(string(data), "managed") {
		t.Errorf("an unset field was written:\n%s", data)
	}
}

func TestConvertOutlineKeepsRiskyProse(t *testing.T) {
	jo := `{"pages":[{"path":"docs/x.md","slug":"x","title":"[[x]] leads","pageType":"t",
	  "job":"Do this # not a comment: and this\n\nthen [[that]]","covers":["# heading","a: b","[[ref]] first","yes","line\none"],"kept":true}]}`
	data, err := ConvertOutline([]NamedData{{Name: "x.json", Data: []byte(jo)}})
	if err != nil {
		t.Fatal(err)
	}
	o, findings := DecodeOutline("outline.yaml", data)
	if len(findings) > 0 {
		t.Fatalf("findings %+v:\n%s", findings, data)
	}
	p := o.Pages[0]
	if p.Title != "[[x]] leads" || p.Job != "Do this # not a comment: and this\n\nthen [[that]]" ||
		!slices.Equal(CoverStrings(p.Covers), []string{"# heading", "a: b", "[[ref]] first", "yes", "line\none"}) {
		t.Errorf("a value changed on the round trip: %+v\n%s", p, data)
	}
}

func TestConvertOutlineErrors(t *testing.T) {
	tests := []struct{ name, json, want string }{
		{"not json", `{`, "outline x.json"},
		{"no pages", `{"crossLinks":[]}`, "no pages array"},
		{"duplicate path", `{"pages":[{"path":"a.md","slug":"a","title":"A","pageType":"t","job":"j"},{"path":"a.md","slug":"b","title":"A","pageType":"t","job":"j"}]}`, "does not decode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ConvertOutline([]NamedData{{Name: "x.json", Data: []byte(tt.json)}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestConvertPlanRiskyProse(t *testing.T) {
	in := fixtureInput(t)
	risky := []string{"Use [[verify]] # as the check: now", "# opens with a hash", "[[verify]] first", "yes", "a: b"}
	in.Plan.Constraints = risky
	in.Plan.Sections[0].Items = []plan.Item{{ID: "h", Kind: "handoff", Text: "[[verify]] # x: y"}}
	conv, err := ConvertPlan(in)
	if err != nil {
		t.Fatal(err)
	}
	b, findings := Decode("x", conv.Data)
	if len(findings) > 0 {
		t.Fatalf("findings %+v:\n%s", findings, conv.Data)
	}
	if !slices.Equal(b.Constraints, risky) || b.Sections[0].Items[0].Text != "[[verify]] # x: y" {
		t.Errorf("a value changed on the round trip:\n%s", conv.Data)
	}
}

// git runs git in dir with a fixed identity and no signing.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// seedRepo builds a repository with the page committed and returns its root.
func seedRepo(t *testing.T, page string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	file := filepath.Join(dir, filepath.FromSlash(convertPage))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "page")
	return dir
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(h[:])
}

func TestInstallConvertedSeedsTheLock(t *testing.T) {
	const (
		intro  = "# Rotate the widget key\n\nIntro text.\n\n"
		before = "You need access.\n\n"
		create = "Generate it.\n\n"
		size   = "Pick a size.\n\n"
		verify = "Check it.\n"
	)
	page := intro + "## Before you begin\n" + before + "## Create the new key\n" + create + "### Choose a size\n" + size + "## Verify the key\n" + verify
	root := seedRepo(t, page)
	conv, _ := convertedFixture(t)

	got, err := InstallConverted(root, testLayout, convertPage, conv.Data)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Seeded || got.Blueprint != "blueprints/docs/guides/rotate-the-widget-key.blueprint.yaml" {
		t.Fatalf("installed = %+v", got)
	}
	written, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(got.Blueprint)))
	if err != nil || string(written) != string(conv.Data) {
		t.Fatalf("the blueprint on disk differs from the conversion (%v)", err)
	}

	lock, err := ReadLock(filepath.Join(root, "blueprints", "drafted.lock"))
	if err != nil {
		t.Fatal(err)
	}
	e, ok := lock.Pages[convertPage]
	if !ok || len(lock.Pages) != 1 {
		t.Fatalf("lock pages = %+v", lock.Pages)
	}
	if want := git(t, root, "rev-parse", "HEAD:"+convertPage); e.Page != want {
		t.Errorf("page id = %q, want %q", e.Page, want)
	}
	if want := git(t, root, "hash-object", got.Blueprint); e.Blueprint != want {
		t.Errorf("blueprint id = %q, want %q", e.Blueprint, want)
	}
	if e.Accepted != "" {
		t.Errorf("accepted = %q, want empty", e.Accepted)
	}
	// A region is the lines below its heading joined by newlines, so each one loses the newline
	// that ends its last line unless the page ends there.
	cut := func(s string) string { return strings.TrimSuffix(s, "\n") }
	wantSections := map[string]string{
		IntroKey:             sum(cut(intro)),
		"before-you-begin":   sum(cut(before)),
		"create-the-new-key": sum(cut(create)),
		"choose-a-size":      sum(cut(size)),
		"verify-the-key":     sum(verify),
	}
	if !reflect.DeepEqual(e.Sections, wantSections) {
		t.Errorf("sections = %v, want %v", e.Sections, wantSections)
	}

	// A second conversion is refused twice over: the blueprint and the lock entry both exist.
	if _, err := InstallConverted(root, testLayout, convertPage, conv.Data); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("a second install gave %v", err)
	}
}

func TestInstallConvertedRefusals(t *testing.T) {
	conv, _ := convertedFixture(t)
	bpRel := "blueprints/docs/guides/rotate-the-widget-key.blueprint.yaml"

	t.Run("an existing blueprint is never overwritten", func(t *testing.T) {
		root := seedRepo(t, "## Before you begin\nx\n")
		dest := filepath.Join(root, filepath.FromSlash(bpRel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, []byte("mine\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := InstallConverted(root, testLayout, convertPage, conv.Data); err == nil {
			t.Fatal("expected a refusal")
		}
		if got, _ := os.ReadFile(dest); string(got) != "mine\n" {
			t.Errorf("the blueprint was overwritten: %q", got)
		}
	})

	t.Run("an existing lock entry is refused and nothing is written", func(t *testing.T) {
		root := seedRepo(t, "## Before you begin\nx\n")
		lockFile := filepath.Join(root, "blueprints", "drafted.lock")
		if err := os.MkdirAll(filepath.Dir(lockFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := WriteLock(lockFile, &Lock{Version: Version, Pages: map[string]LockEntry{convertPage: {Accepted: "abc"}}}); err != nil {
			t.Fatal(err)
		}
		_, err := InstallConverted(root, testLayout, convertPage, conv.Data)
		if err == nil || !strings.Contains(err.Error(), "already has an entry") {
			t.Fatalf("err = %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(bpRel))); statErr == nil {
			t.Error("a blueprint was written despite the refusal")
		}
	})

	t.Run("a page with uncommitted changes is refused", func(t *testing.T) {
		root := seedRepo(t, "## Before you begin\nx\n")
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(convertPage)), []byte("edited\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := InstallConverted(root, testLayout, convertPage, conv.Data)
		if err == nil || !strings.Contains(err.Error(), "uncommitted") {
			t.Fatalf("err = %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(bpRel))); statErr == nil {
			t.Error("a blueprint was written despite the refusal")
		}
	})

	t.Run("a missing page writes the blueprint and seeds nothing", func(t *testing.T) {
		root := t.TempDir()
		got, err := InstallConverted(root, testLayout, convertPage, conv.Data)
		if err != nil || got.Seeded {
			t.Fatalf("got %+v, %v", got, err)
		}
		if _, err := os.Stat(filepath.Join(root, "blueprints", "drafted.lock")); err == nil {
			t.Error("a lock was written for a page that does not exist")
		}
	})
}

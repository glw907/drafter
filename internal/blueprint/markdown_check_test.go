package blueprint

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

const (
	guideMD     = "blueprints/docs/guide.blueprint.md"
	guidePage   = "docs/guide.md"
	tracksBlock = "tracks:\n  - name: operate\n    reader: Operators who run widgets.\n    standard: google\n    paths: [\"docs/**\"]\n"
)

// goodMD is a markdown blueprint for docs/guide.md that passes every check.
const goodMD = `---
page: Rotate the widget key
type: task guide
track: operate
exemplar: guides/rotate-a-secret
---

# Rotate the widget key

:::reader
**Who reads it.** An operator who rotates keys. :fact[aaaaaa]
:::

## Before you begin

*Gets the reader ready to rotate.*

- You need the admin role. :fact[bbbbbb]
- See [the key reference](https://example.com/docs/keys).

## Rotate the key

*Walks the reader through the rotation.*

1. Generate a key in the service settings.

:::row
**Symptom.** The settings page refuses to generate a key.
**Cause.** The account lacks the admin role.
**Fix.** Ask an admin to grant the role. :fact[bbbbbb]
:::
`

// mdProject builds a project whose only blueprint is the markdown text md for docs/guide.md,
// under an outline that declares the operate track.
func mdProject(t *testing.T, md string, extra map[string]string) Project {
	t.Helper()
	files := map[string]string{
		guideBlueprint:            "",
		guideMD:                   md,
		"blueprints/outline.yaml": readFixture(t, "outline.yaml") + tracksBlock,
	}
	maps.Copy(files, extra)
	return catalogProject(t, files)
}

func errorsOf(fs []plan.Finding) []plan.Finding {
	return slices.DeleteFunc(slices.Clone(fs), func(f plan.Finding) bool { return f.Severity != plan.SeverityError })
}

func TestCheckMarkdownPasses(t *testing.T) {
	got, err := mdProject(t, goodMD, nil).Check(nil)
	if err != nil {
		t.Fatal(err)
	}
	if errs := errorsOf(got); len(errs) != 0 {
		t.Errorf("the good blueprint has errors: %+v", errs)
	}
}

func TestCheckMarkdownDefects(t *testing.T) {
	edit := func(old, new string) string {
		if !strings.Contains(goodMD, old) {
			t.Fatalf("goodMD has no %q", old)
		}
		return strings.Replace(goodMD, old, new, 1)
	}
	tests := []struct {
		name  string
		md    string
		extra map[string]string
		file  string
		line  int // 0 means the line of anchor
		anch  string
		msg   string
	}{
		{"no page key", edit("page: Rotate the widget key\n", ""), nil, guideMD, 1, "", "has no page"},
		{"no type key", edit("type: task guide\n", ""), nil, guideMD, 1, "", "has no type"},
		{"no track key", edit("track: operate\n", ""), nil, guideMD, 1, "", "has no track"},
		{"page differs from the outline", edit("page: Rotate the widget key", "page: Rotate a key"), nil, guideMD, 1, "", "outline titles"},
		{"type differs from the outline", edit("type: task guide", "type: symptom row"), nil, guideMD, 1, "", "the type \"task guide\""},
		{"track the outline does not declare", edit("track: operate", "track: admin"), nil, guideMD, 1, "", "not a track the outline declares"},
		{"track whose paths do not hold the page", goodMD, map[string]string{
			"blueprints/outline.yaml": readFixture(t, "outline.yaml") + strings.Replace(tracksBlock, "docs/**", "ref/**", 1)},
			guideMD, 1, "", "do not hold docs/guide.md"},
		{"page the outline does not list", goodMD, map[string]string{"blueprints/docs/other.blueprint.md": goodMD, guideMD: ""},
			"blueprints/docs/other.blueprint.md", 1, "", "lists no page"},
		{"no reader block", edit(":::reader\n**Who reads it.** An operator who rotates keys. :fact[aaaaaa]\n:::\n", ""), nil, guideMD, 1, "", "no :::reader block"},
		{"second reader block", edit("## Before you begin", ":::reader\nAgain.\n:::\n\n## Before you begin"), nil, guideMD, 0, ":::reader", "second :::reader"},
		{"section without a job line", edit("*Gets the reader ready to rotate.*\n\n", ""), nil, guideMD, 0, "## Before you begin", "no job line"},
		{"row without a cause", edit("**Cause.** The account lacks the admin role.\n", ""), nil, guideMD, 0, ":::row", "no **Cause.** part"},
		{"headings sharing a slug", edit("## Rotate the key", "## Before you begin"), nil, guideMD, 0, "## Before you begin", "same slug"},
		{"tag on a wrapped bullet's second line", edit("- You need the admin role. :fact[bbbbbb]", "- You need the admin\n  role. :fact[bbbbbb]"), nil, guideMD, 0, "  role.", "not on a bullet, step, row, or reader line"},
		{"tag in a paragraph", edit("1. Generate", "The key is generated once. :fact[bbbbbb]\n\n1. Generate"), nil, guideMD, 0, "The key is generated", "not on a bullet, step, row, or reader line"},
		{"tagged bullet above the first section", edit("## Before you begin", "- Early. :fact[bbbbbb]\n\n## Before you begin"), nil, guideMD, 0, "- Early.", "not on a bullet, step, row, or reader line"},
		{"tag in a heading", edit("## Rotate the key", "## Rotate the key :fact[bbbbbb]"), nil, guideMD, 0, "## Rotate the key", "not on a bullet, step, row, or reader line"},
		{"tag on a row opener", edit(":::row", ":::row :fact[bbbbbb]"), nil, guideMD, 0, ":::row", "not on a bullet, step, row, or reader line"},
		{"tag on a job line", edit("*Gets the reader ready to rotate.*", "*Gets the reader ready to rotate.* :fact[bbbbbb]\n\n*Gets the reader ready to rotate.*"), nil, guideMD, 0, "rotate.* :fact", "not on a bullet, step, row, or reader line"},
		{"fact the store lacks", edit(":fact[bbbbbb]\n- See", ":fact[zzzzzz]\n- See"), nil, guideMD, 0, "zzzzzz", "in no facts file"},
		{"a yaml blueprint beside the markdown one", goodMD, map[string]string{guideBlueprint: readFixture(t, "base.blueprint.yaml")},
			guideBlueprint, 1, "", "both a .blueprint.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mdProject(t, tt.md, tt.extra).Check(nil)
			if err != nil {
				t.Fatal(err)
			}
			errs := slices.DeleteFunc(errorsOf(got), func(f plan.Finding) bool { return f.File != tt.file })
			if len(errs) != 1 || errs[0].File != tt.file || !strings.Contains(errs[0].Message, tt.msg) {
				t.Fatalf("errors = %+v, want exactly one in %s containing %q", errs, tt.file, tt.msg)
			}
			want := tt.line
			if want == 0 {
				want = strings.Count(tt.md[:strings.LastIndex(tt.md, tt.anch)], "\n") + 1
			}
			if errs[0].Line != want {
				t.Errorf("line = %d, want %d", errs[0].Line, want)
			}
		})
	}
}

func TestCheckMarkdownTrackWhenOutlineHasNone(t *testing.T) {
	noTrack := strings.Replace(goodMD, "track: operate\n", "", 1)
	tests := []struct {
		name, md, outline, msg string
	}{
		{"no tracks and no track key passes", noTrack, "", ""},
		{"no tracks and a track named fails", goodMD, "", "the outline declares no tracks"},
		{"tracks declared and no track key fails", noTrack, tracksBlock, "has no track"},
		{"tracks declared and the right track passes", goodMD, tracksBlock, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mdProject(t, tt.md, map[string]string{"blueprints/outline.yaml": readFixture(t, "outline.yaml") + tt.outline})
			got, err := p.Check(nil)
			if err != nil {
				t.Fatal(err)
			}
			errs := errorsOf(got)
			if tt.msg == "" {
				if len(errs) != 0 {
					t.Errorf("errors = %+v, want none", errs)
				}
				return
			}
			if len(errs) != 1 || !strings.Contains(errs[0].Message, tt.msg) {
				t.Errorf("errors = %+v, want exactly one containing %q", errs, tt.msg)
			}
		})
	}
}

func TestCheckMarkdownExhaustive(t *testing.T) {
	const bullet = "- You need the admin role."
	tests := []struct {
		name, old, new string
		anch           string // the text whose line the warning is on
		want           string // "" for no warning
	}{
		{"clean blueprint", bullet, bullet, "", ""},
		{"bullet", bullet, "- Only an admin can rotate a key.", "Only an admin", "only"},
		{"step", "1. Generate a key", "1. Always generate a key", "Always generate", "always"},
		{"row fix", "**Fix.** Ask an admin", "**Fix.** Never ask an admin", ":::row", "never"},
		{"reader", "An operator who rotates keys.", "Every operator who rotates keys.", "Every operator", "every"},
		{"constraints", "## Before you begin", ":::constraints\nSkip all setup.\n:::\n\n## Before you begin", "Skip all setup", "all"},
		{"spelled count", bullet, "- Three roles can rotate a key.", "Three roles", "three roles"},
		{"digit count", bullet, "- Keys have 4 steps.", "Keys have", "4 steps"},
		{"none", bullet, "- None of the roles rotate a key.", "None of", "none"},
		{"no", bullet, "- No role rotates a key.", "No role", "no"},
		{"mixed case", bullet, "- ONLY an admin rotates.", "ONLY", "only"},
		{"inside a code span", bullet, "- Run `rotate --all` to start.", "", ""},
		{"inside a fact tag", bullet, "- You need the role. :fact[bbbbbb-all]", "", ""},
		{"part of a longer word", bullet, "- Call the normal allocator, a no-op.", "", ""},
		{"count before a singular", bullet, "- Make 4 attempt at once, then a 3 minute wait.", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := strings.Replace(goodMD, tt.old, tt.new, 1)
			got, err := mdProject(t, md, nil).Check(nil)
			if err != nil {
				t.Fatal(err)
			}
			var ws []plan.Finding
			for _, f := range got {
				if f.Check == plan.CheckExhaustive {
					ws = append(ws, f)
				}
			}
			if tt.want == "" {
				if len(ws) != 0 {
					t.Fatalf("warnings = %+v, want none", ws)
				}
				return
			}
			if len(ws) != 1 || ws[0].Severity != plan.SeverityWarning || !strings.Contains(ws[0].Message, `"`+tt.want+`"`) {
				t.Fatalf("warnings = %+v, want one on %q", ws, tt.want)
			}
			if want := strings.Count(md[:strings.Index(md, tt.anch)], "\n") + 1; ws[0].Line != want {
				t.Errorf("line = %d, want %d", ws[0].Line, want)
			}
		})
	}
}

func TestRefsMarkdown(t *testing.T) {
	p := mdProject(t, goodMD, nil)
	for _, id := range []string{"f:bbbbbb", "bbbbbb"} {
		refs, held, err := p.Refs(id)
		if err != nil || !held {
			t.Fatalf("Refs(%s) = %v, held %v, %v", id, refs, held, err)
		}
		want := []Ref{
			{guideMD, lineOf(t, goodMD, "You need the admin role."), "before-you-begin.p1", "example"},
			{guideMD, lineOf(t, goodMD, "**Fix.**"), "rotate-the-key.row1", "example"},
		}
		if !slices.Equal(refs, want) {
			t.Errorf("Refs(%s) = %+v, want %+v", id, refs, want)
		}
	}
	refs, _, err := p.Refs("f:aaaaaa")
	if err != nil || len(refs) != 1 || refs[0].Item != "reader" || refs[0].Line != lineOf(t, goodMD, "An operator") {
		t.Errorf("reader ref = %+v, %v", refs, err)
	}
}

// yamlGuide is the catalog's base blueprint with a cause on its one failure, so its conversion has
// all three parts of a row.
func yamlGuide(t *testing.T) string {
	return withEdit(t, "        handledBy: h-ask-admin", "        cause: |\n          The account lacks the admin role.\n        handledBy: h-ask-admin")
}

const guidePageText = "# Rotate the widget key\n\nIntro.\n\n## Before you begin\n\nx\n\n## Rotate the key\n\ny\n\n## Verify the new key\n\nz\n"

func TestConvertToMarkdownGolden(t *testing.T) {
	p := catalogProject(t, map[string]string{
		guideBlueprint:            yamlGuide(t),
		"blueprints/outline.yaml": readFixture(t, "outline.yaml") + tracksBlock,
	})
	git(t, p.Root, "init", "-q")
	wantFacts, err := p.Cited(guideBlueprint)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.ConvertToMarkdown([]string{guideBlueprint})
	if err != nil || len(got) != 1 || got[0].To != guideMD {
		t.Fatalf("ConvertToMarkdown = %+v, %v", got, err)
	}
	if _, err := os.Stat(p.abs(guideBlueprint)); err == nil {
		t.Error("the YAML blueprint was not deleted")
	}
	data, err := os.ReadFile(p.abs(guideMD))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"page: Rotate the widget key\ntype: task guide\ntrack: operate\nexemplar: guides/rotate-a-secret\n",
		"## Rotate the key\n\n*Rotation is the page's job, so it comes right after the prerequisites.*\n",
		"- You need the admin role on the widget service.\n",
		"- Rotating a key needs the admin role. :fact[bbbbbb]\n",
		"1. Generate a new key in the service settings.\n",
		"- See [widget key reference](https://example.com/docs/keys).\n",
		"**Fix.** Ask an admin to grant the role, then try again.\n:::",
		"**Cause.** The account lacks the admin role.\n",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("the conversion lacks %q:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "- Ask an admin to grant the role") {
		t.Error("the failure's handler is repeated as a bullet")
	}

	findings, err := p.Check(nil)
	if err != nil {
		t.Fatal(err)
	}
	if errs := errorsOf(findings); len(errs) != 0 {
		t.Errorf("the converted blueprint has errors: %+v", errs)
	}
	m, _ := DecodeMarkdown(guideMD, data)
	var heads []string
	for _, h := range m.SectionHeads() {
		heads = append(heads, h.Heading)
	}
	if want := []string{"Before you begin", "Rotate the key", "Verify the new key"}; !slices.Equal(heads, want) {
		t.Errorf("headings = %v, want %v", heads, want)
	}
	gotFacts, err := p.Cited(guideMD)
	if err != nil || !slices.Equal(gotFacts, wantFacts) {
		t.Errorf("cited facts = %v, %v; want the YAML's %v", gotFacts, err, wantFacts)
	}
}

func TestConvertToMarkdownLockRoundTrip(t *testing.T) {
	setup := func(t *testing.T) (Project, string) {
		t.Helper()
		p := catalogProject(t, map[string]string{
			guideBlueprint:            yamlGuide(t),
			guidePage:                 guidePageText,
			"blueprints/outline.yaml": readFixture(t, "outline.yaml") + tracksBlock,
		})
		git(t, p.Root, "init", "-q")
		git(t, p.Root, "add", ".")
		git(t, p.Root, "commit", "-q", "-m", "draft")
		b, _ := Decode(guideBlueprint, []byte(yamlGuide(t)))
		bpBlob, _ := FileBlobID(p.Root, guideBlueprint)
		pageBlob, _ := FileBlobID(p.Root, guidePage)
		lock := &Lock{Version: Version, Pages: map[string]LockEntry{guidePage: {
			Page: pageBlob, Blueprint: bpBlob, Accepted: bpBlob, Sections: SectionHashes(b, []byte(guidePageText)),
		}}}
		if err := WriteLock(p.abs("blueprints/drafted.lock"), lock); err != nil {
			t.Fatal(err)
		}
		git(t, p.Root, "add", ".")
		git(t, p.Root, "commit", "-q", "-m", "lock")
		return p, pageBlob
	}

	t.Run("a converted page stays up to date", func(t *testing.T) {
		p, pageBlob := setup(t)
		if _, err := p.ConvertToMarkdown([]string{guideBlueprint}); err != nil {
			t.Fatal(err)
		}
		git(t, p.Root, "add", ".")
		git(t, p.Root, "commit", "-q", "-m", "convert")
		lock, err := ReadLock(p.abs("blueprints/drafted.lock"))
		if err != nil {
			t.Fatal(err)
		}
		e := lock.Pages[guidePage]
		md, err := os.ReadFile(p.abs(guideMD))
		if err != nil {
			t.Fatal(err)
		}
		if e.Page != pageBlob || e.Accepted != "" || e.Blueprint != git(t, p.Root, "hash-object", guideMD) {
			t.Errorf("entry = %+v", e)
		}
		for _, k := range []string{IntroKey, "before-you-begin", "rotate-the-key", "verify-the-new-key"} {
			if e.Sections[k] == "" {
				t.Errorf("sections lack %s: %v", k, e.Sections)
			}
		}
		introduced, err := LockIntroducedAt(p.Root, "blueprints/drafted.lock", guidePage, e)
		if err != nil || introduced == "" {
			t.Fatalf("LockIntroducedAt = %q, %v", introduced, err)
		}
		o, _ := DecodeOutline(p.Outline, []byte(readFixture(t, "outline.yaml")+tracksBlock))
		facts, err := plan.LoadFacts(p.FactsDir)
		if err != nil {
			t.Fatal(err)
		}
		old := p.revisionAt(ParentCommit(p.Root, introduced), guidePage)
		old.Blueprint = BlobBytes(p.Root, e.Blueprint)
		cur := Revision{Blueprint: md, Entry: &o.Pages[0], Facts: facts}
		if sc := Compare(guideMD, old, cur); sc.Kind != ScopeUnchanged {
			t.Errorf("Compare = %+v, want unchanged", sc)
		}
	})

	t.Run("a stale page is refused and nothing is written", func(t *testing.T) {
		p, _ := setup(t)
		edited := strings.Replace(yamlGuide(t), "Generate a new key in the service settings.", "Generate a key.", 1)
		if err := os.WriteFile(p.abs(guideBlueprint), []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := p.ConvertToMarkdown([]string{guideBlueprint})
		if err == nil || !strings.Contains(err.Error(), "stale") {
			t.Fatalf("err = %v", err)
		}
		if _, err := os.Stat(p.abs(guideMD)); err == nil {
			t.Error("a markdown blueprint was written despite the refusal")
		}
		if _, err := os.Stat(filepath.Join(p.Root, filepath.FromSlash(guideBlueprint))); err != nil {
			t.Error("the YAML blueprint was deleted despite the refusal")
		}
	})
}

func TestCompareMarkdown(t *testing.T) {
	entry := &Page{Path: guidePage, Title: "Rotate the widget key"}
	facts := func(text string) *plan.Facts {
		return plan.ParseFacts(map[string]string{"f.md": "# Facts\n\n- `f:bbbbbb` " + text + " Source: `src/rotate.go`. [verified]\n"}, "")
	}
	rev := func(md string, f *plan.Facts) Revision {
		return Revision{Blueprint: []byte(md), Entry: entry, Facts: f}
	}
	base := facts("Rotating needs the admin role.")
	tests := []struct {
		name string
		cur  Revision
		kind string
		ids  []string
	}{
		{"identical", rev(goodMD, base), ScopeUnchanged, nil},
		{"a bullet reworded", rev(strings.Replace(goodMD, "You need the admin role.", "You need admin.", 1), base), ScopeScoped, []string{"before-you-begin"}},
		{"a row reworded", rev(strings.Replace(goodMD, "The account lacks the admin role.", "No role.", 1), base), ScopeScoped, []string{"rotate-the-key"}},
		{"a new section", rev(goodMD+"\n## Verify\n\n*Checks it.*\n\n- It works.\n", base), ScopeScoped, []string{"verify"}},
		{"a cited fact changed", rev(goodMD, facts("Rotating needs the owner role.")), ScopeScoped, []string{"before-you-begin", "rotate-the-key"}},
		{"the reader reworded", rev(strings.Replace(goodMD, "An operator who rotates keys.", "An admin.", 1), base), ScopeScoped, nil},
		{"undecodable", rev("no front matter", base), ScopeWhole, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compare(guideMD, rev(goodMD, base), tt.cur)
			if got.Kind != tt.kind || !slices.Equal(got.IDs, tt.ids) {
				t.Errorf("Compare = %+v, want %s %v", got, tt.kind, tt.ids)
			}
		})
	}
}

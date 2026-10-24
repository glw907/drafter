package blueprint

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/stock"
)

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"docs/guide.md", "docs/guide.md", true},
		{"docs/*.md", "docs/guide.md", true},
		{"docs/*.md", "docs/a/guide.md", false},
		{"docs/**", "docs/guide.md", true},
		{"docs/**", "docs/a/b/guide.md", true},
		{"docs/**", "other/guide.md", false},
		{"docs/**/guide.md", "docs/guide.md", true},
		{"docs/**/guide.md", "docs/a/b/guide.md", true},
		{"docs/**/guide.md", "docs/a/other.md", false},
		{"**/*.md", "README.md", true},
		{"**/*.md", "a/b/c.md", true},
		{"**/*.md", "a/b/c.txt", false},
		{"**", "a/b", true},
		{"docs/[a-c]*.md", "docs/beta.md", true},
		{"docs/[a-c]*.md", "docs/zed.md", false},
		{"docs/[a", "docs/a", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+" "+tt.name, func(t *testing.T) {
			if got := globMatch(tt.pattern, tt.name); got != tt.want {
				t.Errorf("globMatch(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
			}
		})
	}
}

func TestGlobError(t *testing.T) {
	tests := []struct {
		pattern string
		bad     bool
	}{
		{"docs/**", false},
		{"docs/*/[a-c].md", false},
		{"docs/[a", true},
		{"**/[", true},
		{"docs/\\", true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			if err := globError(tt.pattern); (err != nil) != tt.bad {
				t.Errorf("globError(%q) = %v, want error: %v", tt.pattern, err, tt.bad)
			}
		})
	}
}

func TestRegisterEntries(t *testing.T) {
	src := "# Register\n\n## Voice\n\n### Not an anatomy\n\n## The page anatomies\n\nIntro.\n\n" +
		"- **Concept page** (the extend track). It opens.\n- **Tutorial milestone**: a step.\n* **Symptom row**\n" +
		"  continued text\n\n### Heading entry\n\nText with **bold** inside.\n\n## After\n\n- **Outside**: not an entry.\n"
	entries, found := registerEntries([]byte(src))
	if !found {
		t.Fatal("section not found")
	}
	want := []string{"Concept page", "Tutorial milestone", "Symptom row", "Heading entry"}
	if !slices.Equal(entries, want) {
		t.Errorf("entries = %q, want %q", entries, want)
	}
	if entries, found := registerEntries([]byte("# Register\n\n- **Task guide**: x\n")); found || len(entries) != 0 {
		t.Errorf("a register with no section gave %q, %v", entries, found)
	}
}

func TestDecodeTracks(t *testing.T) {
	head := "version: 1\ntypes:\n  - name: t\npages:\n  - path: a.md\n    managed: false\ntracks:\n"
	track := "  - name: admin\n    reader: Admins.\n    standard: google\n    paths: [\"docs/**\"]\n"
	tests := []struct {
		name, src, want string
	}{
		{"a valid track", head + track, ""},
		{"an empty tracks list", head[:len(head)-1] + " []\n", ""},
		{"a missing reader", head + strings.Replace(track, "    reader: Admins.\n", "", 1), `missing required field "reader"`},
		{"a missing standard", head + strings.Replace(track, "    standard: google\n", "", 1), `missing required field "standard"`},
		{"a missing name", head + strings.Replace(track, "  - name: admin\n    reader", "  - reader", 1), `missing required field "name"`},
		{"a missing paths", head + strings.Replace(track, "    paths: [\"docs/**\"]\n", "", 1), `missing required field "paths"`},
		{"an empty reader", head + strings.Replace(track, "Admins.", `""`, 1), "is empty"},
		{"an empty paths list", head + strings.Replace(track, `["docs/**"]`, "[]", 1), "must hold at least one entry"},
		{"a repeated name", head + track + track, "declared twice"},
		{"an unknown field", head + track + "    owner: me\n", `unknown field "owner"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, fs := DecodeOutline("outline.yaml", []byte(tt.src))
			if tt.want == "" {
				if len(fs) > 0 || o == nil {
					t.Errorf("findings = %v", fs)
				}
				return
			}
			if !slices.ContainsFunc(fs, func(f plan.Finding) bool {
				return f.Severity == plan.SeverityError && strings.Contains(f.Message, tt.want)
			}) {
				t.Errorf("findings = %v, want an error containing %q", fs, tt.want)
			}
		})
	}
}

const (
	tracksAdmin    = "  - name: admin\n    reader: Admins.\n    standard: google\n    paths: [\"docs/**\"]\n"
	tracksRegister = "docs/register.md"
)

// tracksOutline returns the catalog outline with the given tracks block and anatomy.
func tracksOutline(t *testing.T, anatomy, tracks string) string {
	t.Helper()
	o := strings.Replace(readFixture(t, "outline.yaml"), "anatomy: Task guide", "anatomy: "+anatomy, 1)
	switch {
	case strings.HasPrefix(tracks, "[]"):
		o += "tracks: []\n"
	case tracks != "":
		o += "tracks:\n" + tracks
	}
	return o
}

func tracksProject(t *testing.T, outline string, extra map[string]string) []plan.Finding {
	t.Helper()
	files := map[string]string{"blueprints/outline.yaml": outline}
	maps.Copy(files, extra)
	got, err := catalogProject(t, files).Check(nil)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestCheckTracks(t *testing.T) {
	admin := tracksAdmin
	tests := []struct {
		name   string
		tracks string
		extra  map[string]string
		sev    string // empty means no tracks finding at all
		msg    string
	}{
		{"one track matches the page", admin, nil, "", ""},
		{"an empty tracks list checks as no tracks", "[]\n", nil, "", ""},
		{"a page that matches no track", strings.Replace(admin, "docs/**", "guides/**", 1), nil, "error", "matches no track"},
		{"a page that matches two tracks", admin + strings.ReplaceAll(admin, "admin", "dev"), nil, "error", "matches 2 tracks"},
		{"an unknown standard", strings.Replace(admin, "google", "chicago", 1), nil, "error", `"chicago"`},
		{"a register that does not exist", admin + "    register: docs/nowhere.md\n", nil, "error", "docs/nowhere.md"},
		{"a register that exists", admin + "    register: " + tracksRegister + "\n", map[string]string{tracksRegister: "# R\n"}, "", ""},
		{"a malformed glob", strings.Replace(admin, `["docs/**"]`, `["docs/**", "docs/[a"]`, 1), nil, "error", "malformed glob"},
		{"a track that matches no page", admin + strings.ReplaceAll(strings.Replace(admin, "docs/**", "other/**", 1), "admin", "dev"), nil, "warning", "matches no managed page"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tracksProject(t, tracksOutline(t, "Task guide", tt.tracks), tt.extra)
			hits := findingsFor(got, "tracks")
			if tt.sev == "" {
				if len(hits) != 0 {
					t.Errorf("tracks findings = %+v, want none", hits)
				}
				return
			}
			for _, f := range hits {
				if f.Severity == tt.sev && strings.Contains(f.Message, tt.msg) && f.File == "blueprints/outline.yaml" && f.Line > 1 {
					return
				}
			}
			t.Errorf("no %s tracks finding containing %q at a line: %+v", tt.sev, tt.msg, hits)
		})
	}
}

// TestCheckTracksNamedFiles pins that the outline's track checks belong to the whole-project run.
func TestCheckTracksNamedFiles(t *testing.T) {
	p := catalogProject(t, map[string]string{"blueprints/outline.yaml": tracksOutline(t, "Task guide", strings.Replace(tracksAdmin, "docs/**", "guides/**", 1))})
	got, err := p.Check([]string{guideBlueprint})
	if err != nil {
		t.Fatal(err)
	}
	if hits := findingsFor(got, "tracks"); len(hits) != 0 {
		t.Errorf("a check of a named blueprint reported the outline's track findings: %+v", hits)
	}
	all, err := p.Check(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(findingsFor(all, "tracks")) == 0 {
		t.Error("a check of the whole project skipped the outline's track checks")
	}
}

func TestCheckAnatomies(t *testing.T) {
	registerAt := func(register string) string { return tracksAdmin + "    register: " + register + "\n" }
	holding := map[string]string{tracksRegister: "# R\n\n## The page anatomies\n\n- **Symptom row**: a row.\n\n### Tutorial milestone\n\nText.\n"}
	tests := []struct {
		name    string
		anatomy string
		tracks  string
		extra   map[string]string
		sev     string
		msg     string
	}{
		{"a stock name with no tracks", "Reference entry", "", nil, "", ""},
		{"a stock name with tracks", "Troubleshooting page", tracksAdmin, nil, "", ""},
		{"an unknown name with no tracks warns", "Symptom row", "", nil, "warning", `"Symptom row"`},
		{"an unknown name with tracks errors", "Symptom row", tracksAdmin, nil, "error", "names no register"},
		{"known by a register bullet", "Symptom row", registerAt(tracksRegister), holding, "", ""},
		{"known by a register heading", "Tutorial milestone", registerAt(tracksRegister), holding, "", ""},
		{"absent from the register", "Concept map", registerAt(tracksRegister), holding, "error", "has no entry for it"},
		{"a register with no anatomies section", "Symptom row", registerAt(tracksRegister), map[string]string{tracksRegister: "# R\n\n- **Symptom row**: a row.\n"}, "error", "no \"## The page anatomies\" section"},
		{
			"the page's track has no register though another track's register holds it", "Symptom row",
			tracksAdmin + "  - name: dev\n    reader: Developers.\n    standard: google\n    paths: [\"src/**\"]\n    register: " + tracksRegister + "\n",
			holding, "error", "names no register",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tracksProject(t, tracksOutline(t, tt.anatomy, tt.tracks), tt.extra)
			var hits []plan.Finding
			for _, f := range findingsFor(got, "types") {
				if strings.Contains(f.Message, "stock anatomy") {
					hits = append(hits, f)
				}
			}
			if tt.sev == "" {
				if len(hits) != 0 {
					t.Errorf("anatomy findings = %+v, want none", hits)
				}
				return
			}
			for _, f := range hits {
				if f.Severity == tt.sev && strings.Contains(f.Message, tt.msg) && strings.Contains(f.Fix, "Task guide") {
					return
				}
			}
			t.Errorf("no %s anatomy finding containing %q: %+v", tt.sev, tt.msg, hits)
		})
	}
}

func TestCheckAnatomyOfTypeWithoutPages(t *testing.T) {
	unused := "  - name: lookup\n    anatomy: Symptom row\n"
	tests := []struct {
		name, tracks string
		extra        map[string]string
		sev          string
	}{
		{"no tracks warns", "", nil, "warning"},
		{"tracks and no register errors", tracksAdmin, nil, "error"},
		{"any track's register holding it passes", tracksAdmin + "  - name: dev\n    reader: Developers.\n    standard: google\n    paths: [\"src/**\"]\n    register: " + tracksRegister + "\n",
			map[string]string{tracksRegister: "# R\n\n## The page anatomies\n\n- **Symptom row**: a row.\n"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outline := strings.Replace(tracksOutline(t, "Task guide", tt.tracks), "pages:\n", unused+"pages:\n", 1)
			got := tracksProject(t, outline, tt.extra)
			var sev string
			for _, f := range findingsFor(got, "types") {
				if strings.Contains(f.Message, `"lookup"`) {
					sev = f.Severity
				}
			}
			if sev != tt.sev {
				t.Errorf("severity = %q, want %q: %+v", sev, tt.sev, got)
			}
		})
	}
}

func TestCheckFraming(t *testing.T) {
	base := readFixture(t, "base.blueprint.yaml")
	i, j := strings.Index(base, "framing:\n"), strings.Index(base, "orderReason:")
	noFraming := base[:i] + base[j:]
	noPlan := strings.Replace(base, "  introPlan: |\n    Open on the task and say what the page covers.\n", "", 1)
	tests := []struct {
		name, bp, msg string
	}{
		{"no framing", noFraming, "has no framing"},
		{"framing with no introPlan", noPlan, "no introPlan"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.bp == base {
				t.Fatal("the edit changed nothing")
			}
			got, err := catalogProject(t, map[string]string{guideBlueprint: tt.bp}).Check(nil)
			if err != nil {
				t.Fatal(err)
			}
			if hits := findingsFor(got, "framing"); len(hits) != 1 || hits[0].Severity != plan.SeverityError || !strings.Contains(hits[0].Message, tt.msg) {
				t.Errorf("Check framing findings = %+v, want one error containing %q", hits, tt.msg)
			}

			p := catalogProject(t, nil)
			_, _, fs, err := p.CheckSource(guideBlueprint, []byte(tt.bp), []byte(readFixture(t, "outline.yaml")))
			if err != nil {
				t.Fatal(err)
			}
			if hits := findingsFor(fs, "framing"); len(hits) != 1 || !strings.Contains(hits[0].Message, tt.msg) {
				t.Errorf("CheckSource framing findings = %+v, want one containing %q", hits, tt.msg)
			}
		})
	}
	got, err := catalogProject(t, nil).Check(nil)
	if err != nil {
		t.Fatal(err)
	}
	if hits := findingsFor(got, "framing"); len(hits) != 0 {
		t.Errorf("the base blueprint has framing findings: %+v", hits)
	}
}

func TestConvertPlanWithoutFramingListsIntroPlan(t *testing.T) {
	in := fixtureInput(t)
	in.Framing = nil
	conv, err := ConvertPlan(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(conv.Judgment, "\n"), "framing.introPlan: write it") {
		t.Errorf("judgment lacks the introPlan line:\n%s", strings.Join(conv.Judgment, "\n"))
	}
	if b, fs := Decode("out.blueprint.yaml", conv.Data); b == nil || len(fs) > 0 {
		t.Errorf("the converted blueprint does not decode: %+v", fs)
	}
	withFraming, _ := convertedFixture(t)
	if strings.Contains(strings.Join(withFraming.Judgment, "\n"), "framing.introPlan") {
		t.Error("a plan with an intro plan still lists the introPlan line")
	}
}

func TestStockAnatomiesAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range stock.Anatomies {
		if seen[a] {
			t.Errorf("anatomy %q is listed twice", a)
		}
		seen[a] = true
	}
}

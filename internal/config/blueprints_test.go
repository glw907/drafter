package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlueprintDefaults(t *testing.T) {
	sb := newSandbox(t)
	writeFile(t, sb.project(), "mode = \"batch\"\n")
	c, err := Load(sb.root, Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Blueprints != (Blueprints{Root: "blueprints", Outline: "blueprints/outline.yaml"}) {
		t.Errorf("Blueprints = %+v", c.Blueprints)
	}
	if r := row(t, c, "blueprints.outline"); r.From != "default" || r.Value != "blueprints/outline.yaml" {
		t.Errorf("outline row = %+v", r)
	}
	if r := row(t, c, "blueprints.required"); r.From != "default" || r.Value != "false" {
		t.Errorf("required row = %+v", r)
	}
	if c.Facts.Dir != "" {
		t.Errorf("facts.dir = %q with no facts directory", c.Facts.Dir)
	}
}

func TestBlueprintSettingsLayer(t *testing.T) {
	sb := newSandbox(t)
	sb.ignoreLocal(t)
	writeFile(t, sb.project(), "[blueprints]\nroot = \"plans\"\nrequired = false\n")
	writeFile(t, sb.local(), "[blueprints]\nrequired = true\n")
	c, err := Load(sb.root, Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Blueprints{Root: "plans", Required: true, Outline: "plans/outline.yaml"}
	if c.Blueprints != want {
		t.Errorf("Blueprints = %+v, want %+v", c.Blueprints, want)
	}
	if r := row(t, c, "blueprints.required"); r.From != "drafter.local.toml" {
		t.Errorf("required source = %q", r.From)
	}
}

func TestBriefPathSetting(t *testing.T) {
	sb := newSandbox(t)
	writeFile(t, sb.project(), "mode = \"batch\"\n")
	c, err := Load(sb.root, Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if r := row(t, c, "blueprints.brief_path"); r.From != "unset" || c.Blueprints.BriefPath != "" {
		t.Errorf("unset brief_path row = %+v, value %q", r, c.Blueprints.BriefPath)
	}

	sb = newSandbox(t)
	sb.ignoreLocal(t)
	writeFile(t, sb.project(), "[blueprints]\nbrief_path = \"notes/{dir}/{name}.json\"\n")
	writeFile(t, sb.local(), "[blueprints]\nbrief_path = \"briefs/{dir}/{name}.json\"\n")
	c, err = Load(sb.root, Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Blueprints.BriefPath != "briefs/{dir}/{name}.json" {
		t.Errorf("brief_path = %q, want the local layer's", c.Blueprints.BriefPath)
	}
	if r := row(t, c, "blueprints.brief_path"); r.From != "drafter.local.toml" || r.Value != "briefs/{dir}/{name}.json" {
		t.Errorf("brief_path row = %+v", r)
	}
}

func TestFactsDirDefaultsToTheBlueprintRoot(t *testing.T) {
	tests := []struct {
		name    string
		project string
		dirs    []string
		want    string // relative to the project root, empty for unset
		from    string
	}{
		{"conventional directory exists", "mode = \"batch\"\n", []string{"blueprints/facts"}, "blueprints/facts", "default"},
		{"directory under a moved root", "[blueprints]\nroot = \"plans\"\n", []string{"plans/facts"}, "plans/facts", "default"},
		{"setting wins", "[facts]\ndir = \"elsewhere\"\n", []string{"blueprints/facts", "elsewhere"}, "elsewhere", "drafter.toml"},
		{"no directory leaves it unset", "mode = \"batch\"\n", nil, "", "unset"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			writeFile(t, sb.project(), tt.project)
			for _, d := range tt.dirs {
				if err := os.MkdirAll(filepath.Join(sb.root, d), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			c, err := Load(sb.root, Flags{})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			want := ""
			if tt.want != "" {
				want = filepath.Join(sb.root, tt.want)
			}
			if c.Facts.Dir != want {
				t.Errorf("facts.dir = %q, want %q", c.Facts.Dir, want)
			}
			if r := row(t, c, "facts.dir"); r.From != tt.from {
				t.Errorf("facts.dir source = %q, want %q", r.From, tt.from)
			}
		})
	}
}

func TestBlueprintSettingRefusals(t *testing.T) {
	tests := []struct {
		name    string
		layer   string
		content string
		want    string // after "<path>:<line>: "
	}{
		{"user layer", "user", "[blueprints]\nroot = \"b\"\n", "blueprints is not allowed here; move it to drafter.toml or drafter.local.toml"},
		{"absolute root", "project", "[blueprints]\nroot = \"/tmp/b\"\n", "blueprints.root must be a path inside the project, relative to its root, not \"/tmp/b\""},
		{"outline climbs out", "project", "[blueprints]\noutline = \"../o.yaml\"\n", "blueprints.outline must be a path inside the project, relative to its root, not \"../o.yaml\""},
		{"unknown key", "project", "[blueprints]\nfolder = \"b\"\n", "unknown key blueprints.folder"},
		{"brief path without {name}", "project", "[blueprints]\nbrief_path = \"notes/{dir}/brief.json\"\n", "blueprints.brief_path must contain {name}, so each page has its own brief, not \"notes/{dir}/brief.json\""},
		{"brief path climbs out", "project", "[blueprints]\nbrief_path = \"../notes/{name}.json\"\n", "blueprints.brief_path must be a path inside the project, relative to its root, not \"../notes/{name}.json\""},
		{"absolute brief path", "project", "[blueprints]\nbrief_path = \"/notes/{name}.json\"\n", "blueprints.brief_path must be a path inside the project, relative to its root, not \"/notes/{name}.json\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			writeFile(t, sb.project(), "mode = \"batch\"\n")
			writeFile(t, sb.layerFile(tt.layer), tt.content)
			_, err := Load(sb.root, Flags{})
			if err == nil || !strings.HasSuffix(err.Error(), tt.want) {
				t.Errorf("error %v, want suffix %q", err, tt.want)
			}
		})
	}
}

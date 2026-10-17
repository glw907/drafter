package config

import (
	"os"
	"path"
	"path/filepath"
)

// applyBlueprintDefaults fills the two defaults that depend on other settings: the outline sits
// in the blueprint root, and the facts store sits in a facts directory there when no layer names
// one. The facts default needs the directory to exist, so a project that keeps no facts under
// the blueprint root still has no facts store, and the consistency seat stays off.
func (c *Config) applyBlueprintDefaults(root string) {
	if _, set := c.src["blueprints.outline"]; !set {
		c.Blueprints.Outline = path.Join(c.Blueprints.Root, "outline.yaml")
		c.record("blueprints.outline", "default", c.Blueprints.Outline)
	}
	if root == "" || c.Facts.Dir != "" {
		return
	}
	dir := filepath.Join(root, filepath.FromSlash(c.Blueprints.Root), "facts")
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		c.Facts.Dir = dir
		c.record("facts.dir", "default", dir)
	}
}

package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Load reads the user layer, then, when root holds a drafter.toml, it and drafter.local.toml
// under it, then the environment and flags, and returns the resolved configuration. Callers
// pass the git top level, or empty outside a git tree; a root with no drafter.toml is outside a
// project and leaves Config.Root empty. Load checks what each layer may hold and what each
// value may be; Validate checks that the settings a command needs are present.
func Load(root string, flags Flags) (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	userDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user config directory: %w", err)
	}
	c := newConfig(home)

	var layers []*layer
	user, err := readLayer("user", inUser, filepath.Join(userDir, "drafter", "config.toml"), home)
	if err != nil {
		return nil, err
	}
	layers = append(layers, user)
	if root != "" {
		project, local, err := readProject(root, home)
		if err != nil {
			return nil, err
		}
		if project != nil {
			c.Root = filepath.Clean(root)
		}
		layers = append(layers, project, local)
	}
	env, err := fromEnv(home)
	if err != nil {
		return nil, err
	}
	layers = append(layers, env, flagLayer(flags))

	var laneFrom *layer
	for _, l := range layers {
		if l == nil {
			continue
		}
		if err := l.checkValues(); err != nil {
			return nil, err
		}
		if l.bit == inProject {
			if err := l.checkProjectPaths(c.Root); err != nil {
				return nil, err
			}
		}
		if l.f.Gate != nil && l.f.Gate.Lane != nil {
			laneFrom = l
		}
		c.apply(l)
	}
	if c.Gate.Lane != "" && c.Gate.Protocol != "cairn-run-gate" {
		return nil, laneFrom.fail("gate.lane", "needs gate.protocol cairn-run-gate, but the resolved protocol is %q",
			c.Gate.Protocol)
	}
	return c, nil
}

// readProject reads drafter.toml and drafter.local.toml under root. Either result is nil when
// its file is absent. A drafter.local.toml without a drafter.toml, or one git does not ignore,
// is refused.
func readProject(root, home string) (project, local *layer, err error) {
	localPath := filepath.Join(root, "drafter.local.toml")
	_, statErr := os.Stat(localPath)
	hasLocal := statErr == nil

	project, err = readLayer("drafter.toml", inProject, filepath.Join(root, "drafter.toml"), home)
	if err != nil {
		return nil, nil, err
	}
	if project == nil {
		if hasLocal {
			return nil, nil, fmt.Errorf("%s: drafter.local.toml needs a drafter.toml beside it; "+
				"%s is not a drafter project", localPath, root)
		}
		return nil, nil, nil
	}
	if !hasLocal {
		return project, nil, nil
	}
	if err := checkIgnored(root, localPath); err != nil {
		return nil, nil, err
	}
	local, err = readLayer("drafter.local.toml", inLocal, localPath, home)
	if err != nil {
		return nil, nil, err
	}
	return project, local, nil
}

// checkIgnored refuses a drafter.local.toml that git does not report as ignored, because the
// file can hold a key source and caps. A tracked file is not ignored even when .gitignore lists
// it, so a clone that commits both the file and the ignore line is refused too.
func checkIgnored(root, path string) error {
	cmd := exec.Command("git", "check-ignore", "-q", "--", filepath.Base(path))
	cmd.Dir = root
	err := cmd.Run()
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[*exec.ExitError](err); ok {
		return fmt.Errorf("%s: not ignored by git; add drafter.local.toml to .gitignore, "+
			"because it can hold a key source and caps", path)
	}
	return fmt.Errorf("check that %s is ignored: %w", path, err)
}

func flagLayer(flags Flags) *layer {
	l := &layer{label: "flag", f: &file{}}
	if flags.Sync {
		mode := "sync"
		l.f.Mode = &mode
	}
	if flags.JSON {
		output := "json"
		l.f.Output = &output
	}
	return l
}

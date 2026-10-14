package e2e

import (
	"strings"
	"testing"
)

func TestConfigAndSpendGitCannotRun(t *testing.T) {
	for _, sub := range []string{"config", "spend"} {
		t.Run(sub, func(t *testing.T) {
			p := newCfgProject(t)
			p.user(t, completeUser)
			env := append(gitEnv(), "HOME="+p.xdg, "XDG_CONFIG_HOME="+p.xdg, "PATH="+t.TempDir())
			r := runEnv(t, p.dir, env, sub)
			if r.code != 1 || !strings.Contains(r.stderr, "git could not run") || r.stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q; want exit 1 and stderr naming git could not run",
					r.code, r.stdout, r.stderr)
			}
		})
	}
}

func TestSpendOutsideGitTree(t *testing.T) {
	p := newSpendProject(t, 1)
	r := p.spend(t)
	if r.code != 0 {
		t.Errorf("exit %d, stderr %q; want the user layer to serve outside a git tree", r.code, r.stderr)
	}
}

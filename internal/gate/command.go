// Package gate runs a project's gate command and attributes its failures to pages.
package gate

import (
	"os"
	"strings"

	"github.com/glw907/drafter/internal/config"
)

// Command substitutes the {page}, {brief}, {slug}, and {baseline} placeholders in tmpl. A
// placeholder with a $ before its brace is shell syntax and stays. Each value is single-quoted for
// POSIX sh, so a space or a quote in a path cannot split or end the argument.
func Command(tmpl, page, brief, slug, baseline string) string {
	values := map[string]string{"page": page, "brief": brief, "slug": slug, "baseline": baseline}
	var b strings.Builder
	for i := 0; i < len(tmpl); i++ {
		if tmpl[i] == '{' && (i == 0 || tmpl[i-1] != '$') {
			if end := strings.IndexByte(tmpl[i:], '}'); end > 0 {
				if v, ok := values[tmpl[i+1:i+end]]; ok {
					b.WriteString(quote(v))
					i += end
					continue
				}
			}
		}
		b.WriteByte(tmpl[i])
	}
	return b.String()
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Env returns the process environment without the variable c.KeyEnv names, any ANTHROPIC_*
// variable, and RUN_GATE_IF_BUSY. Every command drafter runs gets it, so no key reaches a child
// process, its log, or a prompt.
func Env(c *config.Config) []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name == c.KeyEnv || name == "RUN_GATE_IF_BUSY" || strings.HasPrefix(name, "ANTHROPIC_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

# BACKLOG

> Project issue tracker. Managed by `/log-issue`.

## High

- [ ] **#1** Scope the blueprint read by item, or drop findings already present at the base `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  The scoped read blocks per section, so a tweak anywhere in a section meets that section's older findings. In the B2 format test, 0 of 5 tweaks passed and every miss but two carried only findings on untouched items. The same defects also change severity between reads. Evidence is in the B2 PR and on #43.

## Medium

- [ ] **#2** Let a headless agent read the skill's format page from a user-scope install `#bug` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  With `~/.claude/skills/blueprint` linked, a `claude -p --permission-mode acceptEdits` session was denied a Read of `format.md` until the skill directory was added with `--add-dir`. Decide whether the skill ships its format reference as a real file or the nightly agent gets the grant. `claude/.claude/skills/blueprint/format.md`

- [ ] **#3** Have the skill send an agent to `drafter refs` before it corrects a fact `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  In the format test's fact-correction tweak the agent searched with grep and never ran `drafter refs`. `claude/.claude/skills/blueprint/SKILL.md`

- [ ] **#4** Teach the skill when a caution is a failure row with a handler `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  The caution tweak added a `sentence` after the step it qualifies; the read wanted a `failure` item with a handler. `claude/.claude/skills/blueprint/SKILL.md`

## Low

- [ ] **#5** Read `blueprints.root` from `drafter.toml` in the page hook `#improvement` `#drafter` *(2026-10-18)*
  The hook checks only the conventional `blueprints/` root. `bin/.local/bin/drafter-page-hook`

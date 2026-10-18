# BACKLOG

> Project issue tracker. Managed by `/log-issue`.

## High

- [ ] **#1** Scope the blueprint read by item, or drop findings already present at the base `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  The scoped read blocks per section, so a tweak anywhere in a section meets that section's older findings. In the B2 format test, 0 of 5 tweaks passed and every miss but two carried only findings on untouched items. The same defects also change severity between reads. Evidence is in the B2 PR and on #43.
  The change-read pass (2026-10-18) fixed the cause: the rerun scored 1 of 5, and no blocking finding sat on an untouched item. The four misses were blocking findings on the tweak's own added content, so the bar now measures the skill's output. Left open for Geoff's ruling on what comes next.
  The skill-rules pass (2026-10-18) measured the rate with "Check what you add" in the skill: 12 of 21 first-try accepts against the bar of 17. Prompts 6 and 7 passed 6 of 6, prompts 1 to 3 passed 2 of 3 each, and prompts 4 and 5 passed 0 of 3. Still open.

## Medium

- [ ] **#6** Stop an unplaceable consistency answer from blocking a change read `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  A base conflict between two recurring facts escalates whenever a tweak re-cites one of them (the rerun's tweak 5). `internal/pipeline/blueprint.go`
  The first case, a conflict with empty `facts` and `with`, closed in the skill-rules pass: a change read records it as an "Unplaced:" warning and never blocks. Evidence is the unplaced rows of `TestChangeReadConsistency` and format-test runs 20261018T223440Z-9f9c8f61 and 20261018T224305Z-769284ec, each of which recorded an "Unplaced:" warning. `--json`'s `demoted` counts unplaced and already-present conflicts together, so only `record.json` tells them apart.

- [ ] **#7** Record the demoted conflicts and "Already present:" findings beyond a pruned run directory `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  A change-read accept keeps them only in the run's record, and B3 may let a scoped accept write the lock. They need a durable home first. `internal/pipeline/blueprint.go`

- [ ] **#8** Teach the skill the two shapes the rate left at 0 of 3: a failure with no way out and a section that interrupts the steps `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  In the skill-rules format test, prompt 4 (a caution) and prompt 5 (a new section) missed every run. Prompt 4's blocking findings were a handler that loops back to the failing step and a case added to one section with no row in the other; prompt 5's were a teaser heading, an explanation placed between the push and the checks, and a claim wider than the fact. The agents that read "Check what you add" missed these as often as those that did not (12 of 21 sessions read it). Two lines of the section also dropped words from the design: the heading line lost "never a teaser", and the example line lost "cites that fact". Evidence is in the pass's PR. `claude/.claude/skills/blueprint/SKILL.md`, `docs/blueprint-format.md`

## Low

- [ ] **#5** Read `blueprints.root` from `drafter.toml` in the page hook `#improvement` `#drafter` *(2026-10-18)*
  The hook checks only the conventional `blueprints/` root. `bin/.local/bin/drafter-page-hook`

## Done

- [x] **#4** Teach the skill when a caution is a failure row with a handler `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18 → 2026-10-18)*
  The skill now sends a caution to the "Add a failure case" row. In the rerun, tweak 4 added a `failure` item with `handledBy`. `claude/.claude/skills/blueprint/SKILL.md`

- [x] **#3** Have the skill send an agent to `drafter refs` before it corrects a fact `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18 → 2026-10-18)*
  Case 2 now carries the step. In the rerun, tweak 2 ran `drafter refs` before its first edit. `claude/.claude/skills/blueprint/SKILL.md`

- [x] **#2** Let a headless agent read the skill's format page from a user-scope install `#bug` `#drafter` `#initiative:docs-system` *(2026-10-18 → 2026-10-18)*
  Closed with no change. The design's 2026-10-18 probe showed that a headless session is denied a read of a real file inside a user-scope skill directory, so the denial comes from the harness. Live agents run interactively, and the format test grants the directory.

# BACKLOG

> Project issue tracker. Managed by `/log-issue`.

## High

- [ ] **#1** Scope the blueprint read by item, or drop findings already present at the base `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  The scoped read blocks per section, so a tweak anywhere in a section meets that section's older findings. In the B2 format test, 0 of 5 tweaks passed and every miss but two carried only findings on untouched items. The same defects also change severity between reads. Evidence is in the B2 PR and on #43.
  The change-read pass (2026-10-18) fixed the cause: the rerun scored 1 of 5, and no blocking finding sat on an untouched item. The four misses were blocking findings on the tweak's own added content, so the bar now measures the skill's output. Left open for Geoff's ruling on what comes next.
  The skill-rules pass (2026-10-18) measured the rate with "Check what you add" in the skill: 12 of 21 first-try accepts against the bar of 17. Prompts 6 and 7 passed 6 of 6, prompts 1 to 3 passed 2 of 3 each, and prompts 4 and 5 passed 0 of 3. Still open.
  The edit-quality pass (2026-10-18) added the read loop, the "Nit:" bar, and a fixed base, and reran the 21: `drafter check` green on the first commit 21 of 21, first-try acceptance 13 of 21, accepted within two reads 15 of 21 (bar 19), within three 17 of 21 (bar 20), one commit carrying a change no read saw (bar 0). Both controls bit and both positive controls accepted. The four misses were prompt 4 stopping at two reads with a block open, prompt 5 stopping on a consistency escalation twice, and an "Already present:" label on content the change introduced. Still open; Geoff rules on what comes next.

## Medium

- [ ] **#13** Give a whole read a stable bar on a base that reads clean `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  Four whole reads of the edit-quality test base each blocked on a different finding after the previous one was fixed, so a base can't be driven to an accept. The change read is unaffected because it labels base defects "Already present:". `claude/.claude/workflows/docs-page-chain.js`

- [ ] **#6** Stop an unplaceable consistency answer from blocking a change read `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  A base conflict between two recurring facts escalates whenever a tweak re-cites one of them (the rerun's tweak 5). `internal/pipeline/blueprint.go`
  The first case, a conflict with empty `facts` and `with`, closed in the skill-rules pass: a change read records it as an "Unplaced:" warning and never blocks. Evidence is the unplaced rows of `TestChangeReadConsistency` and format-test runs 20261018T223440Z-9f9c8f61 and 20261018T224305Z-769284ec, each of which recorded an "Unplaced:" warning. `--json`'s `demoted` counts unplaced and already-present conflicts together, so only `record.json` tells them apart.
  The base-pairing case closed with #12: a conflict between two store facts the base already carried no longer blocks when the tweak only re-cites one of them.

- [ ] **#7** Record the demoted conflicts and "Already present:" findings beyond a pruned run directory `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  A change-read accept keeps them only in the run's record, and B3 may let a scoped accept write the lock. They need a durable home first. `internal/pipeline/blueprint.go`

- [ ] **#8** Teach the skill the two shapes the rate left at 0 of 3: a failure with no way out and a section that interrupts the steps `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  In the skill-rules format test, prompt 4 (a caution) and prompt 5 (a new section) missed every run. Prompt 4's blocking findings were a handler that loops back to the failing step and a case added to one section with no row in the other; prompt 5's were a teaser heading, an explanation placed between the push and the checks, and a claim wider than the fact. The agents that read "Check what you add" missed these as often as those that did not (12 of 21 sessions read it). Two lines of the section also dropped words from the design: the heading line lost "never a teaser", and the example line lost "cites that fact". Evidence is in the pass's PR. `claude/.claude/skills/blueprint/SKILL.md`, `docs/blueprint-format.md`
  The edit-quality pass (2026-10-18) sent a caution with no fact-supplied handler to the "Add a caveat" row and made the smaller edit on a conflicting request. Prompt 4 then made the caveat in 3 of 3 runs and 2 of 3 were accepted; prompt 5 passed 1 of 3. The prompt 5 misses were consistency escalations (#12), so the shape that remains is a new section placed between the push and the checks, which the read now returns as a Nit. Left open for prompt 5.

## Low

- [ ] **#14** Make `drafter read`'s error hint print a resume command drafter accepts `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  The stopped-run line now carries `--sync` and the blueprint, but the error message `read.go` builds still says `drafter read --resume <id>`, which a sync run refuses. `cmd/drafter/read.go`

- [ ] **#9** Resolve every name on a multi-name Go declaration line in a fact pointer `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  A grouped `const` or `var` line such as `A, B = 1, 2` resolves neither name, so a pointer to either gets a false "no declaration" error. `internal/plan/verify.go`

- [ ] **#10** Keep an existing `.vale/styles/.gitignore` when `drafter init` runs `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  `init` always writes it, so a user's own ignore lines are replaced without a word. It could append `/Google/` when the file exists, or report the replacement. `internal/scaffold/scaffold.go`

- [ ] **#11** Make `drafter gate`'s totals line count Vale's result `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  When only Vale fails, the gate exits 1 under a "0 errors, 0 warnings" line on stderr. `cmd/drafter/gate.go`

- [ ] **#5** Read `blueprints.root` from `drafter.toml` in the page hook `#improvement` `#drafter` *(2026-10-18)*
  The hook checks only the conventional `blueprints/` root. `bin/.local/bin/drafter-page-hook`

- [ ] **#15** Label a resumed run's missing-seat error from the run copy `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-19)*
  On a resume, `loadModules` names the module and picks the stock hint from the current config, not the run's copy, so a stock run resumed after `prompts` is set reads `other.js` with no stock hint. The label is error text only. It could read `state.Inputs["prompts"].Source`. `internal/pipeline/start.go`

- [ ] **#16** Skip fenced code when reading a register's `## The page anatomies` entries `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-19)*
  A heading or bold-lead bullet inside a code fence under that section counts as an anatomy entry. Low risk. `internal/blueprint/tracks.go`

- [ ] **#17** Make the stock read's anatomy slug match drafter's slugger for non-ASCII names `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-19)*
  `slug` in `stock/prompts.js` handles only ASCII names, so a non-ASCII anatomy name with no register builds a `drafter:anatomies.md#` token that may not resolve. Every stock name is ASCII today. `stock/prompts.js`

## Done

- [x] **#12** Stop a base-only fact conflict from escalating a change read `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18 → 2026-10-18)*
  A change read no longer blocks on a conflict between store facts that the base already carried. `Scope` gains `Bullets` (facts whose bullet changed) and `Joined` (facts the change brought onto the page), and a conflict with a fact in neither comes back as "Already present:". A conflict whose bullet changed, or whose fact the change newly placed or cited, still blocks, and a scoped state saved before the fields existed keeps the old rule. Replay, offline against the four conflicts the edit-quality rerun saved: every one has both sides cited at the base, and `Bullets` and `Joined` came back empty for all four, so the rule demotes each to "Already present:" where the old rule blocked on the re-cited side. `internal/pipeline/blueprint.go`, `internal/blueprint/since.go`

- [x] **#4** Teach the skill when a caution is a failure row with a handler `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18 → 2026-10-18)*
  The skill now sends a caution to the "Add a failure case" row. In the rerun, tweak 4 added a `failure` item with `handledBy`. `claude/.claude/skills/blueprint/SKILL.md`

- [x] **#3** Have the skill send an agent to `drafter refs` before it corrects a fact `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18 → 2026-10-18)*
  Case 2 now carries the step. In the rerun, tweak 2 ran `drafter refs` before its first edit. `claude/.claude/skills/blueprint/SKILL.md`

- [x] **#2** Let a headless agent read the skill's format page from a user-scope install `#bug` `#drafter` `#initiative:docs-system` *(2026-10-18 → 2026-10-18)*
  Closed with no change. The design's 2026-10-18 probe showed that a headless session is denied a read of a real file inside a user-scope skill directory, so the denial comes from the harness. Live agents run interactively, and the format test grants the directory.

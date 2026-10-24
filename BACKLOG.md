# BACKLOG

> Project issue tracker. Managed by `/log-issue`.

## High

- [x] **#81** Show a running batch's status: print it while polling, and add `drafter status <run-id>` `#feature` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  On 2026-10-24, run `20261024T080412Z-13b5d021` sat six hours with two redraft batches "in progress", and nothing on the workstation could tell a queued batch from a wedged process or a missed result. The run directory doesn't change while polling, and `drafter page` prints only at the end. Finding out took Geoff reading the Console's Batches view for the drafter workspace. Record each poll's `processing_status` and `request_counts` in the run state, print a line when a status changes, and add `drafter status <run-id>`. That command should query the API with the configured key and report each open batch's status, age, and request counts, plus any billing or rate-limit error. A session can then check a run without handling the key.
- [x] **#82** Don't make a fast page wait for the slowest page at each stage `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #62)*
  In the same run, Content model's first draft returned in 10 minutes and failed its gate, but its redraft waited 79 minutes, until Arrange the admin sidebar's first batch returned, because both redrafts went out together at 01:33. Advance each page through its stages on its own, or submit a page's next request as soon as its own result is handled.
- [x] **#71** Make the two fact parsers agree on a tag note's code spans `#bug` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  A code span inside a `[verified: ...]` note passed `drafter check --fact` but failed cairn-cms `check:facts` as an unresolved path. The grammar should say whether a tag note's code spans count as pointers; cairn-cms #38 is the other half. Record B3-13.

- [ ] **#1** Scope the blueprint read by item, or drop findings already present at the base `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18)*
  The scoped read blocks per section, so a tweak anywhere in a section meets that section's older findings. In the B2 format test, 0 of 5 tweaks passed and every miss but two carried only findings on untouched items. The same defects also change severity between reads. Evidence is in the B2 PR and on #43.
  The change-read pass (2026-10-18) fixed the cause: the rerun scored 1 of 5, and no blocking finding sat on an untouched item. The four misses were blocking findings on the tweak's own added content, so the bar now measures the skill's output. Left open for Geoff's ruling on what comes next.
  The skill-rules pass (2026-10-18) measured the rate with "Check what you add" in the skill: 12 of 21 first-try accepts against the bar of 17. Prompts 6 and 7 passed 6 of 6, prompts 1 to 3 passed 2 of 3 each, and prompts 4 and 5 passed 0 of 3. Still open.
  The edit-quality pass (2026-10-18) added the read loop, the "Nit:" bar, and a fixed base, and reran the 21: `drafter check` green on the first commit 21 of 21, first-try acceptance 13 of 21, accepted within two reads 15 of 21 (bar 19), within three 17 of 21 (bar 20), one commit carrying a change no read saw (bar 0). Both controls bit and both positive controls accepted. The four misses were prompt 4 stopping at two reads with a block open, prompt 5 stopping on a consistency escalation twice, and an "Already present:" label on content the change introduced. Still open; Geoff rules on what comes next.

## Medium

- [ ] **#83** Don't let one untracked batch hold every page after a create with an unknown outcome `#bug` `#drafter` `#initiative:docs-system` *(2026-10-24)*
  After a create whose outcome is unknown, `recover` → `findBatch` → `waitEnded` polls each untracked candidate batch until it ends, and no page advances meanwhile. A candidate that never ends, or ends late, holds the whole run. `internal/pipeline/money.go`. Found in the stage-advance pass (PR #62).
- [x] **#80** Blueprint skill: mark which `drafter check` rows apply to markdown, and allow an in-place fact edit `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  format.md's "What `drafter check` checks" table is mostly YAML-era rows (`why`, `use`, `framing`, `recovery`, `reader-state`); say which apply to a markdown blueprint. The fact-edit rule should allow an exact in-place substitution with the Edit tool, never an append. Record B3-29 in `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`.
- [x] **#79** Drift-check line pointers that carry no anchor span `#bug` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  A `path:lines` pointer with no anchor is never drift-checked: cairn-cms facts f:ivhuyc, f:gh4ckg, f:2p519o, and f:qd7iat had drifted 1 to 30 lines and still passed. Require an anchor on line pointers, or check the span's text. Facts about a dependency's behavior (gray-matter, js-yaml) have no excerptable source and rest on dated runs; a `run` or vendor tag would tell a checker why. Record B3-28.
- [x] **#78** `drafter refs`: count a published page's brief citations before the page has a blueprint `#bug` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  Before a page has a blueprint, `drafter refs <fact>` says "no blueprint places or cites it", so a fact correction looks free while the page's brief cites the fact five or more times. It also labels ordinary bullets `(example)`, a kind format.md never names. Record B3-27.
- [x] **#77** Blueprint skill: say what to do with neighbors' `outOfScope` lines when a page's `covers` narrows `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  Narrowing a page's `covers` leaves neighbors' `outOfScope` lines routing to it (outline.yaml 105, 127, 393); the brief should say whether to reword them. `claude/.claude/skills/blueprint/new-page.md`. Record B3-9.

- [x] **#76** Blueprint skill: add a recipe for a generated example `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  An example that is a render's HTML output has no recipe: the author ran `createRenderer` by hand, and a fact-checker has no `:source` to re-verify it from. `claude/.claude/skills/blueprint/`. Record B3-6.

- [x] **#75** Blueprint skill: fix two broken links and say whether a `:fact` tag on a `Code:` bullet counts `#bug` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  `introductions.md` links drafter's GitHub `blueprint-format.md`, not the local `format.md`; `anatomies.md` links `../docs/blueprint-format.md`, which doesn't exist from the skill directory; and `format.md` doesn't say whether a `:fact` tag on a `Code:` bullet counts. `claude/.claude/skills/blueprint/`. Record B3-5, B3-15.

- [x] **#74** Count a `See` link bullet that has trailing text or an `#anchor` `#bug` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  A `See [text](target)` bullet with trailing text, or with a `#anchor` link and trailing text, is not counted as a link item, so the cross-link check warns; `format.md` never says trailing text disqualifies the bullet. Say so or count it. Seen on configure-media. Record B3-19.

- [x] **#73** Show a `drafter page` run's wait on a batch, and offer a non-batch mode `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, superseded by #81, PR #60)*
  `drafter page` sends its seats through the Message Batches API. The sidebar's draft batch, reserved 08:04Z, was still pending an hour later, and the state file and log showed nothing, so the session read a healthy wait as a stall. Print "waiting on batch <id> since <time>" and offer an attended, non-batch mode. Record B3-17.

- [x] **#72** Excerpt a fact pointer into `node_modules` `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  A pointer into `node_modules` gives "no excerpt: untracked", so a vendor's thrown string can't be shown to a reader of the fact. Record B3-14.

- [x] **#70** Report the line and column of a YAML error in `outline.yaml` `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  A YAML error in `outline.yaml` reports only "malformed", with no line or column. Record B3-4.

- [x] **#69** Load `~/.config/drafter/<repo>.local.toml` as the fallback config layer `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  A fresh worktree has no `drafter.local.toml`, so `drafter page` exits 2 with "prompts is not set, and require_prompts in drafter.toml refuses the stock module", though the template sits in `~/.config/drafter`. Load it as a fallback layer, or name the template in the error. `internal/config/load.go:85` (record 60).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`

- [x] **#68** List the plan's charter- and doc-sourced fact ids for the D1 audit `#feature` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  No tool says which ids a plan cites are charter- or doc-sourced, so the D1 audit (drop charter-sourced claims) runs by hand on every page; two authors did it (record 35).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`

- [x] **#67** Read the briefs of pages with no blueprint in `drafter refs` `#bug` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  `drafter refs` reports "no blueprint places or cites" `f:ld6xam` though the page's existing brief cites it, because it reads briefs only for pages that have a blueprint. Pilot items 10 and 30 are half fixed (record 34).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`

- [x] **#66** List inbound blueprint links when an outline removal retires a page `#feature` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  Retiring a page leaves inbound blueprint links (the add-cairn tutorial, replace-magic-links' See also), and no tool lists them on an outline removal; `drafter refs` is fact-based (record 33).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`

- [x] **#65** Fix two links-check misreads: a `managed: false` crossLink and a cross-page anchor `#bug` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  The links warning flags an outline `crossLink` to a `managed: false` page as "names no page", and a cross-page anchor resolves against the published page, not the target's blueprint, so a link to a section a blueprint adds errors until the page is redrafted (record 31, 32).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`

- [x] **#64** Check sentence length and hinges on blueprint lines `#feature` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  `drafter check` has no sentence-length or hinge check on blueprint lines. The register editor found 11 task sentences over 26 words in the sidebar blueprint, and the fold author wrote a throwaway script for it (record 30).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`

- [x] **#63** Keep outline warnings out of a check run by blueprint name `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  A check by name still prints `outline.yaml` `types` warnings (the tutorial milestone, the symptom row) that belong to no named blueprint; two authors hit it (record 29).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`
  Seen again in batch 3: a named `drafter check` still prints the outline's unrelated `types` warnings, and outline-wide warnings (anatomy names, a crossLink to `docs/reference/sveltekit.md` "names no page the outline lists"). Record B3-3, B3-21.

- [x] **#62** Report each exhaustive-word match at the word's line, and every match on a line `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  A warning inside `:::row` points at the `:::row` opening line, not the word's line, and the lint reports one match per line, which forces a fix, rerun, fix loop (record 27, 28).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`

- [x] **#61** Let `:source[...]` hold a path with brackets `#bug` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  The parse stops at the first `]` and escaping doesn't help, so `preview/[token]` and `[...path]` can't be a source. Two authors dropped `:source` from their `Code:` bullets (record 23). `internal/blueprint/markdown.go`
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`
  Seen again in batch 3: configure-rendering's author hit it on a SvelteKit route path. Record B3-1.

- [x] **#60** Close the store check's gaps: doc-comment anchors, dotted pointers, route paths, figure sources `#bug` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  The `#name` check reads the anchor against the declaration without its doc comment, though `format.md` says the excerpt includes it; it skips dotted `#a.b` pointers (`f:8zdux6` is unverified) and bracket or parenthesis route paths that `drafter excerpt` resolves; and a figure's `:source` gets no existence check (record 21, 22, 24, 25).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`
  Seen again in batch 3: `#Outer.inner` pointers are still skipped by verification. Record B3-22.

- [x] **#59** Require an anchor or content hash on every line pointer `#bug` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  An anchorless `path:N` or `path:A-B` pointer never detects drift: `f:ys1iq1` cited `types.ts:418` for a symbol at 430 and the preview blueprint had 11 stale ranges of which the check warned on 1. The excerpt's window also hides drift (`f:hdrzxd` moved 181 to 189 and passed), and an anchor in parentheses still isn't read (record 19, 20). Pilot 5 stays open.
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`
  Seen again in batch 3: define-an-adapter's author found six facts whose `path:N-M` pointers had drifted off their lines and still passed `drafter check --fact` (`f:aj5c4z`, `f:sz71ur`, `f:7ak73t`, `f:9ro4u5`, `f:n07oiu`, `f:hkh101`), and the +-10-line window still passes `f:c8c9ji` (78-84 against 80-88). An anchorless line range should warn. Record B3-12, B3-22.

- [x] **#58** Blueprint skill: tell the author how to resolve an exemplar, mint ids, edit in place, and find facts `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  The skill never says how an exemplar name resolves (`~/.config/drafter/cairn-cms.local.toml`, `inline_files`), names the mint command (one author hand-picked ids instead of `check-facts.mjs --mint`), says whether `sed` pointer swaps break "Edit tool only", or says to find facts by grepping each id the plan cites, not by heading. `claude/.claude/skills/blueprint/new-page.md` (record 10, 15, 16, 17).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`

- [x] **#57** Blueprint skill: pick `**Background.**` or `:::constraints` for the introduction `#bug` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  `introductions.md` still describes a `**Background.**` paragraph while `new-page.md` says to use `:::constraints`. `claude/.claude/skills/blueprint/introductions.md` (record 9).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`
  Seen again in batch 3: introductions.md calls the reader block's Background paragraph a planning note, while new-page.md says page-wide instructions go in `:::constraints`; one sentence should say how they differ. Record B3-23.

- [x] **#56** Blueprint skill: close format.md's gaps on tables, code examples, `:::row`, and explanation bullets `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  `format.md` has no table form; shows `Code:` with a fence yet says "bare fence" (one fence per bullet, so a command and its output need two); doesn't say when `Code:` may omit `:source`; and is silent on `:::row` outside symptom pages and explanation bullets before the lead-in (`drafter check` accepts both). Add a sentence and an example for each. `claude/.claude/skills/blueprint/format.md` (record 8, 11, 12, 13, 14).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`
  Seen again in batch 3: no table form in format.md, and no guidance for a table in a markdown blueprint. Record B3-23.

- [x] **#55** Blueprint skill: hold the settle step's fix lines to the exhaustive-word rule `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  Both round-2 regressions in batch 2 were the session's fix lines, still too wide. Say a fix line meets the rule the authors follow, and that a finding that returns twice calls for a structural rewrite. `claude/.claude/skills/blueprint/review.md` (record 5).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`

- [x] **#54** Blueprint skill: trace an engine question through code and design history before asking Geoff `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  Four question notes on engine intent (address collision against the resolver throw, `inFeeds`, D1's reach over design specs, an unset `PUBLIC_ORIGIN`) went to Geoff and were all answerable from specs, history, or code. Add to the Review section: send Geoff only product, taste, and scope. `claude/.claude/skills/blueprint/` (record 4).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`

- [x] **#53** Blueprint skill: run the register editor before the fact-check and queueing `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  `docs-register.md` already lists the empty "own", yet it got through 10 times on the sidebar and 6 on the content model; only a fresh `cairn-register-editor` caught it, along with 11 task sentences over 26 words. Run it and apply its rewrites first. `claude/.claude/skills/blueprint/new-page.md` (record 3).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`

- [x] **#52** Blueprint skill: say what to do when a folded section's target page has no blueprint `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  "Adds a section, approve the blueprint first" has no path for a target with none: folding one section into build-the-public-routes became a whole-page blueprint with 13 covers. `claude/.claude/skills/blueprint/new-page.md` (record 2).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`

- [x] **#51** Blueprint skill: add a premise step before authoring a page `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-24 → 2026-10-24, PR #60)*
  Nobody asked whether share-a-draft-preview earned a page until Geoff did, after the full flow ran on it. Add a step before step 1: who has work here that the scaffold, tutorial, or a reference page doesn't cover? An inherited outline entry is no evidence; when the case is weak, write a fold recommendation. `claude/.claude/skills/blueprint/new-page.md` (record 1).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-batch-2-friction.md`
- [x] **#48** Narrow the `exhaustive` lint's `no` and `never` `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-22 → 2026-10-24, PR #60)*
  On cairn's 585-line custom admin screen blueprint the lint gives about 70 warnings, and `no` and `never` are most of them: plain negation ("has no default", "makes no network call"), not a claim of a complete set. The whole-branch review dropped the digit counts and the verified fact bullets; whether to drop `no` and `never` too is Geoff's call. `internal/plan/exhaustive.go`
  Batch 2: the lint gave 33 to 39 warnings per page, nearly all false: "no" before a code span or as a determiner, counts, "never" in `:::constraints`, and bold UI labels. Exempt those forms and add a per-line settled marker (record 26).
  Seen again in batch 3: the lint gave 27 warnings on configure-rendering and 37 on configure-media, nearly all `:::constraints` lines (instructions, not claims), absence wording ("no `registry`"), and "no" as a determiner ("no manifest row"). Exempt the constraints block and tell absence phrasing from an exhaustive claim. Record B3-2, B3-20.
- [x] **#49** Give a `repro` figure block a markdown form `#feature` `#drafter` `#initiative:docs-system` *(2026-10-22 → 2026-10-24, PR #60)*
  A cairn page's figure is a `repro` fence, which names a story, alt text, and caption. In the markdown format a fence attaches only to a `Code:` bullet, so a `Figure:` bullet above a `repro` fence reads as a bare fence. The end-to-end check wrote the figure as a `Figure:` bullet with a `:source` and no fence. Decide whether a `Figure:` bullet takes a fence too. `internal/blueprint/markdown.go`
- [x] **#50** Warn on a second `:source` tag on one bullet `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-22 → 2026-10-24, PR #60)*
  `DecodeMarkdown` keeps the first `:source` of a `Code:` or `Figure:` bullet and drops any later one without a finding. `internal/blueprint/markdown.go`

- [ ] **#35** Read a code fence inside an open `:::row` or `:::reader` block as block text `#bug` `#drafter` `#initiative:docs-system` *(2026-10-21)*
  `DecodeMarkdown` checks for a fence before it checks for an open block, so a fence line inside a row or the reader block starts a fence, and the block's closing `:::` inside that fence is swallowed. The block can then be misread or never close. Found by the reviewed-pages CT1 recheck. `internal/blueprint/markdown.go:161-170`

- [ ] **#29** Refuse a resume on a binary other than the run's `drafter_version` `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-20)*
  A run started under one build resumes under another, so a stock change like the 64000-to-128000 draft ceiling can split one run across two configurations. `internal/pipeline/resume.go`

- [ ] **#30** Follow `--first-parent` in `LockIntroducedAt` across merge commits `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-20)*
  On a branch that merges task branches, the commit that introduced a lock entry can sit on a side parent, and the start-commit inference reads the wrong base. `blueprint/drafted.go:41-60`

- [ ] **#31** Show the consistency seat the linked page's text, not only its headings `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-20)*
  The consistency seat sees placed fact text and the headings of linked pages. A table under a linked page's title heading looked absent, and debug-your-site's read blocked on it in the acceptance pass. `internal/pipeline/blueprint.go`

- [ ] **#32** Keep every round-2 seat's small findings in the s6 record `#bug` `#drafter` `#initiative:docs-system` *(2026-10-20)*
  When an earlier round-2 seat's findings are all small and a later seat escalates, only the escalating seat's findings reach `record.json`, so the evidence drops the earlier ones. Control flow is right. `internal/pipeline/steps.go:646`

- [ ] **#27** Make `FactChanged` ignore a fact whose only change is a moved `Source:` path `#improvement` `#drafter` `#initiative:repo-organization` *(2026-10-20)*
  The docs-layout sweep moved `docs/superpowers/` paths, so fact f:swjwxb's `Source:` line changed while its claim did not. The next `drafter page` on cairn-cms's `docs/extend/debug-your-site.md` now counts the bullet as changed and redrafts the page whole instead of scoped. A rename-aware comparison would treat a pointer that resolves to the same content as unchanged. Reported by the docs-layout session. `internal/blueprint/since.go`

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

- [ ] **#34** Refuse a `:fact` tag in front matter and a malformed tag `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-21)*
  `page: Rotate keys :fact[bbbbbb]` decodes with the tag left in the title, and `:fact[]` or `:fact[a b]` does not match `factTag`, so each is ignored with no finding, although the format doc says a tag anywhere else is an error. The reviewed-pages tests also lack a case for a tag on a `:::reader` opener. Found by the reviewed-pages CT1 recheck. `internal/blueprint/markdown.go:93,129-147`

- [ ] **#18** Retry a reader read that stops at `max_tokens` `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-19)*
  The reader's schema lets the model run a string on past its end: in the B3 live runs about one reader call in four filled its 32000-token ceiling with repeated blank lines, quotes, or tags, and the page escalated as an unreadable reader at about $1.2. `noRetry` gives `stReader1` and `stReader2` no second try by design. The reader prompt now asks for plain text and the stock reader runs at low effort, which cut the rate in a replay but not to zero. A single retry of a `max_tokens` stop, or a lower `max_tokens` for the reader, needs Geoff's ruling because it changes the failures table. `internal/pipeline/handle.go`
  The acceptance pass (2026-10-20) hit it again: debug-your-site's s7 reader wrote a full paraphrase, then repeated a separator and "(fin)" to 32000 output tokens, and the page escalated "unreadable reader" (run 20261020T082130Z-33e822ff). A guard that cuts a degenerate repetition loop would have saved the page.

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

- [ ] **#19** Detect a redraft that rewrites the page to the same bytes `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-19)*
  A fact bullet changes, the redraft writes byte-identical text, and the lock entry comes out identical too, so the next run sees the same fact change and redrafts again. The lock's commit field (B4) closes it. `cmd/drafter/draftscope.go`

- [ ] **#20** Run the consistency read when the blueprint read is skipped and a cited fact changed `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-19)*
  When the lock's `accepted` equals the blueprint at the start commit, the one stage-1 batch is skipped, so a fact bullet changed since the last read drafts with no fresh consistency read. The same skip leaves a change to the outline entry alone with no change read, though the plan's table calls for one. `internal/pipeline/blueprint.go`

- [ ] **#21** Record the start commit in a lock entry `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-19)*
  The lock names no commit, so `drafter page` infers what the draft saw from the parent of the commit that introduced the entry, and a fact or outline edit committed alone between a run's start and its lock commit can hide from the next run. It warns now. A start-commit field replaces the walk (B4). `internal/blueprint/drafted.go`

- [ ] **#22** Let a stopped pre-B3 run resume across the new `coherence` seat `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-19)*
  The fingerprint hashes every configured seat, so a run stopped before B3 refuses to resume with "the configuration changed since the run started (seat.coherence.…)". Fixing it edits `fingerprint.go`, which needs the auth-data bar. Resume such a run with the old binary before installing the new one. `internal/pipeline/fingerprint.go`

- [ ] **#24** Keep a heading that another page links to, or show the links to the gate-fix step `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-19)*
  Twice in B3c a redraft dropped a heading another page linked to, and the run stopped "tree red outside drafter's pages". The pass worked around it with a pre-run sweep of inbound anchors against blueprint ids. The draft or gate-fix prompt could carry the inbound anchors instead.


## Done

- [x] **#43** Accept framing-heading variants in `convert plan` and write the seeded blob `#bug` `#drafter` `#initiative:docs-system` *(2026-10-22 → 2026-10-24, won't do)*
  Two of three v1 framing records failed on exact heading matches, and the lock it seeds names a blob absent from the object store, so `convert markdown` calls the page stale with the wrong remedy. `internal/blueprint/convert.go:162` (record 1, 2).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-pilot-friction.md`
  Closed as won't do by Geoff on 2026-10-24: every new page starts from a prose plan, and the legacy YAML blueprints convert by hand when next touched.

- [x] **#42** Make `convert markdown` emit a reviewable blueprint `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-22 → 2026-10-24, won't do)*
  Three of three pilot pages came out as skeletons (44 check errors, no job lines, duplicated fact text, glued reader block, lost code sources) and needed a full author rewrite (record 3).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-pilot-friction.md`
  Closed as won't do by Geoff on 2026-10-24: every new page starts from a prose plan, and the legacy YAML blueprints convert by hand when next touched.

- [x] **#36** Add a single-fact check, an outline query, and `--version` `#feature` `#drafter` `#initiative:docs-system` *(2026-10-22 → 2026-10-22)*
  No subcommand checks one fact's pointers, so its output mixes in the whole store's findings; there is no `drafter outline <slug>` and no `--version` (record 11, 12).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-pilot-friction.md`
  Landed in glw907/drafter#58: `drafter check --fact`, `drafter outline`, and `drafter --version`.

- [x] **#37** Make `drafter excerpt` serve non-code and long sources `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-22 → 2026-10-22)*
  A vendor fact exits 1 and prints only the bullet; print its URLs and read dates as a checklist. A charter-sourced fact's warning says "point it at the code"; name the drop rule or `facts.prose_sources`. Cap or summarize very long excerpts (`f:0gihxq`: 2,110 lines) (record 7-9).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-pilot-friction.md`
  Landed in glw907/drafter#58: `drafter excerpt` prints a URL-sourced fact as a checklist, says to verify a prose-sourced fact by hand, and stops at 200 lines unless `--full`.

- [x] **#38** Show what a fact correction will redraft and break `#feature` `#drafter` `#initiative:docs-system` *(2026-10-22 → 2026-10-22)*
  `drafter refs` lists blueprint lines only, one id per call. A correction to `f:39sn8c` broke a published page's sentence brief (`check:provenance`), seen only in the full static leg. Cover briefs and drafted pages, and take several ids (record 10, 30).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-pilot-friction.md`
  Landed in glw907/drafter#58: `drafter refs` takes several ids and lists the section, brief entry, and drafted page of each hit.

- [x] **#39** Flag pointer drift inside the excerpt window and parenthetical skips `#bug` `#drafter` `#initiative:docs-system` *(2026-10-22 → 2026-10-22)*
  Six drifted pointers passed the store check while their anchor stayed within ±10 lines; a pointer in parentheses is read as commentary with only "no block shows source lines" (record 5, 6).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-pilot-friction.md`
  Landed in glw907/drafter#58: an anchor off its cited lines is a warning naming the new line, and a parenthetical pointer is counted in the no-excerpt message.

- [x] **#40** Validate `track` when the outline declares no tracks `#bug` `#drafter` `#initiative:docs-system` *(2026-10-22 → 2026-10-22)*
  An empty or wrong `track` passes `drafter check` silently (record 4).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-pilot-friction.md`
  Landed in glw907/drafter#58: `track` is optional when the outline declares no tracks, and naming one is an error.

- [x] **#41** Lint blueprint lines and fact bullets for unqualified exhaustive words `#feature` `#drafter` `#initiative:docs-system` *(2026-10-22 → 2026-10-22)*
  Every imprecise fact-check finding in the pilot was a line or fact saying more than its source; "only", "every", "no", and counts were the returning cause across rounds. A lint would aim the fact-checker before it runs (record, The numbers).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-pilot-friction.md`
  Landed in glw907/drafter#58: the `exhaustive` warning on blueprint lines and fact bullets.

- [x] **#44** Blueprint skill: a rule for parallel authors sharing one facts store `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-22 → 2026-10-22)*
  `f:9xthnq` was flagged wrong by one author and corrected by another; two authors split on charter-sourced facts until Geoff ruled. Say who fixes a shared fact, and tell an author to run `npm ci` so `check:facts` can run (record 18; cairn-cms 26).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-pilot-friction.md`
  Landed in glw907/drafter#58: `new-page.md` says who fixes a shared fact and to run the facts check with its dependencies installed.

- [x] **#45** Blueprint skill: fix format.md's stale YAML wording, hinge example, and lead-in rule `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-22 → 2026-10-22)*
  format.md still calls a blueprint one YAML file; its job-line example uses the banned trailing hinge; the skill doesn't say whether a task section may open with a lead-in line (record 15-17).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-pilot-friction.md`
  Landed in glw907/drafter#58.

- [x] **#46** Blueprint skill: frame the reader from the outline's job, never the worked example `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-22 → 2026-10-22)*
  Geoff sent back the custom admin screen: the plan's signups example had narrowed the reader to screens over non-markdown data, though the outline's job was general. new-page.md should say to frame from the job (record, The numbers).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-pilot-friction.md`
  Landed in glw907/drafter#58: `new-page.md` and `introductions.md` frame the reader from the outline entry's job.

- [x] **#47** Blueprint format: give code examples, figures, and page constraints a markdown form `#feature` `#drafter` `#initiative:docs-system` *(2026-10-22 → 2026-10-22)*
  All three pilot authors improvised: a code item's source no longer reaches the draft seat (`CodeSources` returns nil for markdown), and figures and YAML `constraints`/`orderReason`/`argument` have no home (record 13, 14).
  Record: `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-pilot-friction.md`
  Landed in glw907/drafter#58: the `Code:` and `Figure:` bullets and the `:::constraints` block.

- [x] **#33** Give the fact-truth seat repository access, an agentic read `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-20 → 2026-10-21)*
  Closed, superseded by the reviewed-pages design (glw907/initiatives `docs-system/specs/2026-10-21-drafter-reviewed-pages-design.md`): a fresh-context session agent with repository access now fact-checks each blueprint and page, so no API seat needs an agentic read. The item was filed only on the parked `worktree-blueprint-first` branch, so it is recorded here to keep the number from being reused.

- [x] **#28** Resolve a dotted excerpt anchor to a nested declaration `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-20 → 2026-10-21)*
  A pointer like `file.ts#outer.inner` excerpts as "declaration not found", so the fact seat can't see the claim's source. The acceptance pass re-anchored 35 such cairn pointers to line ranges by hand (finding 8). Harvested from the parked `worktree-blueprint-first` branch in the reviewed-pages pass. `internal/assemble/excerpt.go`

- [x] **#26** Raise the stock `seat.draft.max_tokens` above 64000 `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-19 → 2026-10-20)*
  Two of cairn's longest task guides stopped at 64000 tokens of page, brief, and thinking; at 128000 the same drafts fit. `internal/config/stock.toml`

- [x] **#25** Let a scoped accept advance the lock, or skip the s1 re-read after one `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-19 → 2026-10-20)*
  A `--since` read that accepts writes nothing, so a later `page --whole` re-reads the blueprint whole at s1 and can block on findings the scoped read never weighed. B3c's rotate probe blocked that way. `cmd/drafter/read.go`

- [x] **#23** Hold a page whose gate red names only a sibling page of the same run `#bug` `#drafter` `#initiative:docs-system` *(2026-10-19 → 2026-10-20)*
  In B3c's wave 2, two pages went to a gate-fix draft for a `check:symbols` line in a third page of the run, spending a draft attempt each on a defect they could not fix. A red line that names another page in the run should hold the page as a foreign red does. `internal/pipeline/check.go`

- [x] **#13** Give a whole read a stable bar on a base that reads clean `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18 → 2026-10-19)*
  Four whole reads of the edit-quality test base each blocked on a different finding after the previous one was fixed, so a base can't be driven to an accept. The change read is unaffected because it labels base defects "Already present:". `claude/.claude/workflows/docs-page-chain.js`
  B3c landed the bar: a whole read that blocks records `blocked` in `drafted.lock`, and the next read is scoped to the change since that commit. Observed live on cairn's security-model page: the first read was whole and blocked, a fix followed, and a bar read accepted and cleared `blocked`. `cmd/drafter/bar.go`, `internal/pipeline/blueprint.go`

- [x] **#12** Stop a base-only fact conflict from escalating a change read `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18 → 2026-10-18)*
  A change read no longer blocks on a conflict between store facts that the base already carried. `Scope` gains `Bullets` (facts whose bullet changed) and `Joined` (facts the change brought onto the page), and a conflict with a fact in neither comes back as "Already present:". A conflict whose bullet changed, or whose fact the change newly placed or cited, still blocks, and a scoped state saved before the fields existed keeps the old rule. Replay, offline against the four conflicts the edit-quality rerun saved: every one has both sides cited at the base, and `Bullets` and `Joined` came back empty for all four, so the rule demotes each to "Already present:" where the old rule blocked on the re-cited side. `internal/pipeline/blueprint.go`, `internal/blueprint/since.go`

- [x] **#4** Teach the skill when a caution is a failure row with a handler `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18 → 2026-10-18)*
  The skill now sends a caution to the "Add a failure case" row. In the rerun, tweak 4 added a `failure` item with `handledBy`. `claude/.claude/skills/blueprint/SKILL.md`

- [x] **#3** Have the skill send an agent to `drafter refs` before it corrects a fact `#improvement` `#drafter` `#initiative:docs-system` *(2026-10-18 → 2026-10-18)*
  Case 2 now carries the step. In the rerun, tweak 2 ran `drafter refs` before its first edit. `claude/.claude/skills/blueprint/SKILL.md`

- [x] **#2** Let a headless agent read the skill's format page from a user-scope install `#bug` `#drafter` `#initiative:docs-system` *(2026-10-18 → 2026-10-18)*
  Closed with no change. The design's 2026-10-18 probe showed that a headless session is denied a read of a real file inside a user-scope skill directory, so the denial comes from the harness. Live agents run interactively, and the format test grants the directory.

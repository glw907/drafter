# Project status

Present tense only. Shipped history is in git and the PRs; strategy lives in the docs-system
initiative at `~/Projects/initiatives/docs-system/`.

## Current state

- `main` is at the merge of PR #60 (`15afb8d`), the batch-friction pass, run by executor
  `drafter-batch-friction` (`44a07fee`) under `conductor-drafter`. It lands backlog #48 to #81,
  less #42, #43, and #82.
  - `drafter check` warns on an anchorless line pointer, checks doc-comment anchors, dotted
    pointers, route paths, and figure sources, and drops `no` and `never` from the exhaustive
    lint. A named check prints only that blueprint's findings. `check.sentence_words` turns on a
    sentence-length warning.
  - `:source[...]` takes one level of brackets, and a `Figure:` bullet takes a fence.
  - `drafter refs` reads every brief, lists links into a page with `--page`, and names each
    fact's source kind.
  - `drafter page` prints a line when a batch's status changes, and `drafter status <run-id>`
    reports open batches.
  - A worktree with no `drafter.local.toml` reads `~/.config/drafter/<repo>.local.toml`.
  - The `blueprint` skill and `docs/blueprint-format.md` carry the batches' fixes.
- `~/.local/bin/drafter` is still the PR #58 build (`5680e8f`). Install waits on run
  `20261024T080412Z-13b5d021`: both its batches were `in_progress` at the merge.
- cairn-cms backlog #56 carries the follow-up settings for cairn (`check.sentence_words = 26` and
  two `managed: false` outline entries), and cairn #38 notes drafter's tag-note rule.
- The stage-advance pass (#82) is at PR-ready on `worktree-stage-advance`, run by executor
  `drafter-stage-advance` under `conductor-drafter`. In batch mode a page submits its next stage as
  soon as its own batch is handled, while siblings' batches stay in flight. The blueprint skill
  tells a session to name a findings path in the fact-checker dispatch. #42 and #43 are closed as
  won't do: every new page starts from a prose plan, and legacy YAML blueprints convert by hand.

## Next action

Merge the stage-advance PR on Geoff's approval, after the workstation pass's `~/.dotfiles` PR.
The plan is `~/Projects/initiatives/docs-system/plans/2026-10-24-drafter-stage-advance-plan.md`.

The next cairn docs pass's first multi-page `drafter page` is #82's live check. Read that run's
`state.json`: each page's `intents[].time` (submit) and `created`, and `poll.seen`. A page whose
next submit waited for a sibling's batch to end is an escape.

Once run `20261024T080412Z-13b5d021` has no open batch (`drafter status` with a build of `main`),
run `make install` on pavilion from the main checkout and check `drafter --version`. thinkpad-x1
picks the build up at its next pull and `make install`.

The last nine cairn extend blueprints start from the merged skill.

## Open decisions

None.

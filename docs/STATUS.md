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
- #42 (`convert markdown` writes skeletons) and #43 (`convert plan` heading matches and the
  seeded lock blob) sit in Medium, deferred by Geoff on 2026-10-22.

## Next action

The stage-advance pass (#82, each page advances on its own through its stages) runs under
`conductor-drafter`. Its plan is
`~/Projects/initiatives/docs-system/plans/2026-10-24-drafter-stage-advance-plan.md`.

Once run `20261024T080412Z-13b5d021` has no open batch (`drafter status` with a build of `main`),
run `make install` on pavilion from the main checkout and check `drafter --version`. thinkpad-x1
picks the build up at its next pull and `make install`.

The last nine cairn extend blueprints start from the merged skill.

## Open decisions

None.

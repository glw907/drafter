# Project status

Present tense only. Shipped history is in git and the PRs; strategy lives in the docs-system
initiative at `~/Projects/initiatives/docs-system/`.

## Current state

- `main` is at the merge of PR #58 (`5680e8f`), the pilot-friction pass, and
  `~/.local/bin/drafter` is that build. The batch-friction pass is at PR-ready on branch
  `worktree-batch-friction`; `conductor-drafter` sends its packet and merges on Geoff's approval.
- The pass lands backlog #48 to #81, less #42, #43, and #82, the drafter-owned friction from
  extend blueprints batches 2 and 3. Its plan is
  `~/Projects/initiatives/docs-system/plans/2026-10-24-drafter-batch-friction-plan.md`.
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
- #42 (`convert markdown` writes skeletons) and #43 (`convert plan` heading matches and the
  seeded lock blob) sit in Medium, deferred by Geoff on 2026-10-22.

## Next action

After the merge, `conductor-drafter` checks run `20261024T080412Z-13b5d021` with the merged
build's `drafter status`. While a batch is open, the install waits: STATUS then reads "install
waits on run 20261024T080412Z-13b5d021". Once none is open, `make install` runs on pavilion, and
thinkpad-x1 picks the build up at its next pull and `make install`.

The last nine cairn extend blueprints then start from the merged skill. #82 (each page advances
on its own through its stages) is the next drafter pass, and needs a live run to prove it.

## Open decisions

None.

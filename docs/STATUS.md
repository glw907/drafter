# Project status

Present tense only. Shipped history is in git and the PRs; strategy lives in the docs-system
initiative at `~/Projects/initiatives/docs-system/`.

## Current state

- `main` is at the merge of PR #62 (`1ce0408`), the stage-advance pass, run by executor
  `drafter-stage-advance` (`09a45069`) under `conductor-drafter`. In batch mode a page submits its
  next stage as soon as its batch is handled, while siblings' batches stay in flight (#82).
  The blueprint skill tells a session to name a findings path in the fact-checker dispatch. #42
  and #43 are closed as won't do. #83 (a recovered create of unknown outcome can still hold every
  page on one batch) sits in Medium.
- PR #60 (`15afb8d`), the batch-friction pass, run by executor `drafter-batch-friction`
  (`44a07fee`), landed backlog #48 to #81: the anchorless-pointer warning and wider store checks,
  the lint fixes and `check.sentence_words`, `refs --page` and source kinds, `drafter status`, the
  user-scope local config fallback, and the skill and format-doc fixes.
- `~/.local/bin/drafter` is still the PR #58 build (`5680e8f`). Install waits on run
  `20261024T080412Z-13b5d021`: both its batches were still `in_progress` at the #62 merge.
- cairn-cms backlog #56 carries the follow-up settings for cairn (`check.sentence_words = 26` and
  two `managed: false` outline entries), and cairn #38 notes drafter's tag-note rule.

## Next action

Once run `20261024T080412Z-13b5d021` has no open batch (`drafter status` with a build of `main`),
run `make install` on pavilion from the main checkout and check `drafter --version`. thinkpad-x1
picks the build up at its next pull and `make install`.

The next cairn docs pass's first multi-page `drafter page` is #82's live check. Read that run's
`state.json`: each page's `intents[].time` (submit) and `created`, and `poll.seen`. A page whose
next submit waited for a sibling's batch to end is an escape.

The last nine cairn extend blueprints start from the merged skill.

## Open decisions

None.

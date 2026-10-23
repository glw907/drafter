# Project status

Present tense only. Shipped history is in git and the PRs; strategy lives in the docs-system
initiative at `~/Projects/initiatives/docs-system/`.

## Current state

- PR #58 (`worktree-pilot-friction`) is at PR-ready and waits on Geoff's approval and a merge by
  `conductor-drafter`. `main` is at the merge of PR #57 (reviewed-pages) plus the backlog commits
  that file the extend blueprints pilot's friction.
- The pass lands backlog #36 to #41 and #44 to #47, the drafter-owned friction from the extend
  blueprints pilot (cairn-cms PR #136). Its record is
  `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-pilot-friction.md`.
  - Markdown blueprints gain a `Code:` bullet with `:source[...]` and its fence, a `Figure:`
    bullet, and a `:::constraints` block. A `Code:` source reaches the draft seat.
  - The `blueprint` skill frames the reader from the outline's job and has a rule for parallel
    authors who share one facts store.
  - `drafter check` warns on unqualified exhaustive words and on a pointer whose anchor has
    drifted off its cited lines, and requires `track` only when the outline declares tracks.
    `check --fact <id>` verifies one fact's pointers.
  - `drafter refs` takes several ids and lists the section, brief entry, and drafted page of
    each hit. `drafter outline` and `drafter --version` exist, and `drafter excerpt` serves
    URL-sourced, prose-sourced, and very long sources.
- #42 (`convert markdown` writes skeletons) and #43 (`convert plan` heading matches and the
  seeded lock blob) are deferred to Medium by Geoff on 2026-10-22: the next fifteen cairn pages
  have prose plans only.
- The exhaustive-word lint still fires about 50 times on a 586-line blueprint. Whether `no` and
  `never` stay on its list waits on how authors use it (`BACKLOG.md` #48).

## Next action

After the merge, `conductor-drafter` runs `make install` in the main checkout so
`~/.local/bin/drafter` is the merged build, files the cairn-cms backlog item "Move the three pilot
blueprints to the markdown forms and drop `track: extend`" (`#initiative:docs-system`), and lands
the hand-over. Then the other fifteen cairn extend blueprints start from the merged skill, each
written from its prose plan.

## Open decisions

None.

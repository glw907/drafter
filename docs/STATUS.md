# Project status

Present tense only. Shipped history is in git and the PRs; strategy lives in the docs-system
initiative at `~/Projects/initiatives/docs-system/`.

## Current state

- `main` is at the merge of PR #58 (`5680e8f`), the pilot-friction pass, and
  `~/.local/bin/drafter` is that build (`drafter --version` answers). The executor was
  `drafter-pilot-friction` (`89811a02`), under `conductor-drafter` (`0aeb475d`).
- The pass landed backlog #36 to #41 and #44 to #47, the drafter-owned friction from the extend
  blueprints pilot. Its record is
  `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-pilot-friction.md`.
  - Markdown blueprints have a `Code:` bullet with `:source[...]` and its fence, a `Figure:`
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
  seeded lock blob) sit in Medium, deferred by Geoff on 2026-10-22: the next fifteen cairn pages
  have prose plans only.
- The exhaustive-word lint keeps `no` and `never` on its list (Geoff, 2026-10-22, packet for
  PR #58). #48 revisits them once authors have used the lint on the next pages.
- cairn-cms backlog #30 moves the three pilot blueprints to the markdown forms and drops their
  `track: extend`, which `drafter check` now flags.

## Next action

The other fifteen cairn extend blueprints start from the merged skill, each written from its
prose plan. That work runs in cairn-cms; no drafter pass is waiting.

## Open decisions

None.

# Project status

Present tense only. Shipped history is in git and the PRs; strategy lives in the docs-system
initiative at `~/Projects/initiatives/docs-system/`.

## Current state

- `main` is at the merge of PR #57 (reviewed-pages), plus the backlog commit that files the
  extend blueprints pilot's friction.
- The extend blueprints pilot ran in cairn-cms on 2026-10-22 and merged as cairn-cms PR #136.
  Three blueprints went from v1 plans through `drafter convert plan` and `convert markdown`,
  an author rewrite, and fact-check rounds to Geoff's approval. Its friction record is
  `~/Projects/initiatives/docs-system/research/2026-10-22-extend-blueprints-pilot-friction.md`,
  with each item filed to its owning repo.
- Geoff is fixing that friction before the next round of cairn blueprints, the other fifteen
  extend pages. drafter owns most of it, in `BACKLOG.md`:
  - High: #42 (`convert markdown` writes skeletons), #43 (`convert plan` heading matches and
    the seeded lock blob), #46 (the skill frames the reader from the outline's job), and #47
    (a markdown form for code examples, figures, and page constraints).
  - Medium: #38 (refs that show what a correction redrafts and breaks), #39 (pointer drift
    inside the excerpt window), #40 (`track` validation), #41 (a lint for unqualified
    exhaustive words), #44 (parallel authors sharing a facts store), and #45 (`format.md`'s stale
    wording and hinge example).
  - Low: #36 (a single-fact check, an outline query, and `--version`) and #37 (`excerpt` for
    vendor, charter, and very long sources).
- The blueprint skill lives in this repo at `claude/.claude/skills/blueprint/`, linked into
  `~/.claude/skills/blueprint`. #44 to #47 change it.
- The worktree list holds about 45 merged `agent-*` worktrees and several merged pass
  worktrees, none dirty. The `reviewed-pages` worktree is locked by a session whose process is
  gone, so the lock is stale.

## Next action

Plan and run the pilot-friction pass (launch prompt below).

## Carried into the pass

- The record's numbered items map to the backlog entries, and each entry cites its numbers.
  Read the record whole before planning: its numbers table and Geoff's send-back on the
  custom admin screen are the evidence for #41 and #46.
- The four items in other repos stay out of this pass: approval-queue #21 to #23, dotfiles #31,
  and cairn-cms #28 and #29.
- cairn-cms PR #134 (the admin troubleshooting blueprint, draft) last moved on 2026-10-21 and has
  no live session. A skill or format change here applies to it the next time it is worked.

## Open decisions

None yet. #47 adds markdown blueprint syntax, so it is a format design for Geoff's approval in
the pass's design step.

### Next action (pilot-friction pass)

> **Goal.** Fix the drafter-owned friction from the extend blueprints pilot, so the next round of
> cairn blueprints starts from a reviewable conversion and a skill that frames each reader from
> the outline's job.
>
> **Scope.** In: drafter backlog #36 to #47. Out: the approval-queue, dotfiles, and cairn-cms
> items the record also files, and any cairn blueprint or page.
>
> **Settled (do not re-brainstorm):** charter-sourced facts are dropped from every blueprint,
> with `facts.prose_sources` as the way back once it ships (Geoff, architecture decide D1,
> 2026-10-22). A reader block is framed from the outline's job, never from the plan's worked
> example (Geoff's custom admin send-back, 2026-10-22).
>
> **Approach.** Input: the friction record above and `BACKLOG.md`. Bring a design for #47's
> markdown forms, and the pass's order, for one approval; the High items go first. Go work
> follows `go-conventions`, and the skill files under `claude/.claude/skills/blueprint/` are
> `runner` class, which sends the PR as a packet. Gate: `bash scripts/drafter-gate.sh`.
> Before dispatching, clear the stale `reviewed-pages` lock and prune the merged worktrees.
> Load `pass-core` to start. Launch directory: `~/Projects/drafter`. Model:
> `claude --model claude-opus-5-5`.

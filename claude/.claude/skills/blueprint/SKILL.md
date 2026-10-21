---
name: blueprint
description: Change a documentation page by changing its blueprint. Use when code changes documented behavior, when you notice a docs deficiency while reading (a wrong claim, a missing example, caveat, failure case, or link), when a new docs page is needed, when a repo has a `blueprints/` directory, or when you run `drafter check`, `drafter refs`, or `drafter page`.
---

# Change a page through its blueprint

A blueprint is the durable plan for one docs page: what the page tells its reader, section by section, with each line tied to the facts it rests on. In a repo with a `blueprints/` root, you change the blueprint and let `drafter page` write the prose. A blueprint is `<page>.blueprint.md`, markdown with YAML front matter; an older page may still have a `<page>.blueprint.yaml`, which `drafter convert markdown` turns into the markdown form. The [format page](format.md) is the one home of every format rule; this skill says when to act and in what order.

Every blueprint and every drafted page reaches Geoff only after a fresh agent has fact-checked it against the code and you have settled every finding. API credit pays for drafting only: you author, fact-check, and fix on your session.

## Pick the case

1. **A page is missing.** Follow [new-page.md](new-page.md).
2. **A code change alters documented behavior.** Update the fact bullet for the behavior. Run `drafter refs f:<id>` before you correct a fact: it lists every line on every blueprint that cites it. Fix each line the new fact makes wrong, then redraft the page with `drafter page <blueprint>`. The blueprint, the page, and the facts ride in the code PR. Fact-check the changed lines, settle the findings, and queue one change review for the PR (see [Review](#review)).
3. **You notice a deficiency while reading.** Fix the blueprint, not the page, and take it through the same review as case 2. When the deficiency is a wrong fact, run `drafter refs f:<id>` first and fix each line the corrected fact makes wrong.

A change that adds or removes a section, or changes the reader's path through the page, gets its blueprint approved before the redraft, as a new page does.

## Edit

Read the blueprint whole before you edit it. Each section is a heading with an italic job line under it, then the lines the page carries: bullets and numbered steps, each ending in the `:fact[id]` tags of the facts it rests on, and `:::row` blocks on a symptom page. [Markdown blueprints](format.md#markdown-blueprints) gives every rule. Edit fact bullets with the Edit tool, never a shell append. Never edit `drafted.lock`.

## Check

Run `drafter check <blueprint>` after every edit and fix each error before you commit. A run that names a blueprint skips the facts store, so its `store` warnings are not yours to chase. When a fact changes, `drafter refs f:<id>` is the lookup.

## Fact-check

Commit, then dispatch the `fact-checker` agent on the file at that commit. It must be a fresh agent, never the session that wrote the file. It returns one JSON array of claims, each with a verdict (`holds`, `wrong`, `imprecise`, or `unsupported`), the `file:line` that settles it, and a proposed fix. Save the array verbatim to `<approval-queue state>/findings/<repo>/<file path with / as __>/<commit>.json`.

Settle every finding that is not `holds`, as the author:

- When the code agrees with the finding, fix it: rewrite the line, or the fact and then each line `drafter refs` names.
- When you hold the line is right, dispute it. The dispute goes to Geoff as a decide note with both sides; never resolve one with another agent.
- When the code cannot settle it, ask. The question goes to Geoff as a question note.

Fact-check the fixed lines again with a fresh agent. After three rounds on one document, go on only while each round fixes a cause you have traced; a finding that returns after its fix gets one structural fix (the line or the fact rewritten from the code), and if it returns again it goes to Geoff as a question note.

## Review

Write the notes file the queue shows under each line: an `info` note with `was` and `now` for each fix, a `decide` note with `sides` and `choices` for each dispute, and a `question` note for each question. Then queue the document and wait for Geoff's decision:

| What | Command |
|---|---|
| A blueprint, before its draft | `approval-queue blueprint <worktree> <title> <file> --notes <notes.json>` |
| A drafted page | `approval-queue page <worktree> <title> <file> --pr <owner/repo#n> --notes <notes.json>` |
| A code PR carrying blueprint and page edits | `approval-queue change <owner/repo#n> <title> --notes <notes.json>` |

Each save Geoff makes in the review is a commit in the worktree, so do not edit the file while the review is open. An approve prints the approved blob; a send-back prints his note, and you fix forward from it, fact-check the touched lines, and queue again.

## Draft

Draft only after Geoff approves the blueprint: `approval-queue approved <file> <blob>` must exit 0 for the blueprint's current blob. Then run `drafter page <blueprint>`. With no flag it runs the draft seat, the edit pass that applies the project's editing preferences, and the deterministic gate; `--judge` adds the older judging seats, which this order does not use. Fact-check the draft, settle it the same way, and queue it with `approval-queue page`. Geoff's edits to the page are final: when one changes a claim, update the blueprint or the fact in the same PR. The PR merges after he approves the page.

## Page prose

Change a managed page only through its blueprint and `drafter page`, with two exceptions: the fixes that settle a draft's fact-check findings, and the page edits the project's gate demands when your code change renamed or removed something the page names. In both, change the blueprint to match in the same PR.

---
name: blueprint
description: Change a documentation page by changing its blueprint. Use when code changes documented behavior, when you notice a docs deficiency while reading (a wrong claim, a missing example, caveat, failure case, or link), when a new docs page is needed, when a repo has a `blueprints/` directory, or when you run `drafter check`, `drafter refs`, or `drafter page`.
---

# Change a page through its blueprint

A blueprint is the durable plan for one docs page: what the page tells its reader, section by section, with each line tied to the facts it rests on. In a repo with a `blueprints/` root, you change the blueprint and let `drafter page` write the prose. A blueprint is `<page>.blueprint.md`, markdown with YAML front matter; an older page may still have a `<page>.blueprint.yaml`, which `drafter convert markdown` turns into the markdown form. The [format page](format.md) is the one home of every format rule; this skill says when to act and in what order.

Every blueprint and every drafted page reaches Geoff only after a fresh agent has fact-checked it against the code and you have settled every finding. API credit pays for drafting only: you author, fact-check, and fix on your session.

## Pick the case

1. **A page is missing.** Follow [new-page.md](new-page.md).
2. **A code change alters documented behavior.** Update the fact bullet for the behavior. Run `drafter refs f:<id>...` before you correct a fact (it takes several ids): it lists every line on every blueprint that cites it with the section it sits in, and the briefs and pages a correction breaks. Fix each line the new fact makes wrong, then redraft the page with `drafter page <blueprint>`. The blueprint, the page, and the facts ride in the code PR. Fact-check the changed lines, settle the findings, and queue one change review for the PR (see [Review](#review)).
3. **You notice a deficiency while reading.** Fix the blueprint, not the page. When the deficiency is a wrong fact, run `drafter refs f:<id>...` first (it takes several ids and also lists the briefs and pages the correction breaks) and fix each line the corrected fact makes wrong. Redraft with `drafter page <blueprint>`, open a PR carrying the blueprint and page edits, and take it through the same review as case 2.

A change that adds or removes a section, or changes the reader's path through the page, gets its blueprint approved before the redraft, as a new page does.

## Edit

Read the blueprint whole before you edit it. Each section is a heading with an italic job line under it, then the lines the page carries: bullets and numbered steps, each ending in the `:fact[id]` tags of the facts it rests on, and `:::row` blocks for a symptom, its cause, and its fix. [Markdown blueprints](format.md#markdown-blueprints) gives every rule. Edit fact bullets with the Edit tool, never a shell append. Never edit `drafted.lock`: drafter writes it, and a hand edit desyncs the page from its blueprint.

## Check

Run `drafter check <blueprint>` after every edit and fix each error before you commit. Before the commit, run `drafter check` with no name too: only that run runs the `store` check and the outline-wide checks ([what a named run skips](format.md#what-drafter-check-checks)). After you correct one fact, `drafter check --fact <id>` verifies just its pointers.

## Register edit

When the project has a register editor (cairn's is `cairn-register-editor`), run it on the blueprint before the fact-check and apply its rewrites. A fact-check on prose the editor then rewrites has to run twice.

## Fact-check

Commit, then dispatch the `fact-checker` agent on the file at that commit. It must be a fresh agent, never the session that wrote the file. It returns one JSON array of claims, each with a verdict (`holds`, `wrong`, `imprecise`, or `unsupported`), the `file:line` that settles it, and a proposed fix. Name `${XDG_STATE_HOME:-$HOME/.local/state}/approval-queue/findings/<repo>/<file path with / as __>/<commit>.json` in the dispatch, and the agent writes the array there verbatim and checks that the file parses.

Settle every finding that is not `holds`, as the author:

- When the code agrees with the finding, fix it: rewrite the line, or the fact and then each line `drafter refs` names.
- When you hold the line is right, dispute it. The dispute goes to Geoff as a decide note with both sides; never resolve one with another agent.
- When the code cannot settle it, ask. The question goes to Geoff as a question note. First trace what the code intends through the code, the specs, and the git history, and ask Geoff only product, taste, and scope questions.
- Write each fix line so it passes the `exhaustive` check: a fix that says `only` or `every` draws the same warning it settles.

Fact-check the fixed lines again with a fresh agent. After three rounds on one document, go on only while each round fixes a cause you have traced. A finding that returns after its fix gets one structural rewrite: the line or the fact rewritten from the code, not patched again. If it returns after that, send it to Geoff as a question note.

## Review

Write the notes file the queue shows under each line: a JSON array, one object per note, with `id`, `kind` (`info`, `decide`, or `question`), `anchor`, `title`, and `body` (an array of paragraphs). The anchor names the line the note sits under, in one of two shapes. Use the quote form for every note below a heading: `{"quote": "<text from the line>"}`, with an optional `prefix` or `suffix` of the text around it. The quote can sit on any line, a `:::reader` line included, and it keeps its line when the section gains or loses a bullet above it. Quote text that occurs once in the file, or add a `prefix` or `suffix` that settles which occurrence you mean; a quote that matches nothing puts the note at the top of the file. The index form is a string: `<heading slug>.heading` for a heading, or `<heading slug>.p<n>` for the n-th bullet, step, or row under that heading counting from 0, with `.symptom`, `.cause`, or `.fix` for a row's part. Run `approval-queue anchors <file>` to print the file's heading anchors, slugged as cairn's pages are. Write an `info` note with `was` and `now` for each fix, a `decide` note with `sides` (`who`, `text`) and `choices` (`label`, `done`, and `text` for a wording the choice puts in) for each dispute, and a `question` note with `choices` for each question. The command refuses any other shape.

Then queue the document. `<file>` is relative to `<worktree>` and must be committed:

| What | Command |
|---|---|
| A blueprint, before its draft | `approval-queue blueprint <worktree> <title> <file> --description <description.md> --notes <notes.json>` |
| A drafted page | `approval-queue page <worktree> <title> <file> --description <description.md> --pr <owner/repo#n> --notes <notes.json>` |
| A code PR carrying blueprint and page edits | `approval-queue change <owner/repo#n> <title> --notes <notes.json>` |

`--description` is required on `blueprint` and `page`, and the command refuses a queue without it. The file holds two or three paragraphs that Geoff reads under the title before the document: a background paragraph on the page's reader and job and what came before, then one or two on what this blueprint, or this draft, proposes. The title is sentence case with no closing punctuation and at most 80 characters. The command prints the queued link on stderr as `approval-queue: queued <id> at <url>`. A blueprint or page you queue again after a send-back shows "Revision N" on its own. One you queue again after Geoff approved it opens as an amend review: the screen shows the diff since the approved text, so the `--description` says what changed since the approval, and the decision carries `amends: <blob>`.

Run the command under `setsid -f` with its stdout to a decision file, and watch that file. Never leave the wait to a background task alone: the command's exit trap deletes the queued item when the command dies. Each save Geoff makes in the review is a commit in the worktree, so do not edit the file while the review is open.

An approve prints `{"decision":"approve","file":...,"commit":...,"blob":...,"calls":[...]}`. A send-back prints `{"decision":"reject","note":...}`: fix forward from the note, fact-check the touched lines, and queue again. An amend review's approve and send-back also carry `amends`. A `{"decision":"defer","id":...}` means Geoff will decide later: record the id in your STATUS and end, and a later session runs `approval-queue collect <id>` and acts on what it prints. To take back a queued item, run `approval-queue withdraw <id>`; to resend a deferred one, withdraw it first.

## Draft

For a new page, or a change that adds or removes a section or changes the reader's path, draft only after Geoff approves the blueprint: `approval-queue approved <file> <blob>` must exit 0, with `<blob>` from `git rev-parse HEAD:<file>`. Then run `drafter page <blueprint>` without `--judge`. With no flag it runs the draft seat, the edit pass that applies the project's editing preferences, and the deterministic gate; `--judge` adds review seats and reads that spend credit this process does not use. Exit 0 means the page was accepted. On exit 3, rerun with `--resume <run-id>` and the same arguments; on exit 1, report the escalation; on exit 2 the run could not start, so fix the cause it names and run again.

Fact-check the draft, settle it the same way, run the project's docs gate, and queue it with `approval-queue page ... --description <description.md>`, as the table above shows. Geoff's edits to the page are final: when one changes a claim, update the blueprint or the fact in the same PR. The PR merges after he approves the page.

## Page prose

Change a managed page only through its blueprint and `drafter page`, with two exceptions: the fixes that settle a draft's fact-check findings, and the page edits the project's gate demands when your code change renamed or removed something the page names. In both, change the blueprint to match in the same PR.

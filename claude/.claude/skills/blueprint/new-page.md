# Add a new page

Use this procedure when a docs page does not exist yet. You write the page's entry in `outline.yaml`, its facts, and its blueprint, from the code. Geoff approves the blueprint before anything is drafted, and the page after. Read the [format page](format.md) sections linked below before you write.

## 1. Settle the page's job and type

Test the premise first: name a reader whose task no page in the outline already covers. When you
can't, recommend folding the page into the closest neighbor, put the recommendation in the
hand-back report, and stop.

Write down the job: what the reader accomplishes with the page. Pick its page type from the types `outline.yaml` declares. Read the anatomy the type names in [anatomies.md](anatomies.md) for its introduction and ending, or, when the page's track names a register with an entry for the anatomy, that entry. The outline entry's fields and what each requires are in [outline.yaml](format.md#outlineyaml).

Set `covers` and `outOfScope` from the neighbors: read the outline whole, so you know each page's job and what another page owns. Keep out of this page whatever a neighbor owns. When this page's `covers` narrows, reword each neighbor's `outOfScope` line that routes a topic here. When a section folds into a page that has no blueprint, write that page a whole-page blueprint and queue it with notes that point at the section.

## 2. File or re-verify each fact from the code

Every command, step, warning, success signal, and assertion the page makes is a claim, and each one rests on a fact bullet in the facts store. Before the blueprint cites a fact, read the code it points at:

- A fact already filed for this page, or one the page reuses, is re-verified: read its source and correct the bullet or its pointer where the code says otherwise.
- A claim with no fact gets one, filed code first. Find the code, config, or vendor document that settles it, choose the narrowest pointer that shows the whole claim, run `drafter excerpt <pointer>`, and write the bullet with the excerpt in view, so it says no more than the lines show.
- A claim whose only source is another docs page is retraced to code, config, or a vendor document, or it is not filed.
- A claim another page owns is not filed here; the blueprint links that page.

Never file a fact tagged `candidate` or `docs-drift`. Edit fact bullets in place with the Edit tool: replace the wrong text exactly, and never add a second bullet, a line below it, or a shell append.

When an example on the page comes from a script you ran, put the script's path in the `Code:` bullet's `:source`. When it comes from a command, name the command in the bullet's text. Either way the fact-checker can run it again.

When several authors share one facts store, an author who finds a shared fact wrong fixes the bullet and names the fix in the hand-back report. An author who sees another author's fix re-verifies it against the code and never reverts it; the pilot's parallel authors reverted each other's fixes. Before you commit, install the project's dependencies and run its facts check. Cairn's `check:facts`, for one, needs `npm ci` first.

## 3. Write the blueprint

Write `<page>.blueprint.md` in [the markdown format](format.md#markdown-blueprints): the front matter (`page`, `type`, `track` only when the outline declares tracks, and an optional `exemplar`), then the `:::reader` block, written from the questions in [introductions.md](introductions.md): who reads the page, why they arrive, what they already know, what the page does for them, and what it leaves to other pages. Take the reader from the outline entry's job and `covers`, never from a worked example in a plan; Geoff sent back the custom admin screen page (2026-10-22) because its reader shrank to the plan's example.

Put instructions that bind the whole page in a [`:::constraints` block](format.md#the-constraints-block); the pilot's authors put them in a `**Background.**` paragraph inside the reader block.

Set `exemplar` to the published page to imitate. A page outside the repository must be allowed by `inline_files`, which `drafter.local.toml` or `~/.config/drafter/<repo>.local.toml` sets ([Configuration](https://github.com/glw907/drafter/blob/main/docs/configuration.md)). Mint each new fact id with the project's id command when it has one, since a hand-typed id can collide. Find a plan's facts by grepping the store for each id the plan cites.

Then the sections. Order them by when the reader needs each one; Google's outline guidance is the source. Don't copy the order of `covers`: it is an inventory.

Under each heading, write the italic job line that says what the section does for the reader, then the lines the page will carry, each a bullet or numbered step ending in its `:fact[id]` tags. A symptom page gives each symptom a `:::row` with its Symptom, Cause, and Fix. Write each line as the page should say it to its reader: the blueprint is what Geoff reads and approves, so it reads as the page's plan in plain sentences, not as notes.

## 4. Check and commit

Run `drafter check`. Fix every error: [What `drafter check` checks](format.md#what-drafter-check-checks) lists them. Commit the outline entry, the facts, and the blueprint together.

## 5. Fact-check, settle, and queue

Fact-check the blueprint with a fresh `fact-checker` agent, settle every finding, write the notes, and queue it with `approval-queue blueprint <worktree> <title> <file> --description <description.md> --notes <notes.json>`, as [Fact-check](SKILL.md#fact-check) and [Review](SKILL.md#review) say.

## 6. Draft, fact-check, and queue the page

When `approval-queue approved <file> <blob>` exits 0 for the blueprint, run `drafter page <blueprint>`. Fact-check the drafted page, settle the findings with hand edits to the page (and to the blueprint or the fact where a claim was wrong at its source), run the project's docs gate, and queue the page with `approval-queue page <worktree> <title> <file> --description <description.md> --pr <owner/repo#n> --notes <notes.json>`. The PR merges after Geoff approves the page.

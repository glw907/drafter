# Add a new page

Use this procedure when a docs page does not exist yet. You write the page's entry in `outline.yaml`, its facts, and its blueprint, from the code. Geoff approves the blueprint before anything is drafted, and the page after. Read the [format page](format.md) sections linked below before you write.

## 1. Settle the page's job and type

Write down the job: what the reader accomplishes with the page. Pick its page type from the types `outline.yaml` declares. Read the anatomy the type names in [anatomies.md](anatomies.md) for its introduction and ending, or, when the project's register has an entry for the anatomy, that entry. The entry's fields and what each requires are in [outline.yaml](format.md#outlineyaml).

Set `covers` and `outOfScope` from the neighbors: read the outline whole, so you know each page's job and what another page owns. Keep out of this page whatever a neighbor owns.

## 2. File or re-verify each fact from the code

Every command, step, warning, success signal, and assertion the page makes is a claim, and each one rests on a fact bullet in the facts store. Before the blueprint cites a fact, read the code it points at:

- A fact already filed for this page, or one the page reuses, is re-verified: read its source and correct the bullet or its pointer where the code says otherwise.
- A claim with no fact gets one, filed code first. Find the code, config, or vendor document that settles it, choose the narrowest pointer that shows the whole claim, run `drafter excerpt <pointer>`, and write the bullet with the excerpt in view, so it says no more than the lines show.
- A claim whose only source is another docs page is retraced to code, config, or a vendor document, or it is not filed.
- A claim another page owns is not filed here; the blueprint links that page.

Never file a fact tagged `candidate` or `docs-drift`. Edit fact bullets with the Edit tool, never a shell append.

## 3. Write the blueprint

Write `<page>.blueprint.md` in [the markdown format](format.md): the front matter (`page`, `type`, `track`, `exemplar`), the `:::reader` block that says who reads the page, why they arrive, what the page does for them, and what it leaves to other pages, then the sections. Argue their order from the reader: Google's outline guidance is the source, so the page introduces information when it is most relevant to the reader, and the order of `covers` is only an inventory.

Under each heading, write the italic job line that says what the section does for the reader, then the lines the page will carry, each a bullet or numbered step ending in its `:fact[id]` tags. A symptom page gives each symptom a `:::row` with its Symptom, Cause, and Fix. Write each line as the page should say it to its reader: the blueprint is what Geoff reads and approves, so it reads as the page's plan in plain sentences, not as notes.

## 4. Check and commit

Run `drafter check`. Fix every error: [What `drafter check` checks](format.md#what-drafter-check-checks) lists them. Commit the outline entry, the facts, and the blueprint together.

## 5. Fact-check, settle, and queue

Fact-check the blueprint with a fresh `fact-checker` agent, settle every finding, write the notes, and queue it with `approval-queue blueprint`, as [Fact-check](SKILL.md#fact-check) and [Review](SKILL.md#review) say.

## 6. Draft, fact-check, and queue the page

When `approval-queue approved <file> <blob>` exits 0 for the blueprint, run `drafter page <blueprint>`. Fact-check the drafted page, settle the findings with hand edits to the page (and to the blueprint or the fact where a claim was wrong at its source), run the project's docs gate, and queue the page with `approval-queue page`. The PR merges after Geoff approves the page.

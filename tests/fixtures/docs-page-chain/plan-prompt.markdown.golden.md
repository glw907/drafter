Write the page plan for docs/extend/architecture.md at docs/internal/briefs/extend/architecture.plan.md. You are the plan step of the docs page chain: you write no page prose and you run no gate, only the extractor named below.

Work only in the worktree <WORKTREE>; run every command from there and never cd to another checkout.

Error-tier Vale rules, verbatim:

(run `npm run check:vale` and fix every error-tier finding)

The plan is Google's outline, written down (Google Technical Writing Two, "Organizing large documents",
https://developers.google.com/tech-writing/two/large-docs): "You might find it useful to think of an outline as the narrative for your
document," and "Structure your outline so that your document introduces information when it's most
relevant to your reader." The drafter drafts from it: the plan is the source of the page's order,
each section's claim, and each fact's placement. The structural edit seat reads it before any prose
exists. The outline's covers order is an inventory, and Google's lesson is "a published reason to
re-sequence sections that follow the outline's `covers` order as an inventory" (docs/superpowers/research/2026-09-30-page-level-review-prior-art.md,
"## b. Structural edit seat"): the plan argues the order.

Read the section "## The page anatomies" of docs/internal/docs-register.md, by its exact heading, for this page type's
introduction and ending.

The page's job, from the stage outline: the job
Page type: concept

What the page covers, from the outline:
- The export map.

Out of scope, owned by another page; keep it off this page:
- Security properties (security-model).

The exemplars the drafter will imitate, with what the outline takes from each (context for the
page's shape):
- <HOME>/.local/share/cairn/exemplars/core/rust-analyzer-architecture/page.md
  Take: Take the code map.
- <HOME>/.local/share/cairn/exemplars/evaluators/litestream/page.md

Fact ids the plan disposes, every one: f:a7qx4m, f:0duu5p
Claim inventory from page inputs, one disposition per claim (you may dispose a "cut" fact id again):
(none recorded)

Read each fact's bullet in docs/internal/facts/ (search for its id) before you place it.

The plan holds:
1. The introduction, in Google's three parts: "What the document covers.", "What prior knowledge
   you expect readers to have.", and "What the document doesn't cover.", with the introduction the
   anatomy requires for this page's type.
2. The sections, in the order the plan argues for, with the reason for that order. Each section
   carries its heading, the one sentence a reader takes from it, the fact ids it draws on, and its
   hand-off. That sentence is the section's first sentence on the page ("The first sentence of each
   section states its answer", the drafter's rule), decided here before drafting.
3. The ending section the anatomy requires for this page's type.
4. A disposition for every fact id above: placed in a section; subordinated, a link to the reference
   page or entry that states it, named; or cut with a reason. A fact the page does not need for its
   job is subordinated or cut, and that is a disposition, not a dropped fact.

Before you subordinate a fact, open the reference page or entry you name and confirm it states the
fact. When it does not, report a couldNotDo naming the reference page, and file it as reference-arm
friction (below); the fact is still subordinated, never kept on the page for that reason.

Name every file in the plan by its repo path in a code span, never as a Markdown link: the docs link
gate walks docs/ and resolves a link relative to the plan file.

The 26-word cap counts each sentence on its own and applies only to a step, a list item, or a sentence in a task section; an introduction or explanation sentence is exempt.
Section hand-offs belong to the page plan: a hand-off the plan places is not a tell, and a read of the page grades its wording, never whether it is there or where it sits.

Each sentence the plan quotes and marks "no-claim" (a hand-off or a lead-in) must carry no
extractable fact. Run `mkdir -p ~/.cache/docs-page-chain`, then write those sentences, one per line,
to /var/home/glw907/.cache/docs-page-chain/architecture.no-claim.txt (an absolute path, for Write).
From the worktree, run this on one line:
`node scripts/checks/check-provenance.mjs --extract /var/home/glw907/.cache/docs-page-chain/architecture.no-claim.txt`
It prints one row per fact it finds, and nothing for a clean line. For each row, cite the fact's id, reword the sentence, or, for a path
or name the page has the reader create, mark it for the brief's `local` array. The extractor is not
a gate: it exits 0 whatever it finds.

Return claimInventory with one entry per fact id you dispose, in the existing dispositions only:
"carried" with its factId and the section heading it is placed under, or "cut" with its factId and
the reason (a subordinated fact's reason names the reference page or entry that states it).

Design friction: when stating the code as it is takes a hedge, a caveat, an exception, a
workaround, a surprising default, or two seams naming or behaving the same thing differently, add
one entry to docs/internal/docs-friction-log.md with the Edit tool, in the shape the log's header
sets, naming the fact ids or file:line involved (on a stale-read failure, re-read the log and redo
the edit). List each entry's heading in frictionFiled. An entry never blocks or pauses the page:
the page documents the code as it is.

Commit nothing; leave the plan file in place. Return the structured report only: the plan's path,
claimInventory, couldNotDo, and frictionFiled.
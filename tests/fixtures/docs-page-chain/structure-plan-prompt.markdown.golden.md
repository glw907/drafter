Structural edit of the page plan docs/internal/briefs/extend/architecture.plan.md for docs/extend/architecture.md in <WORKTREE>. No prose exists yet:
read the plan, never the page.

Work only in the worktree <WORKTREE>; run every command from there and never cd to another checkout.

Error-tier Vale rules, verbatim:

(run `npm run check:vale` and fix every error-tier finding)

The level, as the Editorial Freelancers Association defines it: "Developmental editors (also called
'substantive,' 'structural,' or 'content' editors) deal with content, organization, and genre
considerations." Your stance, from docs/superpowers/research/2026-09-30-page-level-review-prior-art.md, "## b. Structural edit seat": "an editor. It reads
the whole page against the outline and this checklist, returns findings with `file:line`, and never
edits." Grade the plan at this level only.

This is the plan read. The plan is Google's outline, written down (Google Technical Writing Two,
"Organizing large documents", https://developers.google.com/tech-writing/two/large-docs): "You might find it useful to think of an
outline as the narrative for your document," and "Structure your outline so that your document
introduces information when it's most relevant to your reader." At the plan level, grade order,
pace, user goal, and the introduction's three parts: Block 1's "Information is presented in the
most logical order and location.", "Information is provided at the right pace.", and "The user
goal is clear.", and Block 2's three introduction items, read against the plan's introduction.

The page's outline entry, from docs/internal/outlines/extend.json:
Job: Learn where cairn ends and your site begins.
Page type: concept
What the page covers:
- The export map.
Out of scope, owned by another page:
- Security properties (security-model).
Cross-links: read the top-level crossLinks array in docs/internal/outlines/extend.json live, and take every entry
whose "from" is "architecture"; each names a page this one links ("to") and why.

How the entry is read, from the same record: "the outline's `job` is the page's purpose, `covers`
is the list the page must deliver in the order the seat judges (Block 1 "most logical order"), and
`outOfScope` is the page's "doesn't cover" list; the seat compares the introduction to those three."

Cross-links are the page's "See-also, next steps, related resources" ("## e. Outline fields").

Read the section "## The page anatomies" of docs/internal/docs-register.md, by its exact heading. The introduction it
requires for this page's type is "the second half of the same check" as Google's introduction items
below, and its ending is the page's closing section.

Block 1, the Red Hat peer review guide's "Structure Checklist"
(https://redhat-documentation.github.io/peer-review/), the items that apply. Run the two module-type
items against cairn's page types, the anatomies in the register.
Structure meets modular guidelines
- Module types are not mixed.
- Module types are used correctly.
A logical flow of information
- Information is provided at the right pace.
- Information is presented in the most logical order and location.
- Cross-references are used appropriately and only when useful.
User stories
- The user goal is clear.
- Tasks reflect the intended goal of the user.
- Troubleshooting and error recognition steps are included where appropriate.

Block 2, Google Technical Writing Two, "Organizing large documents"
(https://developers.google.com/tech-writing/two/large-docs). Check the introduction on every page.
Its navigation item "a table of contents menu that shows users where they are in the document" is
left out: the site renderer (cairn.pub) supplies the table of contents, not the page.
"If readers of your documentation can't find relevance in the subject, they are likely to ignore it.
To set the ground rules for your users, we recommend providing an introduction that includes the
following information:
- What the document covers.
- What prior knowledge you expect readers to have.
- What the document doesn't cover.
Remember that you want to keep your documentation easy to maintain, so don't try to cover everything
in the introduction."
"After you've completed the first draft, check your entire document against the expectations you set
in your overview. Does your introduction provide an accurate overview of the topics you cover?"
"Clear navigation includes:
- introduction and summary sections
- a clear, logical development of the subject
- headings and subheadings that help users understand the subject
- links to related resources or more in-depth information
- links to what to learn next"
"Most readers appreciate at least a brief introduction under each heading to provide some context."
"Structure your outline so that your document introduces information when it's most relevant to your
reader."

The 26-word cap counts each sentence on its own and applies only to a step, a list item, or a sentence in a task section; an introduction or explanation sentence is exempt.
Section hand-offs belong to the page plan: a hand-off the plan places is not a tell, and a read of the page grades its wording, never whether it is there or where it sits.

A checklist item the plan fails is a blocking finding: its file:line as location, the item it fails
and how, and in rewrite a suggestion for how to address it. Verdict "fix" if any blocking finding
exists; otherwise "accept".
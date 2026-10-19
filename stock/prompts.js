// drafter's stock prompts module, run when a project's configuration names no prompts module. It
// runs in goja as the body of an async function, with DRAFTER_HOST and args as globals, and
// returns the seat map. It defines the blueprint read and the seats that draft and review a page
// from its blueprint.

const GOOGLE_LARGE_DOCS = "https://developers.google.com/tech-writing/two/large-docs";

const PAGES = (DRAFTER_HOST.resolved && DRAFTER_HOST.resolved.pages) || [];

// slug returns a heading's GitHub-style anchor, as drafter check computes it, for the ASCII names
// the stock anatomies use: lowercased, punctuation dropped, each space a hyphen.
function slug(name) {
  return name.toLowerCase().replace(/[^a-z0-9 _-]/g, "").replace(/ /g, "-");
}

/**
 * The anatomy a page's blueprint read checks the introduction and closing against, as the token
 * drafter inlines after the prompt. The page's resolved entry names the anatomy its type follows
 * and, when its track names one, a register: the register's anatomies section is read then.
 * Otherwise the stock page's section for the anatomy is read, or the whole stock page when the
 * entry names no anatomy, as for a planner's handoff.
 * @param {{anatomy?: string, register?: string}} entry
 * @returns {{token: string, name: string, register: boolean}}
 */
function anatomyFor(entry) {
  const name = entry.anatomy || "";
  if (entry.register) return { token: `${entry.register}#the-page-anatomies`, name, register: true };
  if (name) return { token: `drafter:anatomies.md#${slug(name)}`, name, register: false };
  return { token: "drafter:anatomies.md", name, register: false };
}

/**
 * The paragraph that sends the read to the page's anatomy, only for the introduction and the
 * closing section it requires. Its text names a register only in a register's token.
 * @param {{token: string, name: string, register: boolean}} a
 * @returns {string}
 */
function anatomyParagraph(a) {
  let read;
  if (a.register) {
    read = a.name
      ? `Read the entry for the anatomy "${a.name}" in \`${a.token}\`, inlined below, by its exact name.`
      : `Read the entry for this page's type in \`${a.token}\`, inlined below.`;
  } else if (a.name) {
    read = `Read the anatomy "${a.name}" in \`${a.token}\`, inlined below.`;
  } else {
    read = `Read the anatomy this page's type names in \`${a.token}\`, inlined below, or, when the
outline entry names none, the one that fits the page's type and job.`;
  }
  return `${read}
Read it only for the introduction and the closing section it requires for this page's type: they
are the second half of the same check as the introduction items below. No other rule in it is
graded here. Its heading forms and other rules govern prose, and the editor seat applies them;
neither blueprint read returns a finding under one.`;
}

/**
 * The blueprint read's prompt: a developmental edit of a durable blueprint against a checklist,
 * with the reads that stop a page a reader can't follow (reader state, recovery, completeness)
 * and the reads a durable file adds. drafter appends the blueprint, its outline entry, the facts
 * it places, and the anatomy token's text after this text. Any blocking finding means "fix". A
 * truthy `scope` (the sections that changed since a revision, or "page") appends one paragraph
 * that turns the read into a change read: the blueprint is still read whole, but a finding blocks
 * only when the change makes the page wrong or leaves a reader stuck. The scope's value never
 * reaches the text, since drafter's change block names the sections, so every scoped render of a
 * page is byte-identical and shares one prompt cache entry.
 * @param {string} [scope]
 * @param {{token: string, name: string, register: boolean}} anatomy
 * @returns {string}
 */
function blueprintPrompt(scope, anatomy) {
  return `Blueprint read. The blueprint below is a documentation page's durable plan, a YAML file with
an \`id\` and a \`why\` on each section, items of several kinds, and a \`framing\` block. No prose exists
yet: read the blueprint, never a page. The page's job, type, and what it covers sit in its outline
entry, and the facts it places sit in the facts block, all inlined below. A finding's location is a
section id and, when it applies, an item id.

The blueprint is Google's outline, written down (Google Technical Writing Two, "Organizing large
documents", ${GOOGLE_LARGE_DOCS}): "You might find it useful to think of an outline as the narrative for
your document," and "Structure your outline so that your document introduces information when it's
most relevant to your reader." Your stance is an editor's: the Editorial Freelancers Association says
"Developmental editors (also called 'substantive,' 'structural,' or 'content' editors) deal with
content, organization, and genre considerations." You read the whole blueprint against this checklist,
return findings, and never edit.

How the blueprint is read. A \`step\` item is an action; \`requires\` and \`produces\` name state tokens, and
\`preconditions\` lists the state a reader brings. A \`failure\` item names the step it follows in \`from\`
and the item that handles it in \`handledBy\`. A \`fact\` item places a fact, and its \`use\` says which
part of the fact the placement carries. An \`example\` item is a worked example. A prose field names
another section as [[section-id]]. An item with \`origin: human\` carries a person's edit. A
section's \`why\` says why it is there and sits where it does; the page-level \`orderReason\` explains
only the order as a whole. \`drafter check\` has already run: ids are unique, every step's \`requires\`
is declared by an earlier step or by \`preconditions\`, every step marked \`canFail\` has a failure
item, and every fact resolves. That proves the blueprint consistent, not complete. Judge the rest
yourself.

${anatomyParagraph(anatomy)}

Order, pace, and user goal. Block 1, the Red Hat peer review guide's "Structure Checklist"
(https://redhat-documentation.github.io/peer-review/), the items that apply:
- Information is presented in the most logical order and location.
- Information is provided at the right pace.
- The user goal is clear.
- Tasks reflect the intended goal of the user.
- Troubleshooting and error recognition steps are included where appropriate.
Read the sections in order against the outline entry: its \`job\` is the page's purpose, its \`covers\`
the list the page must deliver, and its \`outOfScope\` the page's "doesn't cover" list.

The introduction. Block 2, Google Technical Writing Two, "Organizing large documents": "an introduction
that includes the following information: What the document covers. What prior knowledge you expect
readers to have. What the document doesn't cover." Compare the blueprint's introduction to the
outline entry's \`job\`, \`covers\`, and \`outOfScope\`, and grade the framing block's introduction plan
against what the sections deliver.

Completeness of steps and failures. A page with no deploy step passes the check unless some step
declares that it requires a deployment. Return a blocking finding for each step the page's job needs
that the blueprint lacks, and for each failure a reader following the steps could meet that has no
\`failure\` item, naming the missing step or failure and where in the blueprint it belongs.

Reader state. Read each step's \`text\` as a reader would. Return a blocking finding for a step whose
prerequisite a reader cannot hold at that point: the step relies on something that no earlier step
produces and no precondition lists, even where \`requires\` does not declare it. A dependency stated
only in prose, such as a step that assumes a deployed Worker, is the case this read exists for.

Recovery. Return a blocking finding for a failure a reader following the steps can meet at a step
but that has no \`from\`, or whose \`from\` names a step where it cannot occur, and for a recovery
route that appears before the diagnosis that tells the reader which failure they have. A failure
with no \`from\` is a free-standing symptom row, which the format allows for a symptom met in the
site's ordinary use after the steps end, such as an editor's failed publish. Do not ask for a step
the page does not contain.

A \`use\` names scope only. Compare each \`use\` with its fact in the facts block. Return a blocking
finding for each \`use\` that restates the fact's content instead of scoping it, since a corrected
fact would leave that paraphrase stale. A \`use\` that starts with "Only" or "Omit" and names a part of
the fact passes.

Durable fields describe the page as it is. \`orderReason\`, each \`why\`, \`constraints\`, and the \`framing\`
parts say what the page is and why. Return a blocking finding for each one that narrates the planning
session instead: what was merged, moved, or cut against an older outline, or a departure from an
anatomy. That history belongs in the commit message.

Human-origin items. A person wrote each item marked \`origin: human\`, and the redraft must carry its
point with real weight. Return a blocking finding for each such item the blueprint would bury: placed
where the section's order or its \`opens\` line leaves it a clause, a parenthetical, or a footnote.

A checklist item the blueprint fails is a blocking finding: its section id, and item id where one
applies, as location, the item it fails and how, and in rewrite a suggestion for how to address it.
Verdict "fix" if any blocking finding exists; otherwise "ok".${scope ? `

Change read. This read judges a change, as a code reviewer judges a pull request. The change block
after the facts opens by naming the sections the change touched or, for a page-level change, by
saying that no section's own content changed. It then holds the diff of the blueprint since the
earlier revision, the old and new text of each fact bullet that changed, and the outline entry before
and after when it changed. Read the whole blueprint for context, as Google's code review standard asks
("Sometimes you have to look at the whole file to be sure that the change actually makes sense",
https://google.github.io/eng-practices/review/reviewer/looking-for.html), and judge the change by the
same standard: "favor approving a CL once it is in a state where it definitely improves the overall
code health" (https://google.github.io/eng-practices/review/reviewer/standard.html). A finding blocks
only when the change makes the page wrong or leaves a reader stuck: a claim past its fact, invented
output, a handler that loops or dead-ends, a state no step supplies, or a missing step. That covers a
defect in what the change adds or alters and a defect the change causes elsewhere, such as a step
whose \`requires\` the change leaves with no source, or a \`failure\` item whose step the change
removes. Locate that second kind at the changed item that causes it. Order, pace, heading form, field
placement, and a \`use\` that restates its fact are non-blocking, and their finding text starts "Nit:".
The same standard says of a Nit: "if it's not very important, prefix it with something like "Nit: " to
let the author know that it's just a point of polish that they could choose to ignore." In a change
read, this rule replaces the blocking rule above for order, pace, heading form, field placement, and a
restating \`use\`: return each as a non-blocking finding starting "Nit:". A Nit never makes the
verdict "fix". Every other defect is non-blocking: set blocking to false and start its finding with
"Already present:". The standard sends an existing problem to a bug ("encourage the author to file a
bug"), so report it and let it pass. A non-blocking finding never makes the verdict "fix".` : ""}`;

}

/**
 * A placeholder for a drafting or review seat: one line naming the page. The seat prompts replace
 * these.
 * @param {string} seat
 * @returns {(c: {page?: {path?: string}}) => string}
 */
function stub(seat) {
  return (c) => `The ${seat} seat, for ${c.page && c.page.path}.`;
}

return {
  blueprint: (c) => {
    const path = c.page && c.page.path;
    const entry = PAGES.find((p) => p.path === path) || {};
    return blueprintPrompt(c.scope || undefined, anatomyFor(entry));
  },
  draft: stub("draft"),
  structure: stub("structure"),
  editor: stub("editor"),
  fact: stub("fact"),
  reader: stub("reader"),
  coherence: stub("coherence"),
};

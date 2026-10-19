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

// The seats below read only the context object: the page's resolved entry in `page`, the round,
// the findings, the narrowed read, the turn, and the scope. Their text holds no prose slash and no
// repository path but the page's and the brief's, since drafter inlines every path a prompt names.

/**
 * The section ids a scope names, from its comma-joined form.
 * @param {string} scope
 * @returns {string[]}
 */
function scopeIDs(scope) {
  return String(scope || "").split(",").map((s) => s.trim()).filter(Boolean);
}

/**
 * The section ids a scope names, each in code font, joined for a sentence.
 * @param {string} scope
 * @returns {string}
 */
function idList(scope) {
  return scopeIDs(scope).map((id) => `\`${id}\``).join(", ");
}

/**
 * The page's brief as a prompt names it: by path when the entry carries one.
 * @param {{brief?: string}} p
 * @returns {string}
 */
function briefName(p) {
  return p.brief ? `the brief at ${p.brief}` : "the page's brief";
}

/**
 * The phrase that sends a seat to the page's anatomy by its token. It names a register only in a
 * register's token.
 * @param {{token: string, name: string, register: boolean}} a
 * @returns {string}
 */
function anatomyRead(a) {
  if (a.register) {
    return a.name
      ? `the entry for the anatomy "${a.name}" in \`${a.token}\`, inlined below, by its exact name`
      : `the entry for this page's type in \`${a.token}\`, inlined below`;
  }
  if (a.name) return `the anatomy "${a.name}" in \`${a.token}\`, inlined below`;
  return `the anatomy in \`${a.token}\`, inlined below, that fits the page's type and job`;
}

/**
 * The brief's schema and the provenance rules drafter's gate holds a page and its brief to.
 * @param {string} lead - the sentence that opens the rules
 * @returns {string}
 */
function briefRules(lead) {
  return `${lead} The brief is a JSON object with one key, "sentences": an array holding one
entry per sentence of the page's prose, in page order, each {"text": "the sentence as the page has
it", "id": the facts it rests on}. drafter's gate checks the page and the brief together, and any
defect fails the page:
- Every sentence of the page's prose is in the brief, and every brief sentence is on the page, word
  for word apart from emphasis and code marks. Prose is the page less its front matter, fenced
  code, HTML comments, details and summary blocks, images, and heading lines, so a list item and a
  table cell are prose too.
- "id" is the bare string "no-claim", one fact id, or a list of the fact ids a sentence states
  together. A list is never empty and never holds "no-claim".
- Cite only an id the facts block holds, with the tag verified, external, or vendor.
- A cited sentence states nothing its facts don't. Each number, version, date, path, URL, command,
  flag, and code name in it appears in a fact it cites; numbers and versions are matched in the
  fact's claim text only.
- A "no-claim" sentence states none of those: an introduction, a lead-in, or a transition that
  carries no checkable specific. A sentence that names a command or a number cites the fact that
  states it.
- Every \`fact\` item of the blueprint is cited by at least one sentence in that item's section.`;
}

/**
 * The paragraph that says how a drafting seat reads the blueprint's fields.
 * @returns {string}
 */
function blueprintReading() {
  return `How the blueprint is read. Draft its sections in its order, each under its \`heading\`
verbatim, at its \`level\`, and each opening on the sentence its \`opens\` field gives. Each item is
content its section carries. A \`fact\` item places a fact, and its \`use\` says which part of the
fact the placement carries. A \`sentence\` item with \`mode: verbatim\` appears word for word, and
one with \`mode: claim\` states its point in your words. A \`step\` is a numbered step, in order. A
\`failure\` item is a symptom with the recovery its \`handledBy\` names. An \`example\` or \`code\`
item is a worked example, a \`link\` item a Markdown link to its \`target\`, and a \`handoff\` the
lead-in to what the reader does next. A prose field names another section as [[section-id]]. An
item marked \`origin: human\` carries a person's edit: give its point full weight. A section's
\`styleNotes\` give wording a person fixed, so use each \`new\` wording. \`constraints\` are rules
for this page. A fact that \`dispositions\` cuts stays off the page, and a subordinated one appears
only as the link its \`reference\` names.`;
}

/**
 * The paragraph that keeps a drafting seat to the facts.
 * @returns {string}
 */
function factsDiscipline() {
  return `Facts. Write only what a fact in the facts block states, and only as much of it as the
item's \`use\` allows. A detail no fact states, such as an option, a default, an output line, or a
number, is an invention the reader will act on: leave it out. When the blueprint needs a claim that
no fact supports, leave the claim out and name it in friction. A source excerpt that follows a fact
is there to read the fact correctly, never to add to it.`;
}

/**
 * The whole draft: the page from its blueprint, with its brief, at round 1 with no findings.
 * @param {Record<string, any>} p - the page's resolved entry
 * @returns {string}
 */
function wholeDraft(p) {
  const a = anatomyFor(p);
  const title = p.title ? `\nThe page's H1, verbatim: # ${p.title}\n` : "";
  const reader = p.reader ? `\nThe page's reader, from its track: ${String(p.reader).trim()}\n` : "";
  return `Draft the page ${p.path} from its blueprint, and write ${briefName(p)}. The blueprint, its
outline entry, and the facts the blueprint places or cites are inlined below. They and the files
this prompt names are everything you draft from. The outline entry holds the page's job, its type,
what it covers, and what it leaves out.
${title}${reader}
${blueprintReading()}

The introduction and the close. Plan the introduction from \`framing.introPlan\` and the
blueprint's \`introduction\`: what the page covers, the prior knowledge it expects, and what it
leaves out. Write it for the readers \`framing.readers\` names. Read \`drafter:introductions.md\`,
inlined below, for how an introduction starts from the reader. Follow ${anatomyRead(a)}, for the
parts the introduction has and the closing section this page's type requires. The anatomy gives
the parts, never the words.

The standard. Write to the Google developer documentation style guide.
\`drafter:google-description.md\` describes the standard and its reader,
\`drafter:google-tells.md\` lists the patterns it rules out, each with a before-and-after pair, and
\`drafter:google-corpus.md\` holds short passages in its voice, all inlined below. Imitate the
corpus for voice only, and never take its facts, terms, product names, or wording.

${factsDiscipline()}

${briefRules("The brief.")}

Answer with the whole page as Markdown, its brief, and any friction you met.`;
}

/**
 * A draft turn that edits the page as it stands: a section redraft's first turn, the round-1
 * redraft, or the final redraft. A scope bounds every turn of a section redraft.
 * @param {Record<string, any>} c - the context object
 * @param {Record<string, any>} p - the page's resolved entry
 * @returns {string}
 */
function editDraft(c, p) {
  const scope = c.scope || "";
  let head;
  if (c.afterFinalRead) {
    head = `Redraft ${p.path} once, on the final reader's findings below, and keep ${briefName(p)} in
step. Change only the spots they cite, and leave the rest of the page as it stands.`;
  } else if (c.round === 1 && scope) {
    head = `Redraft the sections ${idList(scope)} of ${p.path}, and keep ${briefName(p)} in step. The
blueprint entries of those sections changed: the change block after the blueprint shows how, and
each finding below names a section the page must now carry as its blueprint entry stands.`;
  } else {
    head = `Redraft ${p.path} on the review findings below, and keep ${briefName(p)} in step. Fix
every blocking finding, and take an advisory one when it's right. Don't widen the page.`;
  }
  const bound = scope
    ? `

This is a section redraft. Change only the sections in scope: ${idList(scope)}. The blueprint
names each section's id beside its heading. Leave the text before the first heading, every heading
line, and every other section as they stand, line for line, and leave the brief's sentences for
those parts unchanged. A finding from the coherence read is the exception: fix it where it lies.`
    : "";
  return `${head}${bound}

The page, its brief, the blueprint, its outline entry, and the facts the blueprint places or cites
are inlined below. Draft as the first draft did: from the blueprint and its facts, to the Google
developer documentation style guide, and in the shape of the page's anatomy.

${factsDiscipline()}

Findings:
${c.findings}

${briefRules("The brief's rules still hold, and the gate checks them after your edits.")}`;
}

/**
 * The draft seat: a whole draft at round 1 with no findings, and an edit turn otherwise.
 * @param {Record<string, any>} c
 * @returns {string}
 */
function draftPrompt(c) {
  const p = c.page || {};
  if (c.round === 1 && !c.findings) return wholeDraft(p);
  return editDraft(c, p);
}

/**
 * The closing paragraph every read shares: what a finding holds and how the verdict follows.
 * @returns {string}
 */
function findingRule() {
  return `A finding names its location (the section heading, and the sentence when it applies),
what is wrong, a rewrite that fixes it, and whether it blocks. Verdict "fix" if any blocking finding
exists; otherwise "ok".`;
}

/**
 * The note that narrows a review seat's read: to a section redraft's changed sections at rounds 1
 * and 2, to its own round-1 findings at round 2, and to the final redraft's spots at round final.
 * Empty for a round-1 read of a whole draft.
 * @param {Record<string, any>} c
 * @returns {string}
 */
function narrowing(c) {
  if (c.round === "final") {
    return c.scope
      ? `

This read is scoped to the final redraft. The page changed only at the spots the final reader's
findings cite, and those findings follow, as the redraft answered them. Grade the changed
sentences, and leave the rest of the page as the earlier reads graded it.

${c.scope}`
      : "";
  }
  let out = "";
  if (c.scope) {
    out += `

This read is narrowed to a section redraft. The sections ${idList(c.scope)} changed, and the rest
of the page stands as it was accepted. Read the whole page for context, and grade the changed
sections and how each sits with the sections around it. A defect outside them that the change
didn't cause is advisory: set blocking to false and start its finding with "Already present:".`;
  }
  const own = c.narrow && c.narrow.own;
  if (c.round === 2 && own) {
    const fs = (own.findings || [])
      .map((f) => `- [${f.blocking ? "blocking" : "advisory"}] ${f.location}: ${f.finding}`)
      .join("\n");
    const task = own.verdict === "fix"
      ? `Verify each of your blocking round-1 findings below on the page as it stands: fixed, or still
open and still blocking. Then read the page for a defect a fix introduced.`
      : `You returned "ok" in round 1. Read the page for a defect the redraft introduced.`;
    out += `

This is a round-2 read, after a redraft that answered the round-1 findings. ${task} A passage the
redraft didn't change stays as round 1 graded it.${fs ? `

Your round-1 findings:
${fs}` : ""}`;
  }
  return out;
}

/**
 * The structural edit: the page against its blueprint, its anatomy, and two published checklists.
 * @param {Record<string, any>} c
 * @returns {string}
 */
function structurePrompt(c) {
  const p = c.page || {};
  return `Structural edit of ${p.path}. The page is inlined below with ${briefName(p)}, the blueprint
it was drafted from, the blueprint's outline entry, and the facts the blueprint places or cites.
Your stance is an editor's: the Editorial Freelancers Association says "Developmental editors (also
called 'substantive,' 'structural,' or 'content' editors) deal with content, organization, and
genre considerations." Read the whole page against its blueprint and this checklist, return
findings, and never edit. Grade structure only: the editor seat grades the prose, and the fact
read grades the claims.${narrowing(c)}

The blueprint is the page's outline, written down (Google Technical Writing Two, "Organizing large
documents", ${GOOGLE_LARGE_DOCS}): "You might find it useful to think of an outline as the narrative
for your document." The page carries each blueprint section in the blueprint's order, under its
heading, opening on its \`opens\` sentence and carrying its items. A section that is missing or out
of order, or an item a section drops, is a blocking finding. Each \`why\` says why a section sits
where it does, and \`orderReason\` explains the order as a whole.

The anatomy. Read ${anatomyRead(anatomyFor(p))}, for the introduction and the closing section it
requires for this page's type. They are the second half of the same check as the introduction
items below.

Block 1, the Red Hat peer review guide's "Structure Checklist"
(https://redhat-documentation.github.io/peer-review/), the items that apply:
- Information is presented in the most logical order and location.
- Information is provided at the right pace.
- Cross-references are used appropriately and only when useful.
- The user goal is clear.
- Tasks reflect the intended goal of the user.
- Troubleshooting and error recognition steps are included where appropriate.

Block 2, Google Technical Writing Two, "Organizing large documents": "an introduction that includes
the following information: What the document covers. What prior knowledge you expect readers to
have. What the document doesn't cover." Compare the page's introduction with the outline entry's
\`job\`, \`covers\`, and \`outOfScope\`, and with the blueprint's \`framing.introPlan\`. The same
course asks: "Does your introduction provide an accurate overview of the topics you cover?"

A checklist item the page fails is a blocking finding. ${findingRule()}`;
}

/**
 * The copy edit: the page against the Google rubric and tells.
 * @param {Record<string, any>} c
 * @returns {string}
 */
function editorPrompt(c) {
  const p = c.page || {};
  return `Copy edit of ${p.path} against the Google developer documentation style guide. The page is
inlined below with ${briefName(p)}. Grade the page against the rules in
\`drafter:google-rubric.md\` and the patterns in \`drafter:google-tells.md\`, both inlined below.
Each rule a sentence breaks, and each pattern a sentence shows, is a blocking finding; name the
rule or the pattern by its heading. A point of preference that neither file states is advisory,
and its finding starts "Nit:". Also apply the prose rules of ${anatomyRead(anatomyFor(p))}.${narrowing(c)}

A rewrite changes wording, never a claim. Keep each number, command, flag, path, and name the
sentence states, since the brief cites the facts behind them, and a rewrite that adds a specific
breaks the page's provenance. A heading is the blueprint's, verbatim, so a finding on a heading is
advisory: its fix goes in the blueprint. Leave structure to the structural edit and the claims to
the fact read.

${findingRule()}`;
}

/**
 * The fact read: the brief's citations against the facts and their source excerpts.
 * @param {Record<string, any>} c
 * @returns {string}
 */
function factPrompt(c) {
  const p = c.page || {};
  const store = c.facts
    ? `with the facts the blueprint places or cites and, after them, the source excerpt each fact
names. The brief maps each sentence of the page to the fact ids it rests on, or to "no-claim".`
    : `but no facts block, since the project keeps no facts store. The brief can then cite no fact, so
a sentence that states a checkable specific has nothing behind it.`;
  return `Fact read of ${p.path}. The page and ${briefName(p)} are inlined below, ${store}${narrowing(c)}

Check each of these. Each failure is a blocking finding, located at the sentence:
- Each id the brief cites is in the facts block, with the tag verified, external, or vendor.
- Each cited sentence states only what its facts state. Every number, version, date, path, URL,
  command, flag, and code name in it appears in a fact it cites, and the sentence claims no more
  than the fact does.
- Each cited fact still matches its source excerpt. When an excerpt no longer says what its fact
  claims, the finding names the fact id and quotes the excerpt.
- A "no-claim" sentence states no checkable specific. One that does needs the fact that states it.
- Each sentence of the page's prose is in the brief, and each brief sentence is on the page.
- Each \`fact\` item of the blueprint is cited by a sentence in its section, and no fact that the
  blueprint's \`dispositions\` cuts appears on the page.

Leave style to the editor seat and structure to the structural edit. ${findingRule()}`;
}

/**
 * The reader's first step, by page type: a paraphrase test for a concept, a walk-through for a
 * task or a tutorial, and one whole read otherwise.
 * @param {string} type
 * @returns {string}
 */
function readerFirstStep(type) {
  if (/concept/i.test(type)) {
    return `Part V's paraphrase test: "Ask the participant to read to a specific stopping point, known as
a cue. Each time the participant reaches a cue, ask the participant to tell you in his or her own
words what that section means." Each section's end is a cue: write in your own words what that
section means, and keep each paraphrase in summary.`;
  }
  if (/task|tutorial|how-to/i.test(type)) {
    return `DigitalOcean's test for a tutorial: "We ask that you thoroughly test it by reading through it and
following it as a reader would." Follow the page step by step. You can't run a command, so read
each step as the reader about to run it, and note each point where you had to infer something the
page doesn't say. Return each such point as one short entry in friction.`;
  }
  return "Read the page once, whole.";
}

/**
 * The reader: turn 1 reads the page cold and paraphrases it; turn 2 reads the job and returns the
 * read. Round final is the retest after the final redraft.
 * @param {Record<string, any>} c
 * @returns {string}
 */
function readerPrompt(c) {
  const p = c.page || {};
  const inputs = c.inputs || {};
  const retest = c.round === "final";
  if (c.turn === 2) {
    const job = String(p.job || inputs.job || "").trim();
    return `Step 3 of the reader read of ${p.path}. You have read the page and answered steps 1 and 2.

Only now, read the page's job: ${job}

Each mismatch between your paraphrase and that job is a blocking finding: "wherever participants
misunderstood the message, the document has a problem that you should fix" (Part V). Each point
where you had to infer is a finding, blocking when it is an unstated step, a failure the page never
explains, or a term it never defines.${retest ? ` This is the retest after the final redraft: judge
the page as it stands now, and a point the redraft fixed is no longer a finding.` : ""}

${findingRule()}`;
  }
  const who = p.reader
    ? `the one the page's track describes: ${String(p.reader).trim()}`
    : "a developer who arrives at this page.";
  const type = String(p.pageType || inputs.pageType || p.type || "");
  return `Reader read of ${p.path}. You are the reader, ${who} You have this page, inlined below, and
nothing else. You have no blueprint, no facts, and no other file.${retest ? `

The page was redrafted after an earlier reader's findings. Read it fresh, as a reader who hasn't
seen it.` : ""}

The method is the Federal Plain Language Guidelines, Part V, "Test", and you are its participant.

Step 1. ${readerFirstStep(type)}

Step 2. Answer Part V's open-ended questions:
  What would you do if you got this document?
  What do you think the writer was trying to do with this document?
  Thinking of other people you know who might get this document:
    What about the document might work well for them?
    What about the document might cause them problems?
Return your answer to the second question as paraphrase, and the other answers in summary. Keep
the paraphrase to two sentences and the summary to five sentences. Put every point that needs more
room in friction, one short entry each, so that summary stays short. Write each field as plain
text on one line, with no line breaks and no lists inside a string, and no double quote
characters: describe a quotation in words. Return only the JSON object. Each of paraphrase and
summary is one paragraph of plain sentences. Close each string, then the object.

Step 3 follows in the next turn.`;
}

/**
 * The coherence read: the whole page as one argument after a section redraft, with the changed
 * sections read against their neighbors first. Any round but 1 is the retest after its fix.
 * @param {Record<string, any>} c
 * @returns {string}
 */
function coherencePrompt(c) {
  const p = c.page || {};
  const retest = c.round !== 1
    ? `

This is the retest, after a fix that answered your earlier findings. Read the page as it stands,
for the same three things.`
    : "";
  return `Coherence read of ${p.path}. A section redraft changed the sections ${idList(c.scope)}, and the
rest of the page stands as it was accepted. The page is inlined below with ${briefName(p)}, the
blueprint, its outline entry, and the facts the blueprint places or cites.${retest}

Read the whole page as one argument, from the introduction to the close, against the blueprint's
\`argument\` and \`orderReason\`. Check three things:
- Transitions: each section leads into the next, and no lead-in promises what the next section no
  longer delivers.
- Repetition: no point is made twice across sections, as when a changed section restates what a
  neighbor already says.
- Flow: the argument builds in order, and no section relies on something only a later section
  explains.

Read each changed section against its neighbors first, the section before it and the section after
it, since a redraft that reads well alone can break the seams around it.
A finding may lie anywhere on the page, in a changed section or not: locate it where its fix
belongs.

Leave style, the anatomy, and the claims to the reads that have already passed the page. A finding
blocks when a reader reading straight through would lose the thread; otherwise it is advisory.

${findingRule()}`;
}

return {
  blueprint: (c) => {
    const path = c.page && c.page.path;
    const entry = PAGES.find((p) => p.path === path) || {};
    return blueprintPrompt(c.scope || undefined, anatomyFor(entry));
  },
  draft: draftPrompt,
  structure: structurePrompt,
  editor: editorPrompt,
  fact: factPrompt,
  reader: readerPrompt,
  coherence: coherencePrompt,
};

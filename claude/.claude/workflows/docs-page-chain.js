// Runs the draft-docs page chain (docs/superpowers/specs/2026-09-26-draft-docs-approach-design.md,
// "The page chain" and "Scoped re-review") over a list of pages, a few pages in flight at once.
//
// Per page: a page-inputs agent writes the page's job, its type, the fact ids the drafter should
// draw on, and a claim inventory with a disposition per claim
// (carried by a cited fact, newly filed, or cut with a reason), filing any new fact itself with
// the Edit tool as `[verified]` (a `Source:` line) or `[external]` (the vendor URL), never
// `[candidate]`. The plan step then writes the page plan, and the structural edit seat reads it
// before any prose (see "The page plan" below). The framing step then decides the page's
// introduction from its readers and writes the framing record (see "The framing step" below).
// The drafter (`cairn-docs-drafter` by default)
// writes the page and its sentence-to-fact brief, the introduction from the framing record and
// the body from the plan and that record, and files no fact
// of its own; it runs the docs gate itself as its last act and reports the result; it reads the
// page's exemplar sources whole. The structural edit seat, the register editor, and a fact read
// run in parallel, all Opus 5.5, plus a figure-verifier read for a page carrying a figure. One redraft
// round on the combined findings, gated and re-read; by default only the reviewer(s) that
// returned "fix" re-read (`args.bothReviewers` re-reads every reviewer instead and the record
// carries a cross-regression flag: a reviewer that accepted in round 1 and returned "fix" in
// round 2). Round 2's reads are narrowed to round-1 fixes and changed sentences (see "Round 2
// reads" below). A second "fix" from a re-reader escalates to the conductor; there is no third round.
// A page every read accepts then takes the final reader read, last. The conductor reads only the
// per-page records returned. Each record carries the files the run leaves for commit: the page
// (`path`), its brief (`brief`), its plan (`plan`), and its framing record (`framing`).
//
// Invoke by name from the conductor session:
//
//   Workflow({
//     scriptPath: "docs-page-chain",
//     args: {
//       worktree: "/home/glw907/Projects/cairn-cms/.claude/worktrees/<name>",
//       gate: "npm run check:docs-gate -- --page {page} --brief {brief}",   // {page}/{brief} substituted per page
//       gateLane: "light",                 // optional; "light" prefixes CAIRN_GATE_LANE=light
//       inFlight: 3,                       // optional; pages drafted at once, default 3
//       toolGate: "make -C <worktree>/tool check",   // optional; appended for a page with `pinned`;
//                                                     // defaults to `make -C <worktree>/tool check`
//       drafterType: "cairn-docs-drafter", // optional; the drafter's agent type (this is the default)
//       pageInputsType: "general-purpose", // optional; needs WebFetch and Edit (this is the default)
//       bothReviewers: false,              // optional; true re-reads every reviewer after a redraft
//       registerPath: "docs/internal/docs-register.md",   // optional; the register file, this is the default
//       valeErrorRules: "<the error-tier rule list, verbatim>",
//       drafterModel: "claude-opus-5-5",   // optional; default
//       reviewModel: "claude-opus-5-5",    // optional; default, also the page-inputs step's model
//       pageInputsModel: "claude-opus-5-5", // optional; defaults to reviewModel
//       planModel: "claude-opus-5-5",      // optional; the plan step's model, this is the default
//       planEffort: "xhigh",               // optional; the plan step's effort, this is the default
//       framingModel: "claude-opus-5-5",   // optional; the framing step's model, this is the default
//       framingEffort: "xhigh",            // optional; the framing step's effort, this is the default
//       outline: "docs/internal/outlines/extend.json",   // optional; see "The outline" below
//       optionMap: "docs/internal/option-map.json",       // optional; the option map, this is the default
//       pages: [
//         { id: "architecture", path: "docs/extend/architecture.md", track: "extend" },   // with an outline
//         {
//           id: "is-it-working",
//           path: "docs/admin/is-it-working.md",
//           track: "admin",                // editors | admin | extend | reference | front-door | readme;
//                                          // locates its brief file and its register sections
//           job: "<the page's job and type, from the stage outline, verbatim>",
//           pageType: "how-to",            // optional hint; the page-inputs agent may confirm or correct it
//           exemplarSources: ["docs/admin/x.md", "docs/editors/y.md"],  // full pages the drafter reads whole
//           inputs: ["docs/superpowers/plans/<plan>.mining.md#is-it-working", "..."],
//           pinned: ["#slug-one", "#slug-two"],   // optional; slugs the page must keep
//           extraChecks: ["<a sentence the fact read must also verify>"],   // optional
//           figure: false,                 // optional; true adds a figure-verifier read
//           factIds: ["f:abc123"],          // optional; page inputs' starting fact ids
//           covers: ["<a topic the page covers>"],        // optional
//           outOfScope: ["<a topic another page owns>"],   // optional
//           rework: "<job-read findings and owner rulings>"   // optional; see "Rework" below
//         }
//       ]
//     }
//   })
//
// The outline (`args.outline`): a stage outline JSON, repo-relative to `args.worktree`, carrying
// top-level `index` (the arm index, repo-relative), `groups: [{ id, title, pages }]`, and
// `pages: [{ slug, title, path, group, order, job, pageType, exemplars: [{ source, take }], figure,
// figureNote, factIds, covers, outOfScope, absorbs, pinned }]`. With it, each entry in
// `args.pages` needs only `id`, `path`, and `track`: its `title`, `job`, `pageType`, `exemplarSources`,
// `exemplarTakes`, `factIds`, `covers`, `outOfScope`, `absorbs`, `pinned`, `figure`, and
// `figureNote` come from the outline page whose `path` matches, and any of those fields given
// inline in `args.pages` overrides the outline's (inline `exemplarSources` alone drops the
// outline's take lines). An exemplar `source` is a capture directory under
// `~/.local/share/cairn/exemplars/`, read as the `page.md` inside it; its `take` line rides beside
// it in the drafter's and page inputs' prompts. The figure note reaches the drafter and the figure
// read. The fact read checks the claim inventory against the outline's fact ids and the absorbed
// pages' fact sections. A page path the outline does not carry fails the run by name before any
// page agent starts.
//
// The workflow runtime has no filesystem or exec access, so the outline is read by the
// `cairn-docs-outline` helper (`~/.local/bin`, tested by
// `~/.dotfiles/tests/docs-page-chain-outline.test.mjs`): one probe agent runs its `resolve`
// command before any page agent and returns the resolved entries, each with a checksum the runner
// recomputes, plus a report checksum over `ok`, `index`, `missing`, and `error`, so a probe that
// mistranscribes an entry or its report fails the run rather than drafting from it.
//
// The index link: with an outline naming an `index`, the drafter runs the helper's `link` command
// after it writes the page and before its gate, every round. The helper adds one relative link to
// the page, its text the page's H1, under the heading whose text is the page's group title, in outline order, and reports
// a link already present instead of adding a second one. Its read-insert-write holds a lock keyed
// by the index path, which serializes the link-in across in-flight pages. The link-in runs inside
// the drafter because the drafter runs its own gate as its last act, and the index must link the
// page before `check:arm-indexes` sees it; a runner step before the drafter would link a page not
// yet on disk, and one after would follow the gate.
//
// The option map (`args.optionMap`, default `docs/internal/option-map.json`, repo-relative): one
// row per public option path, each a fact id, `exclude <reason>`, or `pending <slug>`, under a
// top-level `pendingCount` constant that `check:options` holds the pending rows to. A page's slug
// is its file's base name (every outline slug is). Page inputs receives the map path, the slug,
// the page's fact ids, and the selection rule (the `pending <slug>` rows, and the rows whose fact
// id is among the page's), reads its rows live, and disposes each pending row in its claim
// inventory as carried, filed, cut (the row becomes `exclude <reason>`), or re-pointed to another
// outline page not yet drafted. A disposal files the fact, then rewrites the row with the Edit
// tool, then lowers the constant; a stale-read failure means another page wrote the map, so the
// agent re-reads and redoes the edit. A retag that takes a mapped fact off `[verified]` (page
// inputs to `[candidate]`, the fact read to `[docs-drift]`) raises the constant once by the
// number of the fact's rows, then rewrites all of them to `pending <slug>`, then retags the fact
// once. In every round the fact read reports as blocking each row the live map still holds
// `pending <this page's slug>`, its own retags' rows included, so an accepted page leaves none. Page inputs reports `rowsReceived` and `rowsDisposed`, carried in
// `record.pageInputs`.
//
// The page plan (`docs/superpowers/research/2026-10-01-draft-docs-2a-page-plan-diagnosis.md`):
// the chain carried no artifact that holds a page's argument, so one plan step runs after page
// inputs (or a rework page's inventory) and before the draft, on `args.planModel` at
// `args.planEffort` (default Opus 5.5 at `xhigh`, the chain's one judgment seat). It writes
// `docs/internal/briefs/<track>/<slug>.plan.md`, Google's outline written down (Google Technical
// Writing Two, "Organizing large documents": "think of an outline as the narrative for your
// document"), from the job, page type, register anatomy, exemplar takes, fact ids, and claim
// inventory: the introduction's three parts, the sections in the order it argues for (each with
// its heading, the one sentence a reader takes from it, the fact ids it draws on, and its
// hand-off), and the ending section the anatomy requires. Every fact id the inventory carries is
// placed in a section, subordinated (a link to the reference page or entry that states it, named),
// or cut with a reason; a subordinated fact whose reference target does not state it is a
// `couldNotDo` naming the reference page and a reference-arm friction entry, never kept on the
// page. The runner writes those dispositions back into the claim inventory the drafter and fact
// read receive, as "carried" with a `section` or "cut" with the reason (a subordinated fact's
// reason names its link), adds every id the plan places to the drafter's fact ids, and escalates
// a plan that leaves an id undisposed. The structural edit
// seat reads the plan before any prose (`structure:<id>:plan`), grading order, pace, user goal,
// and the introduction's three parts at the plan level against the outline entry; its "fix" takes
// one plan revision (`replan:<id>`) and one re-read, and a second "fix" escalates. The record
// carries the plan's reads, `couldNotDo`, and friction in `record.planStep`.
//
// The framing step (owner ruling, Geoff, 2026-10-04, confirmed 2026-10-05 after the intro-only
// round, proven by hand in `9ca04531` on `draft-docs-2a`, whose six
// `docs/internal/briefs/extend/<page>.framing.md` records are the exemplars of its output): a
// page's introduction is not drafted like its body. The fact-driven chain writes strong bodies
// and thin introductions, and an introduction takes high-level reasoning to get its framing and
// background right. So one framing agent per page runs after the plan step and its plan read and
// before the drafter, on `args.framingModel` at `args.framingEffort` (default `claude-opus-5-5` at `xhigh`,
// set per agent through the runtime's `effort` option). It sits after the plan because it frames
// the plan's introduction (the plan's three Google parts stand) and reads the plan's dispositions,
// and before the drafter because the drafter writes the introduction from it. It reads the doc
// set's map: with `args.outline`, the stage outline file whole (`groups` with their titles, every
// page's `slug`, `title`, `job`, `covers`, and `outOfScope`, and `crossLinks`, the one input the
// chain holds that lists every page and its job) plus the arm index the outline names and the
// other arms' indexes under `docs/`; without one, the run's own page list (each path and job) in
// its prompt, plus those indexes. It also reads the register's "### The introduction" section,
// the page anatomies, the page plan, and the facts it leans on. Starting from the reader (who
// arrives at the page, from where, and what each is looking for, since a page can serve several
// readers and the introduction names each reason), it decides the background (the general model,
// where SvelteKit, Cloudflare, and GitHub fit when the page touches them, and why the thing the
// page covers exists), the page's place in the doc set, and an intro plan of numbered paragraphs:
// two or three when the framing needs them, opening on a statement, never an imperative and
// never a sentence about the page. Each page's framing is reasoned fresh, never from a template.
// It writes `docs/internal/briefs/<track>/<slug>.framing.md`, beside the brief, in the exemplars'
// shape: "Who arrives, from where, and why", "Background the page rests on" (fact ids), "Place in
// the doc set", "Intro plan", and "Departures from the plan's introduction" when it departs. A
// background point with no fact is filed `[verified]` or `[external]` with the Edit tool, as page
// inputs files, or reported as `couldNotDo`. The runner adds every fact id the framing names to
// the drafter's fact ids and to the claim inventory as "carried" in section "Introduction" (an
// id the plan cut is re-placed there; one already carried keeps its section), so the fact read
// holds the introduction to it. The drafter writes the introduction from the record and the body
// from the plan; the structural edit seat and the register editor read the record and grade the
// introduction against it (a reader's reason missing, background dropped, an undeclared departure
// from its intro plan, an imperative or page-describing opening are blocking). A rework page takes
// the framing step too, revising any record already on disk. The record path is `record.framing`;
// the step's readers, fact ids, filed facts, `couldNotDo`, and friction land in
// `record.framingStep`. A framing agent that returns nothing escalates the page, and so does one
// whose record a probe (`probe:<id>:framing`, `test -f`) finds missing on disk at its reported path.
//
// Round 2 reads (draft-docs 2a run 2 measured about 2.4M tokens a page against the 1.3M budgeted;
// `docs/superpowers/research/2026-10-03-draft-docs-2a-targeted-close-record.md` on
// `draft-docs-2a`): the round-2 redrafter runs `git hash-object -w -- <page>` before any edit and
// reports the blob as `baseline`, the page as round 1 read it. Each round-2 seat then receives its
// own round-1 findings and the command that prints the diff since round 1
// (`git cat-file blob <baseline> | diff -u - <page>`), verifies each of its round-1 findings'
// fixes, and reads only the changed sentences (deleted lines included) for new defects. Round 2
// no longer re-reads the whole page, no longer re-grades unchanged sentences, and no longer
// re-runs a rework page's `git diff` scope against HEAD. The fact read still reads the option map
// live for rows pending the page's slug. Under `bothReviewers`, a seat that accepted in round 1
// reads the changed sentences for a defect the fixes introduced, so `crossRegression` still
// measures a fix's regression on the narrowed input. A `baseline` must be exactly 40 or 64 hex
// digits and resolve in the repo (a probe, `probe:<id>:baseline`, runs `git cat-file -e`); a
// redraft whose baseline fails either test falls back to whole-page round-2 reads, logged, and
// `rounds[1].scope` records which ran.
//
// Design friction: page inputs, the plan step, the framing step, the drafter, and the fact read,
// the agents that meet the code, file a friction entry in `docs/internal/docs-friction-log.md` for a hedge, a
// caveat, an exception, a workaround, a surprising default, or two seams naming or behaving the
// same thing differently, naming the fact ids or `file:line` involved, and list it in
// `frictionFiled`. An entry never blocks the page. The record carries page inputs' list in
// `record.pageInputs`, the plan step's in `record.planStep`, the framing step's in
// `record.framingStep`, the drafter's in `rounds[].draft`, and the fact read's, copied by the runner, on its entry in
// `rounds[].reads`; the register editor and the figure verifier do not report friction.
//
// The drafter gets the outline entry's `title` as the page's H1 (the title is in the entry
// checksum), the page plan's path as the source of the page's order, each section's claim, and
// each fact's placement, the framing record's path as the source of the introduction, the voice source (the register's drafting brief and its primary exemplar
// only; the page's exemplars give structure and detail per step), and, on a figure page, the
// `cairn-figure` skill file to read and follow.
//
// The structural edit seat (`structural edit` in the record, `general-purpose`, read-only by its
// prompt) runs the structural-edit checklist that
// `docs/superpowers/research/2026-09-30-page-level-review-prior-art.md` adopts in "## b.
// Structural edit seat": the Red Hat peer review guide's Structure Checklist, less its two
// AsciiDoc and reuse items, and Google Technical Writing Two's introduction, review, navigation,
// and heading lead-in items, all quoted verbatim, with the introduction checked on every page.
// Google's navigation item "a table of contents menu" is left out as inapplicable, the way Red
// Hat's modular-docs items are: the site renderer (cairn.pub) supplies the table of contents,
// not the page. Its plan read comes first (see "The page plan" above). Its page read in each
// round reads the whole page against its plan and the checklist, no longer against the outline's
// `covers` order, since the plan carries the order and its reason; it still reads the outline
// entry (`job`, `covers`, `outOfScope`, and the outline's `crossLinks` from the page's slug, read
// live) and the register's page anatomies for the introduction, and never edits. Its findings join the round's like the other reads', and `bothReviewers`
// re-reads it. The register editor's own definition carries line-level grading, which the
// record's ordering (line editing after structure) keeps out of this seat, so it reuses
// `general-purpose` as the fact read does rather than a new agent file.
//
// The final reader read (`record.finalRead`, `general-purpose`) runs once per page, after every
// read has accepted, in round 1 or round 2; an escalated page never takes it. It follows the same
// record's "## f. The final reader read": the Federal Plain Language Guidelines' Part V, with the
// agent as the participant. It reads only the page: a concept page takes Part V's paraphrase
// test, a task or tutorial page is followed as a reader would (DigitalOcean), and every page
// answers Part V's open-ended questions, "What do you think the writer was trying to do with
// this document?" returned as `paraphrase`. Only then does it read the page's `job` from the
// outline file (with no outline, the job rides in its prompt's last step) and report each
// mismatch and each point where it had to infer. Its "fix" sends the page back for one redraft
// of the cited spots, which takes the register editor's and the fact read's reads scoped to the
// changed sentences (the parent spec's "Edits after the chain"); a "fix" or a red gate there
// escalates. When those reads accept on a green gate, the reader reads once more, a re-test, per
// Part V's iteration rule: "Test, make corrections based on feedback, and test again. Plan to
// test at least twice." The re-test's "accept" accepts the page; its "fix" escalates, with no
// further redraft or read. Its verdict, summary, paraphrase, and findings, and any redraft,
// scoped reads, and re-test (`retest`), land in `record.finalRead`, never in `rounds`.
//
// The reader's prompt confines any step it runs to a scratch directory outside the worktree and
// leaves a step needing an account, a secret, or a deploy unrun. That is an operational safety
// constraint resting on the runner's "modify no file" pattern (the outline probe's), not a docs
// rule.
//
// Rework (`rework` on a page): the plan's task 7b
// (`docs/superpowers/plans/2026-09-30-draft-docs-stage-2a.md`, "Task 7b: rework the pilot at the
// page level") reworks committed pages at the page level, a sentence kept where the page plan keeps
// it, and takes them through the new seats. A page carrying `rework`, a string of that page's
// job-read findings and owner rulings, skips page inputs and the round-1 draft: its committed page
// and brief are the draft. It takes the plan step and plan read like any page, its `rework` text in
// the plan's prompt, before its first drafter call. Its first drafter call is the round-2 redraft
// prompt (label `rework:`), with the `rework` text as its findings and the scope line "page-level
// only (introduction, section order, hand-offs, depth, ending, covers); the page plan governs
// order, placement, and cuts, and a sentence is kept where the plan keeps it; follow the register's
// page anatomies." Before any page agent, one probe runs the helper's `rework-state` command
// (checksum-verified, like the outline probe) for HEAD, each rework page's git state, and the fact
// ids its committed brief cites. A rework page that is untracked or has uncommitted changes is not
// run: its record escalates with that reason. Page inputs is replaced by the outline's job and type
// and an inventory built from the brief: an id the brief cites is carried, and an outline id it
// does not cite is cut "cut at the pilot draft (brief at <HEAD>)", so the fact read's coverage rule
// has a baseline and raises nothing for it. That draft takes the round-1 reads, then the existing
// round flow, `bothReviewers`, and the final reader read with its re-test. Its register and fact
// reads, in both rounds, carry the scope note naming the changed sentences by `git diff -- <page>`,
// as the final-read path's scoped reads do, and return "fix" when that diff is empty, since the
// rework then changed nothing; the structural edit seat reads the whole page unscoped. A round-2
// redraft keeps the scope line. The record carries `rework: true` and no `pageInputs`. A page
// without `rework` runs exactly as before.
//
// The drafter writes the introduction, the section hand-off lead-ins, and the ending the
// register's page anatomies require, records each `no-claim` in the brief when it carries no
// extractable fact, and cites the fact id when it does. The removal rule keeps a sentence that
// carries a section's claim from the plan, as it keeps the anatomy's. A brief sentence's `id` is
// one fact id, an array of the ids a synthesizing sentence cites, or `no-claim`, and the brief
// carries a top-level `cuts` array of `{ id, reason }` mirroring the plan's cut dispositions.
//
// The return carries `spent`: the runtime `budget.spent()` delta across the run, in its unit of
// output tokens spent across the main loop and all workflows, so a relative measure only and
// never a count against a pass ceiling. It is null when the runtime supplies no `budget`.
//
// Every agent starts with zero context. The runner renders each stage's prompt from args and
// the page record; nothing load-bearing may live only in the conductor's conversation.

export const meta = {
  name: "docs-page-chain",
  description: "Drafts docs pages through page inputs, plan, intro framing, drafter, gate, register editor, fact read, and one scoped redraft.",
  whenToUse: "A draft-docs pass plan names this workflow for its page tasks.",
  phases: [
    { title: "Outline", detail: "with args.outline, one probe resolves every page's outline entry" },
    { title: "Page inputs", detail: "one agent per page: job, type, fact ids, claim inventory" },
    { title: "Plan", detail: "one agent writes the page plan; the structural edit seat reads it before any prose, one revision on fix" },
    { title: "Framing", detail: "one agent reads the doc set's map and writes the framing record the introduction is drafted from", model: "claude-opus-5-5" },
    { title: "Draft", detail: "the drafter writes the introduction from the framing record, the body from the plan, and runs the docs gate itself" },
    { title: "Read", detail: "structural edit, register editor, and fact read in parallel, all Opus, plus a figure read when the page carries one" },
    { title: "Redraft", detail: "one round on the combined findings; re-readers verify their round-1 fixes and read changed sentences only" },
    { title: "Final read", detail: "one reader read on an accepted page; its fix takes one scoped redraft, scoped reads, and one re-test" },
    { title: "Report", detail: "per-page records for the conductor" }
  ]
};

// `section` is set only on a "carried" claim the page plan places: the heading it sits under.
const CLAIM = {
  type: "object",
  properties: {
    claim: { type: "string" },
    disposition: { type: "string", enum: ["carried", "filed", "cut", "re-pointed"] },
    factId: { type: "string" },
    reason: { type: "string" },
    section: { type: "string" }
  },
  required: ["claim", "disposition"]
};

// The plan step's report: the plan's path and its dispositions, in the existing dispositions only.
const PLAN_SCHEMA = {
  type: "object",
  properties: {
    plan: { type: "string" },
    claimInventory: {
      type: "array",
      items: { ...CLAIM, properties: { ...CLAIM.properties, disposition: { type: "string", enum: ["carried", "cut"] } }, required: ["claim", "disposition", "factId"] }
    },
    couldNotDo: { type: "array", items: { type: "string" } },
    frictionFiled: { type: "array", items: { type: "string" } }
  },
  required: ["plan", "claimInventory"]
};

// The framing step's report: the record's path, one line per reader it frames for, and the fact
// ids the introduction draws on, filed ones included.
const FRAMING_SCHEMA = {
  type: "object",
  properties: {
    framing: { type: "string" },
    readers: { type: "array", items: { type: "string" } },
    factIds: { type: "array", items: { type: "string" } },
    factsFiled: { type: "array", items: { type: "string" } },
    couldNotDo: { type: "array", items: { type: "string" } },
    frictionFiled: { type: "array", items: { type: "string" } }
  },
  required: ["framing", "readers", "factIds"]
};

// An existence probe's report: whether the command it ran exited zero.
const EXISTS_SCHEMA = {
  type: "object",
  properties: { exists: { type: "boolean" }, output: { type: "string" } },
  required: ["exists"]
};

// One option-map row as read: its path key and its value, verbatim.
const MAP_ROW = {
  type: "object",
  properties: { key: { type: "string" }, value: { type: "string" } },
  required: ["key", "value"]
};

const PAGE_INPUTS_SCHEMA = {
  type: "object",
  properties: {
    job: { type: "string" },
    pageType: { type: "string" },
    factIds: { type: "array", items: { type: "string" } },
    addedFactIds: {
      type: "array",
      items: {
        type: "object",
        properties: { factId: { type: "string" }, why: { type: "string" } },
        required: ["factId", "why"]
      }
    },
    claimInventory: { type: "array", items: CLAIM },
    factsFiled: { type: "array", items: { type: "string" } },
    rowsReceived: { type: "array", items: MAP_ROW },
    rowsDisposed: {
      type: "array",
      items: {
        type: "object",
        properties: {
          key: { type: "string" },
          disposition: { type: "string", enum: ["carried", "filed", "cut", "re-pointed", "reopened"] },
          value: { type: "string" },
          reason: { type: "string" }
        },
        required: ["key", "disposition", "value"]
      }
    },
    frictionFiled: { type: "array", items: { type: "string" } },
    couldNotDo: { type: "array", items: { type: "string" } }
  },
  required: ["job", "pageType", "factIds", "claimInventory", "rowsReceived", "rowsDisposed"]
};

const DRAFT_SCHEMA = {
  type: "object",
  properties: {
    path: { type: "string" },
    gate: { type: "string", enum: ["pass", "fail", "not run"] },
    gateCommand: { type: "string" },
    gateTail: { type: "string" },
    indexLink: { type: "string" },
    baseline: { type: "string" },
    frictionFiled: { type: "array", items: { type: "string" } },
    couldNotDo: { type: "array", items: { type: "string" } }
  },
  required: ["path", "gate", "gateCommand"]
};

const FINDING = {
  type: "object",
  properties: {
    location: { type: "string" },
    finding: { type: "string" },
    rewrite: { type: "string" },
    blocking: { type: "boolean" }
  },
  required: ["location", "finding", "blocking"]
};

const OUTLINE_ENTRY = {
  type: "object",
  properties: {
    path: { type: "string" },
    title: { type: "string" },
    job: { type: "string" },
    pageType: { type: "string" },
    exemplarSources: { type: "array", items: { type: "string" } },
    exemplarTakes: { type: "array", items: { type: "string" } },
    factIds: { type: "array", items: { type: "string" } },
    covers: { type: "array", items: { type: "string" } },
    outOfScope: { type: "array", items: { type: "string" } },
    absorbs: { type: "array", items: { type: "string" } },
    pinned: { type: "array", items: { type: "string" } },
    figure: { type: "boolean" },
    figureNote: { type: "string" },
    checksum: { type: "string" }
  },
  required: ["path", "title", "job", "pageType", "exemplarSources", "exemplarTakes", "factIds", "covers", "outOfScope", "absorbs", "pinned", "figure", "figureNote", "checksum"]
};

const OUTLINE_PROBE_SCHEMA = {
  type: "object",
  properties: {
    ok: { type: "boolean" },
    index: { type: "string" },
    missing: { type: "array", items: { type: "string" } },
    entries: { type: "array", items: OUTLINE_ENTRY },
    error: { type: "string" },
    checksum: { type: "string" }
  },
  required: ["ok", "index", "missing", "entries", "checksum"]
};

const READ_SCHEMA = {
  type: "object",
  properties: {
    verdict: { type: "string", enum: ["accept", "fix"] },
    findings: { type: "array", items: FINDING },
    summary: { type: "string" },
    frictionFiled: { type: "array", items: { type: "string" } }
  },
  required: ["verdict", "findings", "summary"]
};

// The helper's rework-state report, as the probe returns it.
const REWORK_STATE_SCHEMA = {
  type: "object",
  properties: {
    ok: { type: "boolean" },
    head: { type: "string" },
    pages: {
      type: "array",
      items: {
        type: "object",
        properties: {
          path: { type: "string" },
          brief: { type: "string" },
          state: { type: "string" },
          cited: { type: "array", items: { type: "string" } },
          error: { type: "string" }
        },
        required: ["path", "brief", "state", "cited"]
      }
    },
    error: { type: "string" },
    checksum: { type: "string" }
  },
  required: ["ok", "head", "pages", "checksum"]
};

// The final reader read's report: a read's verdict and findings, plus its answer to Part V's
// "What do you think the writer was trying to do with this document?"
const READER_SCHEMA = {
  type: "object",
  properties: {
    verdict: READ_SCHEMA.properties.verdict,
    findings: READ_SCHEMA.properties.findings,
    summary: { type: "string" },
    paraphrase: { type: "string" }
  },
  required: ["verdict", "findings", "summary", "paraphrase"]
};

const a = args || {};
const WT = a.worktree;
const GATE = a.gate;
const LANE = a.gateLane === "light" ? "CAIRN_GATE_LANE=light " : "";
const IN_FLIGHT = Number.isInteger(a.inFlight) && a.inFlight > 0 ? a.inFlight : 3;
const DRAFTER = a.drafterModel || "claude-opus-5-5";
const DRAFTER_TYPE = a.drafterType || "cairn-docs-drafter";
const TOOL_GATE = a.toolGate || `make -C ${WT}/tool check`;   // appended for a page carrying `pinned`
const REVIEWER = a.reviewModel || "claude-opus-5-5";
const PAGE_INPUTS_TYPE = a.pageInputsType || "general-purpose";
const PAGE_INPUTS_MODEL = a.pageInputsModel || REVIEWER;
// The plan step is the chain's one judgment seat, so it defaults to the strongest seat.
const PLAN_MODEL = a.planModel || "claude-opus-5-5";
const PLAN_EFFORT = a.planEffort || "xhigh";
// The framing step decides an introduction's framing, which takes high-level reasoning the
// fact-driven seats do not (Geoff, 2026-10-04), so it too runs at the top tier.
const FRAMING_MODEL = a.framingModel || "claude-opus-5-5";
const FRAMING_EFFORT = a.framingEffort || "xhigh";
const BOTH_REVIEWERS = a.bothReviewers === true;
const PAGES_ARG = a.pages || [];
const OUTLINE = a.outline || "";
const OPTION_MAP = a.optionMap || "docs/internal/option-map.json";
// The runtime's budget is absent outside it (a dry run), so `spent` is guarded, not assumed.
const HAS_BUDGET = typeof budget !== "undefined" && budget !== null && typeof budget.spent === "function";
const SPENT_AT_START = HAS_BUDGET ? budget.spent() : 0;

if (!WT || !GATE || !PAGES_ARG.length) {
  throw new Error("docs-page-chain needs args.worktree, args.gate, and args.pages");
}

const REGISTER = a.registerPath || "docs/internal/docs-register.md";

// === REGISTER SECTIONS (docs-page-chain-derivation.test.mjs extracts this block) ===
/**
 * The register sections one page's agent reads from the register file, each named by its exact
 * heading: the track's drafting brief, the shared Names, Visuals, and page-anatomy sections, and
 * the track's own section. The editor also reads the recorded deviations from the base guides
 * on every track, since a reviewer tells a recorded departure from a defect by them. Pure and
 * self-contained so the test can extract it verbatim without the workflow runtime.
 * @param {string} track - editors | admin | extend | reference | front-door | readme
 * @param {"drafter" | "editor"} role
 * @returns {string[]}
 * @throws {Error} on a track with no register track section
 */
function registerSectionsFor(track, role) {
  const frontDoor = "## The front door (`docs/README.md`, `docs/why-cairn.md`, and the root `README.md`)";
  const trackHeadings = {
    editors: "### The editor track (`docs/editors/`)",
    admin: "### The admin track (`docs/admin/`)",
    extend: "### The extend track (`docs/extend/`)",
    reference: "## The reference (`docs/reference/`), a shared instrument",
    "front-door": frontDoor,
    readme: frontDoor
  };
  if (!Object.hasOwn(trackHeadings, track)) throw new Error(`unknown track "${track}": no register track section`);
  const brief = track === "editors" ? "## Drafting brief: editor docs" : "## Drafting brief: developer docs";
  const sections = [
    brief,
    "## Names",
    "## Visuals (every page that carries one)",
    "## The page anatomies",
    trackHeadings[track]
  ];
  if (role === "editor") sections.push("## Deviations from the base guides");
  return sections;
}
// === END REGISTER SECTIONS ===

// Fail on an unknown track before any agent runs.
for (const p of PAGES_ARG) registerSectionsFor(p.track, "drafter");

// === OUTLINE MERGE (docs-page-chain-outline.test.mjs extracts this block) ===
/**
 * The fields the runner reads from an outline entry, in a fixed key order with absent values
 * normalized. Identical to the copy in the `cairn-docs-outline` helper, which the outline test
 * pins, so both sides hash the same string for the same entry.
 * @param {Record<string, unknown>} e
 * @returns {object}
 */
function canonicalEntry(e) {
  const list = (v) => (Array.isArray(v) ? v.map(String) : []);
  return {
    path: String(e.path || ""),
    title: String(e.title || ""),
    job: String(e.job || ""),
    pageType: String(e.pageType || ""),
    exemplarSources: list(e.exemplarSources),
    exemplarTakes: list(e.exemplarTakes),
    factIds: list(e.factIds),
    covers: list(e.covers),
    outOfScope: list(e.outOfScope),
    absorbs: list(e.absorbs),
    pinned: list(e.pinned),
    figure: e.figure === true,
    figureNote: String(e.figureNote || "")
  };
}

/**
 * FNV-1a over a string, as eight hex digits.
 * @param {string} s
 * @returns {string}
 */
function fnv(s) {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return (h >>> 0).toString(16).padStart(8, "0");
}

/**
 * The checksum of one entry's canonical fields.
 * @param {Record<string, unknown>} e
 * @returns {string}
 */
function checksumOf(e) {
  return fnv(JSON.stringify(canonicalEntry(e)));
}

/**
 * The checksum of a whole probe report: its status, index, missing paths, error, and each
 * entry's own checksum, so a report that drops the index or flips `ok` fails verification.
 * @param {{ ok?: boolean, index?: string, missing?: string[], error?: string, entries?: Array<{ checksum?: string }> }} r
 * @returns {string}
 */
function probeChecksum(r) {
  return fnv(JSON.stringify({
    ok: r.ok === true,
    index: String(r.index || ""),
    missing: Array.isArray(r.missing) ? r.missing.map(String) : [],
    error: String(r.error || ""),
    entries: (r.entries || []).map((e) => String(e.checksum || ""))
  }));
}

/**
 * Checks the outline probe's report against the requested pages and merges each page's entry
 * under its inline fields: a field `args.pages` gives inline wins over the outline's. Inline
 * `exemplarSources` without inline `exemplarTakes` drops the outline's take lines, which describe
 * the outline's exemplars, not the inline ones.
 * @param {Array<Record<string, any>>} pages - args.pages
 * @param {{ ok?: boolean, missing?: string[], entries?: Array<Record<string, any>>, error?: string } | null} probe
 * @param {string} outlinePath - args.outline, for the error message
 * @returns {{ pages: Array<Record<string, any>>, index: string }} the merged pages and the verified
 *   outline index
 * @throws {Error} naming every page path the outline does not carry, a report or an entry whose
 *   checksum does not match its fields, or a failed probe
 */
function mergeOutline(pages, probe, outlinePath) {
  if (!probe) throw new Error(`docs-page-chain: the outline probe for ${outlinePath} returned nothing`);
  const entries = probe.entries || [];
  const missing = pages.map((p) => p.path).filter((path) => !entries.some((e) => e.path === path));
  for (const path of probe.missing || []) if (!missing.includes(path)) missing.push(path);
  if (missing.length) throw new Error(`docs-page-chain: not in the outline ${outlinePath}: ${missing.join(", ")}`);
  if (probeChecksum(probe) !== probe.checksum) {
    throw new Error(`docs-page-chain: the outline probe mistranscribed its report (checksum ${probeChecksum(probe)}, reported ${probe.checksum})`);
  }
  if (!probe.ok) throw new Error(`docs-page-chain: the outline probe failed: ${probe.error || "no error given"}`);
  const fields = ["title", "job", "pageType", "exemplarSources", "exemplarTakes", "factIds", "covers", "outOfScope", "absorbs", "pinned", "figure", "figureNote"];
  const merged = pages.map((p) => {
    const entry = entries.find((e) => e.path === p.path);
    if (checksumOf(entry) !== entry.checksum) {
      throw new Error(`docs-page-chain: the outline probe mistranscribed ${p.path} (checksum ${checksumOf(entry)}, reported ${entry.checksum})`);
    }
    const out = { ...p };
    if (p.exemplarSources !== undefined && p.exemplarTakes === undefined) out.exemplarTakes = [];
    for (const f of fields) if (out[f] === undefined) out[f] = canonicalEntry(entry)[f];
    return out;
  });
  return { pages: merged, index: String(probe.index || "") };
}
/**
 * The checksum of a rework-state report: its status, HEAD, error, and every page field, in a
 * fixed order.
 * @param {{ ok?: boolean, head?: string, error?: string, pages?: Array<{ path?: string, brief?: string, state?: string, cited?: string[], error?: string }> }} r
 * @returns {string}
 */
function reworkChecksum(r) {
  return fnv(JSON.stringify({
    ok: r.ok === true,
    head: String(r.head || ""),
    error: String(r.error || ""),
    pages: (r.pages || []).map((x) => [String(x.path || ""), String(x.brief || ""), String(x.state || ""), Array.isArray(x.cited) ? x.cited.map(String) : [], String(x.error || "")])
  }));
}

// === END OUTLINE MERGE ===

/**
 * Resolves the page list: with `args.outline`, one probe agent runs the helper's `resolve` and
 * the runner merges and verifies its report; without one, `args.pages` as given.
 * @returns {Promise<{ pages: Array<Record<string, any>>, index: string }>}
 */
async function resolvePages() {
  if (!OUTLINE) return { pages: PAGES_ARG, index: "" };
  phase("Outline");
  const quoted = PAGES_ARG.map((p) => `'${p.path}'`).join(" ");
  const probe = await agent(
    `Run exactly this command, once, from any directory:

  cairn-docs-outline resolve --worktree '${WT}' --outline '${OUTLINE}' --paths ${quoted}

It prints one JSON object on stdout, whatever its exit status. Return that object as your
structured output with every field and every array element copied verbatim, character for
character, in the same order: a changed, dropped, or reordered string fails the run. If it prints
nothing on stdout, return ok: false, missing: [], entries: [], and error: the text it printed on
stderr. Run no other command and modify no file.`,
    { label: "outline", phase: "Outline", schema: OUTLINE_PROBE_SCHEMA, model: "sonnet", effort: "low" }
  );
  return mergeOutline(PAGES_ARG, probe, OUTLINE);
}

const { pages: PAGES, index: INDEX } = await resolvePages();

/**
 * With any page carrying `rework`, one probe agent runs the helper's `rework-state` command and
 * the runner verifies its report: HEAD, each rework page's git state, and the fact ids its brief
 * cites. Null when no page carries `rework`.
 * @returns {Promise<{ head: string, pages: Array<{ path: string, state: string, cited: string[], error?: string }> } | null>}
 * @throws {Error} on a mistranscribed report or an unresolved HEAD
 */
async function reworkProbe() {
  const pages = PAGES.filter((p) => typeof p.rework === "string" && p.rework !== "");
  if (!pages.length) return null;
  const quote = (xs) => xs.map((x) => `'${x}'`).join(" ");
  const report = await agent(
    `Run exactly this command, once, from any directory:

  cairn-docs-outline rework-state --worktree '${WT}' --paths ${quote(pages.map((p) => p.path))} --briefs ${quote(pages.map(briefPathFor))}

It prints one JSON object on stdout, whatever its exit status. Return that object as your
structured output with every field and every array element copied verbatim, character for
character, in the same order: a changed, dropped, or reordered string fails the run. If it prints
nothing on stdout, return ok: false, head: "", pages: [], and error: the text it printed on
stderr. Run no other command and modify no file.`,
    { label: "rework-state", phase: "Outline", schema: REWORK_STATE_SCHEMA, model: "sonnet", effort: "low" }
  );
  if (!report) throw new Error("docs-page-chain: the rework-state probe returned nothing");
  if (reworkChecksum(report) !== report.checksum) {
    throw new Error(`docs-page-chain: the rework-state probe mistranscribed its report (checksum ${reworkChecksum(report)}, reported ${report.checksum})`);
  }
  if (!report.ok) throw new Error(`docs-page-chain: the rework-state probe failed: ${report.error || "no error given"}`);
  return report;
}

const REWORK_STATE = await reworkProbe();
for (const p of PAGES) {
  if (!p.job) throw new Error(`docs-page-chain: ${p.path} has no job, inline or from an outline`);
}

/**
 * The instruction that names the register sections one role reads for a page.
 * @param {{ track: string }} p
 * @param {"drafter" | "editor"} role
 * @returns {string}
 */
function registerLine(p, role) {
  return `Read these sections of ${REGISTER}, each by its exact heading, before anything else:
${registerSectionsFor(p.track, role).map((h) => `- ${h}`).join("\n")}
The Names convention is enforced by Vale (Cairn.Names, Cairn.NamesRetired); use the sanctioned name
for every part.`;
}

// The preamble every stage shares: the worktree and the error-tier Vale rules.
const common = `Work only in the worktree ${WT}; run every command from there and never cd to another checkout.

Error-tier Vale rules, verbatim:

${a.valeErrorRules || "(run `npm run check:vale` and fix every error-tier finding)"}
`;

/**
 * The page's fact-file base name: its own file name without the `.md` extension.
 * @param {string} path
 * @returns {string}
 */
function baseNoExt(path) {
  const parts = path.split("/");
  return parts[parts.length - 1].replace(/\.md$/, "");
}

/**
 * The brief path the brief-naming rule assigns this page (docs/internal/briefs/README.md).
 * @param {{ path: string, track: string }} p
 * @returns {string}
 */
function briefPathFor(p) {
  return `docs/internal/briefs/${p.track}/${baseNoExt(p.path)}.json`;
}

/**
 * The page plan's path, beside the page's brief.
 * @param {{ path: string, track: string }} p
 * @returns {string}
 */
function planPathFor(p) {
  return `docs/internal/briefs/${p.track}/${baseNoExt(p.path)}.plan.md`;
}

/**
 * The framing record's path, beside the page's brief and plan.
 * @param {{ path: string, track: string }} p
 * @returns {string}
 */
function framingPathFor(p) {
  return `docs/internal/briefs/${p.track}/${baseNoExt(p.path)}.framing.md`;
}

/**
 * The page's slug, the key its option-map rows name: its file's base name, which every outline
 * slug equals.
 * @param {{ path: string }} p
 * @returns {string}
 */
function slugOf(p) {
  return baseNoExt(p.path);
}

/**
 * The friction instruction the three agents that meet the code share: the smells that signal a
 * design flaw, where an entry goes, and that it never blocks the page.
 * @returns {string}
 */
function frictionLine() {
  return `Design friction: when stating the code as it is takes a hedge, a caveat, an exception, a
workaround, a surprising default, or two seams naming or behaving the same thing differently, add
one entry to docs/internal/docs-friction-log.md with the Edit tool, in the shape the log's header
sets, naming the fact ids or file:line involved (on a stale-read failure, re-read the log and redo
the edit). List each entry's heading in frictionFiled. An entry never blocks or pauses the page:
the page documents the code as it is.`;
}

/**
 * The map rule's retag order for an agent that may take a mapped fact off [verified].
 * @param {{ path: string }} p
 * @param {string} tag - the tag the agent retags to
 * @returns {string}
 */
function retagOrder(p, tag) {
  return `Before you retag a fact ${tag}, find every row in the option map ${OPTION_MAP} whose value is
that fact id; one fact may back several rows. Then, in this order and with the Edit tool: raise
pendingCount once, by the number of those rows; then rewrite every one of those rows to
"pending ${slugOf(p)}"; and only then retag the fact, once. This order keeps the option gate green
for every page in flight; on a stale-read failure, re-read the map and redo the edit.`;
}

/**
 * Page inputs' option-map step: the map, the page's slug, the row selection rule, and how each
 * pending row is disposed under the map rule.
 * @param {{ path: string, factIds?: string[] }} p
 * @returns {string}
 */
function mapRowsLine(p) {
  const slug = slugOf(p);
  const ids = p.factIds || [];
  return `
Option map rows. The option map ${OPTION_MAP} holds one row per public option path (its key), whose
value is a fact id, "exclude <reason>", or "pending <slug>", under a top-level pendingCount
constant the option gate holds the pending rows to. This page's slug is "${slug}". Your rows are
every row whose value is "pending ${slug}"${ids.length ? `, and every row whose value is one of these fact ids: ${ids.join(", ")}` : ""};
read them live from the file, never from a copy. Return each one in rowsReceived, key and value
verbatim. A row naming a fact id is context: that fact carries the option.

Dispose each pending row as one claim-inventory entry naming its key, then return it in
rowsDisposed with its disposition and the row's new value:
- "carried" or "filed": a [verified] fact names the member in backticks and its Source: cites the
  declaring type's file. The row becomes that fact id.
- "cut": the option stays off every page, for the reason you give. The row becomes
  "exclude <reason>".
- "re-pointed": the option belongs to another outline page not yet drafted. The row becomes
  "pending <that page's slug>", and the claim carries the reason.
A disposal runs in this order: file the fact, then rewrite the row with the Edit tool, then lower
pendingCount by one (a re-pointed row leaves it unchanged). Rewrite no row but your own and those
the retag order above names, never raise pendingCount except by that order, and on a stale-read
failure (another page in flight wrote the map) re-read the file and redo the edit.
`;
}

/**
 * The gate string for one page: the conductor's template with `{page}` and `{brief}`
 * substituted, plus the tool gate for a page carrying pinned slugs.
 * @param {{ path: string, track: string, pinned?: string[] }} p
 * @returns {string}
 */
function gateFor(p) {
  const withPlaceholders = GATE.replaceAll("{page}", p.path).replaceAll("{brief}", briefPathFor(p));
  return (p.pinned && p.pinned.length && TOOL_GATE) ? `${withPlaceholders} && ${TOOL_GATE}` : withPlaceholders;
}

function gateLine(p) {
  return `Run the docs gate through the gate runner, exactly:
  ${LANE}cairn-run-gate '${gateFor(p)}'
Follow cairn-run-gate's own output for whether to re-issue and for the result; report the exact
command and the last twenty lines as your report's gate fields. A red whole-tree
component (one the --page/--brief scoping does not narrow) that names another in-flight page's
file does not count against this page; report it as "fail" with the failing rule and the other
page named, and do not edit that page. A gate red on a rule the plan says a later task closes (for
example check:arm-indexes before the index task) is reported the same way, and is not a reason to
edit a page this task does not own. Run this gate as your last act.`;
}

function pageInputsPrompt(p) {
  return `Page inputs for ${p.path} in ${WT}. You are step 1 of the docs page chain: you do not
draft the page and you do not run a gate.

${common}
This page's job, from the stage outline, verbatim:

${p.job}
${p.pageType ? `The outline's page type: ${p.pageType}` : ""}
${startingInventory(p)}${exemplarTakesLine(p)}
Read the page at ${p.path} if it already exists on disk, for its claims only, never its prose or
structure: every command, step, transcript, figure, warning, success signal, and prose assertion
is a claim. If the page does not exist yet, there are no claims to inventory; say so and move on.
Give each claim exactly one disposition:
- "carried": a fact bullet in the container already covers it exactly. Cite the fact id.
- "filed": you traced it to code, config, or a vendor doc and filed a new bullet for it with the
  Edit tool (never a shell append), tagged [verified] with a Source: line, or [external] with the
  vendor's URL for a Cloudflare or GitHub step. Cite the new fact id.
- "cut": the claim does not survive the page; give the reason.
- "re-pointed": an option-map row only (below); another page owns the option.

A cited fact whose only source is an arm page is retraced to code, config, or a vendor doc first;
when that fails, retag it [candidate] and do not cite it. Never file a fact tagged [candidate] or
[docs-drift] yourself, and never retag any fact except that one narrow case; the independent fact
read does the rest of the retagging, not you. ${retagOrder(p, "[candidate]")} Then dispose that row
like any other pending row of yours, and return it in rowsDisposed as "reopened" before its final
disposition.
${mapRowsLine(p)}
${frictionLine()}

Inputs to trace claims against (read each in full; the only source of a command, a transcript, a
JSON example, or a fact; never open the old page's own prose for a claim's wording):
${(p.inputs || []).map((i) => `- ${i}`).join("\n")}

Return the structured report only: the page's job (one paragraph), its page type, every fact id
the drafter should draw on, each id you added with why, the full claim inventory, any new fact
id you filed, the map rows you received and disposed, and any friction entry you filed.`;
}

/**
 * The outline's starting inventory for the page-inputs step: its fact ids, what the page covers,
 * and what another page owns. Empty when the page carries none of the three.
 * @param {{ factIds?: string[], covers?: string[], outOfScope?: string[] }} p
 * @returns {string}
 */
function startingInventory(p) {
  const ids = p.factIds || [];
  const covers = p.covers || [];
  const out = p.outOfScope || [];
  if (!ids.length && !covers.length && !out.length) return "";
  return `
The outline's starting inventory for this page. Start from it; it is not the whole of your work.
Fact ids the outline traced to this page's sources: ${ids.join(", ") || "(none)"}
What the page covers:
${covers.map((c) => `- ${c}`).join("\n") || "(none listed)"}
Out of scope, owned by another page:
${out.map((c) => `- ${c}`).join("\n") || "(none listed)"}

Return every outline fact id in factIds. An outline id whose fact this page should not state still
goes in factIds and gets a claim-inventory entry with disposition "cut" and its reason. You may add
an id you trace for a covered topic the outline's ids miss; list each added id in addedFactIds with
why you added it. A topic listed as out of scope stays off the page.
`;
}

/**
 * The exemplar files, one bullet each, with the outline's take line for that exemplar beneath it.
 * @param {{ exemplarSources?: string[], exemplarTakes?: string[] }} p
 * @returns {string}
 */
function exemplarList(p) {
  const takes = p.exemplarTakes || [];
  return (p.exemplarSources || [])
    .map((e, i) => (takes[i] ? `- ${e}\n  Take: ${takes[i]}` : `- ${e}`))
    .join("\n");
}

/**
 * The exemplars and their take lines for the page-inputs step, which reads no exemplar itself but
 * sees what the drafter will take from each. Empty when no exemplar carries a take line.
 * @param {{ exemplarSources?: string[], exemplarTakes?: string[] }} p
 * @returns {string}
 */
function exemplarTakesLine(p) {
  if (!(p.exemplarTakes || []).some(Boolean)) return "";
  return `\nThe exemplars the drafter will imitate, with what the outline takes from each (context for
the page's shape; you do not read them):\n${exemplarList(p)}\n`;
}

/**
 * The outline's scope for the drafter: what the page covers and what another page owns, each a
 * bullet list under its own lead line. Empty when the page carries neither.
 * @param {{ covers?: string[], outOfScope?: string[] }} p
 * @returns {string}
 */
function scopeLines(p) {
  const bullets = (xs) => xs.map((x) => `- ${x}`).join("\n");
  const parts = [];
  if ((p.covers || []).length) parts.push(`What the page covers, from the outline:\n${bullets(p.covers)}`);
  if ((p.outOfScope || []).length) parts.push(`Out of scope, owned by another page; keep it off this page:\n${bullets(p.outOfScope)}`);
  return parts.length ? `\n${parts.join("\n\n")}\n` : "";
}

/**
 * The drafter's index-link step: the helper command it runs after writing the page and before
 * its gate. Empty without an outline naming an index.
 * @param {{ path: string }} p
 * @returns {string}
 */
function indexLinkLine(p) {
  if (!INDEX) return "";
  return `After you write the page and its brief, and before you run the gate, link the page into the
arm index ${INDEX} by running exactly:
  cairn-docs-outline link --worktree '${WT}' --outline '${OUTLINE}' --page '${p.path}'
It adds one link under the page's group heading in outline order, or reports that the index already
links the page. Never edit the index yourself and never add a second link. Report its one output
line as indexLink; if it exits non-zero, report its error as indexLink and in couldNotDo.
`;
}

/**
 * The claim inventory as prompt lines: one per claim, with the section a plan places it in and
 * the reason a claim is cut.
 * @param {{ claimInventory?: Array<{ claim: string, disposition: string, factId?: string, section?: string, reason?: string }> }} pageInputs
 * @returns {string}
 */
function inventoryLines(pageInputs) {
  return (pageInputs.claimInventory || [])
    .map((c) => `- [${c.disposition}] ${c.claim}${c.factId ? ` (${c.factId})` : ""}${c.section ? ` in section "${c.section}"` : ""}${c.reason ? ` -- ${c.reason}` : ""}`)
    .join("\n");
}

/**
 * Writes the plan's dispositions back into the claim inventory. The ids the plan must dispose are
 * the outline's and page inputs' fact ids, less any id every inventory entry already cuts or
 * re-points; the plan may still dispose such an id again. A plan entry disposes its id only as
 * "carried" with a section or "cut" with a reason. An inventory claim keeps its own wording and
 * takes the plan's disposition for its id; a plan entry for an id the inventory lacks is appended.
 * @param {{ factIds?: string[] }} p
 * @param {{ factIds?: string[], claimInventory?: Array<Record<string, any>> }} pageInputs
 * @param {Array<Record<string, any>>} [planInventory] - the plan step's claimInventory
 * @returns {{ required: string[], undisposed: string[], inventory: Array<Record<string, any>> }}
 */
function applyPlan(p, pageInputs, planInventory) {
  const inv = pageInputs.claimInventory || [];
  const settled = (id) => {
    const es = inv.filter((c) => c.factId === id);
    return es.length > 0 && es.every((c) => c.disposition === "cut" || c.disposition === "re-pointed");
  };
  const ids = [...new Set([...(p.factIds || []), ...(pageInputs.factIds || []), ...inv.map((c) => c.factId).filter(Boolean)])];
  const required = ids.filter((id) => !settled(id));
  const byId = new Map();
  for (const e of planInventory || []) {
    if (!e || !e.factId) continue;
    if (e.disposition === "carried" && e.section) byId.set(e.factId, { disposition: "carried", section: e.section });
    else if (e.disposition === "cut" && e.reason) byId.set(e.factId, { disposition: "cut", reason: e.reason });
  }
  const inventory = inv.map((c) => (c.factId && byId.has(c.factId) ? { claim: c.claim, factId: c.factId, ...byId.get(c.factId) } : c));
  for (const e of planInventory || []) {
    if (e && byId.has(e.factId) && !inventory.some((c) => c.factId === e.factId)) inventory.push({ claim: e.claim, factId: e.factId, ...byId.get(e.factId) });
  }
  return { required, undisposed: required.filter((id) => !byId.has(id)), inventory };
}

// Google Technical Writing Two, "Organizing large documents": the source of the page plan, and
// of the structural edit seat's introduction, review, and navigation items.
const GOOGLE_LARGE_DOCS = "https://developers.google.com/tech-writing/two/large-docs";

/**
 * The plan step's prompt, for the first plan or its one revision on the plan read's findings. The
 * plan is Google's outline written down, per the 2026-10-01 page-plan diagnosis
 * (docs/superpowers/research/2026-10-01-draft-docs-2a-page-plan-diagnosis.md): the chain carried
 * no artifact that holds a page's argument.
 * @param {Record<string, any>} p
 * @param {Record<string, any>} pageInputs
 * @param {string} [findings] - the plan read's findings a revision works from
 * @returns {string}
 */
function planPrompt(p, pageInputs, findings) {
  const head = findings
    ? `Revise the page plan ${planPathFor(p)} for ${p.path} once, on the structural edit's findings below. Fix every blocking finding, keep the plan's dispositions in step with the revision, and return the whole report again.`
    : `Write the page plan for ${p.path} at ${planPathFor(p)}. You are the plan step of the docs page chain: you write no page prose and you do not run a gate.`;
  const rework = typeof p.rework === "string" && p.rework !== ""
    ? `\nThis page is committed and is being reworked. Its job-read findings and owner rulings, verbatim:\n\n${p.rework}\n`
    : "";
  return `${head}

${common}
The plan is Google's outline, written down (Google Technical Writing Two, "Organizing large documents",
${GOOGLE_LARGE_DOCS}): "You might find it useful to think of an outline as the narrative for your
document," and "Structure your outline so that your document introduces information when it's most
relevant to your reader." The drafter drafts from it: the plan is the source of the page's order,
each section's claim, and each fact's placement. The structural edit seat reads it before any prose
exists. The outline's covers order is an inventory, and Google's lesson is "a published reason to
re-sequence sections that follow the outline's \`covers\` order as an inventory" (${PRIOR_ART},
"## b. Structural edit seat"): the plan argues the order.

Read the section "## The page anatomies" of ${REGISTER}, by its exact heading, for this page type's
introduction and ending.

The page's job, from the stage outline: ${pageInputs.job}
Page type: ${pageInputs.pageType}
${scopeLines(p)}
The exemplars the drafter will imitate, with what the outline takes from each (context for the
page's shape):
${exemplarList(p) || "(none named; follow the register's anatomy)"}
${rework}
Fact ids the plan disposes, every one: ${applyPlan(p, pageInputs).required.join(", ") || "(none)"}
Claim inventory from page inputs, one disposition per claim (you may dispose a "cut" fact id again):
${inventoryLines(pageInputs) || "(none recorded)"}

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

Return claimInventory with one entry per fact id you dispose, in the existing dispositions only:
"carried" with its factId and the section heading it is placed under, or "cut" with its factId and
the reason (a subordinated fact's reason names the reference page or entry that states it).
${findings ? `\nThe structural edit's findings on the plan:\n${findings}\n` : ""}
${frictionLine()}

Commit nothing; leave the plan file in place. Return the structured report only: the plan's path,
claimInventory, couldNotDo, and frictionFiled.`;
}

// The register heading that holds the house rulings on introductions, read by the framing step.
const INTRO_HEADING = "### The introduction";

/**
 * The doc set's map for the framing step: the stage outline whole when the run has one, the one
 * input that lists every page and its job; otherwise the run's own page list, inline.
 * @returns {string}
 */
function docMapLine() {
  const indexes = `Then read the arm indexes under docs/ (each docs/<arm>/README.md that exists, and
docs/README.md), for the pages outside this run that link to this page or that it links to.`;
  if (OUTLINE) {
    return `Read the stage outline ${OUTLINE} whole: its groups and their titles, every page's slug,
title, job, covers, and outOfScope, and its crossLinks (each names a page that links another, and
why). ${INDEX ? `Read the arm index ${INDEX} it names. ` : ""}${indexes}`;
  }
  return `The pages this run drafts, each with its job:
${PAGES.map((q) => `- ${q.path}: ${q.job}`).join("\n")}
${indexes}`;
}

/**
 * The framing step's prompt: decide the introduction's framing from the reader, before the
 * drafter writes it, and record it beside the brief. The method was proven by hand in the
 * 2026-10-04 intro round (`9ca04531` on `draft-docs-2a`), whose records set the output's shape.
 * @param {Record<string, any>} p
 * @param {Record<string, any>} pageInputs - carrying the plan's dispositions
 * @returns {string}
 */
function framingPrompt(p, pageInputs) {
  const out = framingPathFor(p);
  return `Frame the introduction of ${p.path} in ${WT}, and write the framing record at ${out}. You are
the framing step of the docs page chain: you write no page prose, you leave the page plan as it is,
and you do not run a gate.

${common}
Why this step exists (house ruling, Geoff, 2026-10-04): a page's introduction is not drafted like
its body. The body is fact-driven. The introduction carries the high-level reasoning a body leaves
out, and getting its framing and background right takes judgment, which you supply before the
drafter writes a word. Reason this page's framing fresh: introductions follow no fixed pattern, and
another page's framing record is never a template for this one.

Read these sections of ${REGISTER}, each by its exact heading:
- ${INTRO_HEADING} (under "## Drafting brief: developer docs"), whole: the source of every rule below
- ## The page anatomies, for this page type's introduction
${p.track === "editors" ? "- ## Drafting brief: editor docs, for this track's voice\n" : ""}
The doc set's map. ${docMapLine()}

${p.title ? `The page's title: ${p.title}\n` : ""}The page's job: ${pageInputs.job}
Page type: ${pageInputs.pageType}
${scopeLines(p)}
The page plan ${planPathFor(p)}: read it whole. Its introduction's three parts (what the page
covers, the prior knowledge it expects, what it leaves out) stand, and its body sections are
fixed; the framing builds the introduction around them.

Fact ids the page carries: ${(pageInputs.factIds || []).join(", ") || "(none)"}
Claim inventory, with the plan's dispositions:
${inventoryLines(pageInputs) || "(none recorded)"}
Read each fact's bullet in docs/internal/facts/ (search for its id) before you lean on it.

Work in this order.
1. Readers. From the map, work out who arrives at this page, from where (which pages link here,
   which index group lists it, a search, the setup command's output), what each is looking for,
   what each already knows, and what each lacks that the page assumes. A page can serve several
   readers with different reasons; name each one. When the map shows a reader likely to arrive in
   the wrong place, name them and the page they belong on.
2. Background. Decide the general model the page sits in; where SvelteKit, Cloudflare, and GitHub
   fit when the page touches them (say so when one is left out on purpose, and why); and why the
   thing the page covers exists (why a default is what it is, before a page that changes it).
   Back each point with a fact id. When a point the readers need has no fact, trace it to code,
   config, or a vendor doc and file a new bullet with the Edit tool (never a shell append), tagged
   [verified] with a Source: line, or [external] with the vendor's URL, never [candidate]; list it
   in factsFiled. A point you cannot back is a couldNotDo, never a point in the record.
3. Place in the doc set: the page's group and neighbors, what links in and why, and what sibling
   pages own, so the introduction words a shared idea fresh instead of repeating a neighbor's
   opening. You may read a sibling page or its framing record on disk for that, never to copy its
   shape.
4. Intro plan: the introduction as numbered paragraphs, two or three when the framing needs them,
   ahead of the plan's bounds lists. It opens on a statement about the subject, from the reader's
   situation, never on an imperative and never on a sentence about the page itself. It names each
   reader's reason, so every reader learns early whether the page answers it. It keeps the plan's
   three introduction parts, and on a task guide it keeps the anatomy's one-line contract, the
   sentence that names what the reader accomplishes. Name the fact ids each paragraph draws on.

The record is agent-facing markdown, under these headings:
- "# Framing record: <the page's title>", then one line: agent-facing; drives the introduction of
  ${p.path} only; the body follows the page plan.
- "## Who arrives, from where, and why": one entry per reader (a list or a table, whichever reads
  better), each with who they are, where they arrive from, what they came for, what they know, and
  what they lack.
- "## Background the page rests on": each point with its fact ids.
- "## Place in the doc set".
- "## Intro plan".
- "## Departures from the plan's introduction", only when the intro plan departs from the page
  plan's introduction: how, and why.
Name every file in a code span, never as a Markdown link: the docs link gate walks docs/. If a
framing record already exists at ${out}, read it, then reason the framing fresh against the current
plan, keep what still holds, and overwrite it.

${frictionLine()}

Commit nothing; leave the record in place. Return the structured report only: the record's path as
framing, one line per reader as readers, every fact id the intro plan draws on as factIds (filed
ones included), factsFiled, couldNotDo, and frictionFiled.`;
}

// The section a fact the framing record draws on is placed in.
const INTRO_SECTION = "Introduction";

/**
 * Writes the framing record's fact ids into the page inputs: each joins the drafter's fact ids,
 * and the claim inventory carries it in the introduction. An id already carried or filed keeps
 * its entry; an id the plan cut is re-placed in the introduction; an id the inventory lacks is
 * appended.
 * @param {Record<string, any>} pageInputs - carrying the plan's dispositions
 * @param {string[]} ids - the framing step's factIds
 * @returns {Record<string, any>}
 */
function applyFraming(pageInputs, ids) {
  let inventory = (pageInputs.claimInventory || []).slice();
  for (const id of ids || []) {
    if (!id) continue;
    const entries = inventory.filter((c) => c.factId === id);
    if (entries.some((c) => c.disposition === "carried" || c.disposition === "filed")) continue;
    const intro = { factId: id, disposition: "carried", section: INTRO_SECTION };
    if (entries.some((c) => c.disposition === "cut")) {
      inventory = inventory.map((c) => (c.factId === id && c.disposition === "cut" ? { claim: c.claim, ...intro } : c));
    } else {
      inventory.push({ claim: "background the framing record places in the introduction", ...intro });
    }
  }
  const factIds = [...new Set([...(pageInputs.factIds || []), ...(ids || []).filter(Boolean)])];
  return { ...pageInputs, factIds, claimInventory: inventory };
}

/**
 * Asks a probe agent whether one command exits zero, since the runtime itself has no filesystem or
 * exec access. A probe that returns nothing counts as a miss, so the caller fails closed.
 * @param {string} label
 * @param {string} command - run from the worktree, read-only
 * @returns {Promise<boolean>}
 */
async function probeExists(label, command) {
  const r = await agent(
    `Run exactly this command, once, from the worktree ${WT}:

  ${command}

Return exists: true if it exited with status 0, otherwise exists: false, and its output, if any,
as output. Run no other command and modify no file.`,
    { label, phase: "Read", schema: EXISTS_SCHEMA, model: "sonnet", effort: "low" }
  );
  return Boolean(r && r.exists === true);
}

/**
 * The framing step: one agent decides the introduction's framing and writes its record, and the
 * runner writes the record's fact ids back into the page inputs. Nothing reviews the record
 * before the draft; the structural edit seat and the register editor grade the introduction
 * against it.
 * @param {Record<string, any>} p
 * @param {Record<string, any>} pageInputs - carrying the plan's dispositions
 * @param {Record<string, any>} record - gains `framingStep`
 * @returns {Promise<{ pageInputs: Record<string, any> } | { escalate: Record<string, any> }>}
 */
async function framingStep(p, pageInputs, record) {
  const f = await agent(framingPrompt(p, pageInputs), { label: `framing:${p.id}`, phase: "Framing", schema: FRAMING_SCHEMA, model: FRAMING_MODEL, effort: FRAMING_EFFORT, agentType: "general-purpose" });
  if (!f) return { escalate: { ...record, status: "escalate", reason: "the framing step returned nothing" } };
  const path = String(f.framing || "").trim();
  if (!path || !(await probeExists(`probe:${p.id}:framing`, `test -f '${path}'`))) {
    return { escalate: { ...record, status: "escalate", reason: `the framing step reported a record not on disk: ${path || "(no path)"}` } };
  }
  record.framingStep = {
    readers: f.readers || [],
    factIds: f.factIds || [],
    factsFiled: f.factsFiled || [],
    couldNotDo: f.couldNotDo || [],
    frictionFiled: f.frictionFiled || []
  };
  return { pageInputs: applyFraming(pageInputs, f.factIds) };
}

/**
 * The framing record's instruction for the drafter: the introduction comes from the record, the
 * body from the plan.
 * @param {{ path: string, track: string }} p
 * @returns {string}
 */
function framingDraftLine(p) {
  return `The framing record at ${framingPathFor(p)} is the source of the introduction: read it whole
before you draft, and write the introduction from its intro plan, naming each reader's reason it
names and carrying the background it decides, with the fact ids it gives each paragraph. The
introduction opens on a statement, never an imperative and never a sentence about the page; it may
run two or three paragraphs, and it keeps the plan's three introduction parts. The body stays
fact-driven, drafted from the page plan.`;
}

/**
 * The instruction a page read gets to grade the introduction against the framing record.
 * @param {{ path: string, track: string }} p
 * @returns {string}
 */
function framingReadLine(p) {
  return `The framing record ${framingPathFor(p)} decides the introduction: its readers, the background the
page rests on, the page's place in the doc set, and its intro plan. Read it before the page, and
grade the introduction against it and the register's "${INTRO_HEADING}" section. Each of these is a
blocking finding: a reader's reason the record names that the introduction omits; background the
record decides that the introduction drops; a departure from its intro plan the record does not
declare; an opening on an imperative or on a sentence about the page itself.`;
}

/**
 * The drafter's prompt for a first draft, the round-2 redraft, or the one redraft after the final
 * reader read, which changes only the spots that read cites.
 * @param {Record<string, any>} p
 * @param {Record<string, any>} pageInputs
 * @param {number} round
 * @param {string} [findings] - the combined findings a redraft works from
 * @param {boolean} [afterFinalRead] - the redraft is the final reader read's
 * @param {boolean} [snapshot] - the chain's round-2 redraft, which first records the page as round
 *   1 read it, the baseline the narrowed round-2 reads diff against
 * @returns {string}
 */
function draftPrompt(p, pageInputs, round, findings, afterFinalRead = false, snapshot = false) {
  const head = round === 1
    ? `Draft the page ${p.path} at its final path, from its page plan and the page inputs below and nothing else.`
    : afterFinalRead
      ? `Redraft ${p.path} once, on the final reader read's findings below. Change only the spots they cite and leave the rest of the page as it stands.`
      : `Redraft ${p.path} once, on the combined findings below. Fix every blocking finding; take a non-blocking one when it is right. Do not widen the page.`;
  const inventory = inventoryLines(pageInputs);
  const baseline = snapshot
    ? `\nBefore you edit anything, run exactly this from the worktree:
  git hash-object -w -- ${p.path}
and report the hex object name it prints, verbatim, as baseline: the page as the round-1 reads saw
it, which the round-2 reads diff against. Run it once, before your first edit.\n`
    : "";
  return `${head}
${baseline}
${common}
${registerLine(p, "drafter")}
The page plan at ${planPathFor(p)} is the source of the page's order, each section's claim, and each
fact's placement: read it whole before you draft, and draft its sections in its order, each opening
on the sentence the plan gives it. The register's drafting brief is the source of voice. Voice comes
only from the register's drafting brief and its primary exemplar, docs/extend/choose-an-ai-posture.md;
the page's exemplars below supply structure and detail per step, never voice or wording.
${framingDraftLine(p)}
${p.title ? `\nThe page's H1, verbatim: # ${p.title}\n` : ""}
The page's job: ${pageInputs.job}
Page type: ${pageInputs.pageType}
${scopeLines(p)}
Exemplar sources: read each in full, and imitate its anatomy and its detail per step, never its
voice, wording, terms, or product names:
${exemplarList(p) || "(none named; follow the register's anatomy)"}
${p.figure ? `\nThis page carries a figure: read and follow the skill file ~/.claude/skills/cairn-figure/SKILL.md.\n` : ""}${p.figure && p.figureNote ? `\nFigure note, from the outline: ${p.figureNote}\n` : ""}

Fact ids to draw on: ${(pageInputs.factIds || []).join(", ") || "(none)"}
Claim inventory, one disposition per claim, with the page plan's dispositions (a "carried" claim
sits in the section the plan names; a "cut" or "re-pointed" claim stays off the page, and a
subordinated one takes the link to the reference page its reason names):
${inventory || "(the page is new; no prior claims to carry)"}
${p.pinned && p.pinned.length ? `Pinned heading slugs this page must keep, verbatim: ${p.pinned.join(", ")}` : ""}
${round > 1 ? `\nCombined findings from the reads:\n${findings}\n` : ""}
Write the introduction, the section hand-off lead-ins, and the ending the register's page anatomies
require for this page's type; the removal rule does not cut them, nor a sentence that carries a
section's claim from the plan. In the brief, record each of
those sentences "no-claim" when it carries no extractable fact, and cite its fact id when it does:
scripts/checks/check-provenance.mjs fails a no-claim sentence that holds one ("a no-claim sentence
cites nothing, so any extractable fact in it fails").

Write the page's sentence-to-fact brief alongside the page at ${briefPathFor(p)}, citing only the
fact ids above or "no-claim". A sentence's id is one fact id, an array of the fact ids it
synthesizes when it states two or more together, or "no-claim". The brief also carries a top-level
"cuts" array mirroring the plan's cut dispositions: one { "id", "reason" } entry per fact id the
claim inventory above marks "cut", its reason verbatim. File no fact yourself, new or retagged. A claim the page needs whose
fact is not among the ids above is a couldNotDo naming the missing fact; do not draft that claim
and do not file its fact yourself, since the conductor re-runs page inputs for it.

${frictionLine()}

Commit nothing; leave the tree with your edits in place.

${indexLinkLine(p)}
${gateLine(p)}

Return the structured report only.`;
}

/**
 * The scope line for a register or fact read on an edit after the chain: the parent spec's rule,
 * where the changed sentences are, and the findings the redraft worked from. Empty for a chain
 * round on a drafted page.
 * @param {string} [scope] - the findings the redraft received
 * @param {string} [where] - where the changed sentences are; defaults to the final reader read's spots
 * @returns {string}
 */
function scopeNote(scope, where = "The page changed only at the spots the final reader read cites below") {
  if (!scope) return "";
  return `
This read is scoped. ${where}. "Changed sentences get both reviews, scoped to those sentences"
(docs/superpowers/specs/2026-09-26-draft-docs-approach-design.md, "Edits after the chain"). Grade
the changed sentences.

${scope}
`;
}

/**
 * The note that narrows a round-2 read: the seat verifies its own round-1 findings' fixes and
 * reads only the sentences the redraft changed, by the diff against the redrafter's baseline.
 * Under `bothReviewers` a seat that accepted in round 1 still reads the changed sentences, for a
 * defect the fixes introduced, which keeps the cross-regression flag measuring.
 * @param {{ path: string }} p
 * @param {{ baseline: string, own?: { verdict: string, summary: string, findings: Array<Record<string, any>> } }} [narrow]
 * @param {string} name - the seat's name in the round-1 findings
 * @returns {string}
 */
function roundTwoNote(p, narrow, name) {
  if (!narrow) return "";
  const own = narrow.own;
  const findings = own ? combined([[name, { ...own, findings: own.findings || [] }]]) : "";
  const task = own && own.verdict === "fix"
    ? `1. Verify each of your blocking round-1 findings below: fixed, or still open (still blocking).
   Read around a finding's location only as far as judging its fix needs.
2. Read only the changed sentences, for a new defect under your checklist, a defect a fix
   introduced included.`
    : `You accepted in round 1. Read only the changed sentences, for a defect the fixes introduced,
under your checklist.`;
  return `
This is a round-2 read, narrowed to the redraft. Do not re-read the whole page. Run exactly this
from the worktree:
  git cat-file blob ${narrow.baseline} | diff -u --label round-1 --label round-2 - ${p.path}
It prints every line the redraft changed, against the page as round 1 read it; no output means the
redraft changed nothing, so every blocking round-1 finding stands. A deleted line is a changed
line: judge what its removal costs.
${task}
An unchanged sentence stays as round 1 graded it.
${findings ? `\nYour round-1 findings:\n${findings}\n` : ""}`;
}

function editorPrompt(p, scope, where, narrow) {
  return `Adversarial register edit of ${p.path} in ${WT}.

${common}
${registerLine(p, "editor")}
${framingReadLine(p)}
${scopeNote(scope, where)}${roundTwoNote(p, narrow, "register editor")}
Run plain \`vale ${p.path}\` from the worktree, never the docs gate, and read every alert it
prints${narrow ? " (on a narrowed read, grade only the alerts on changed lines)" : ""}. Read the page, then grade the brief's structure checklist, the brief's tells, and Vale's
alerts together, with the Names section, logic, and facts-adjacent phrasing. Return ranked
findings with a proposed rewrite each, and a verdict: "fix" if any finding is blocking.`;
}

function factPrompt(p, pageInputs, scope, where, narrow) {
  const inventory = inventoryLines(pageInputs);
  return `Fact read of ${p.path} in ${WT}. The page's claim inventory from its page-inputs step, with
the dispositions of its page plan, ${planPathFor(p)}: a "carried" claim names the section the plan
places it in, and a subordinated fact is "cut" with a reason naming the reference page or entry
that states it.
${inventory || "(none recorded)"}
${outlineCoverage(p)}${scopeNote(scope, where)}${roundTwoNote(p, narrow, "fact read")}
Check every claim on the page against its cited fact id, retrace every cited fact against its
source and fix or retag it [docs-drift] in the same chain when it no longer matches, and confirm
every claim the inventory marks "carried" or "filed" still appears on the page. A claim on the
page with no cited fact behind it, a cited fact that no longer matches its source, or an inventory
claim the page dropped without a "cut" or "re-pointed" disposition is a blocking finding (location,
claim, what is wrong).

${retagOrder(p, "[docs-drift]")} Each row you rewrote is a blocking finding, located at
${OPTION_MAP} and the row's key, so the page escalates unless the redraft round resolves it.

Read ${OPTION_MAP} live now: every row whose value still reads "pending ${slugOf(p)}" is a blocking
finding, located at the map and the row's key, whoever left it pending. Page inputs disposes this
page's rows and does not re-run, so an accepted page leaves no row pending its slug.

${frictionLine()}

Verdict "fix" if any blocking finding exists; otherwise "accept" with the count of claims traced.`;
}

/**
 * The fact read's outline coverage check: every outline fact id is carried on the page or cut with
 * a reason, and an absorbed page's facts are not silently dropped. The inventory carries the page
 * plan's dispositions, so an id the plan subordinates or cuts is disposed, and an id it places is
 * held to the page. Empty without outline ids or absorbed pages.
 * @param {{ track: string, factIds?: string[], absorbs?: string[] }} p
 * @returns {string}
 */
function outlineCoverage(p) {
  const ids = p.factIds || [];
  const absorbs = p.absorbs || [];
  if (!ids.length && !absorbs.length) return "";
  const idPart = ids.length
    ? `The outline's fact ids for this page: ${ids.join(", ")}
Each one must be cited on the page (see its brief, ${briefPathFor(p)}) or appear in the claim
inventory above as "cut" with a reason. An outline id that is neither is a blocking finding.
An outline id the plan subordinates or cuts with a reason is disposed, not dropped; an id the plan
places in a section that the page omits is a blocking finding.`
    : "";
  const absorbPart = absorbs.length
    ? `This page absorbs these retired pages, whose facts sit in docs/internal/facts/${p.track}.md under
a heading naming each:
${absorbs.map((x) => `## ${x}`).join("\n")}
Read those sections. A topic of theirs the page drops with no "cut" in the inventory is a blocking
finding.`
    : "";
  return `\n${[idPart, absorbPart].filter(Boolean).join("\n\n")}\n`;
}

function figurePrompt(p, narrow) {
  return `Verify every figure on ${p.path} in ${WT} against the two figure tests and the
2026-08-15 visual-layer rulings. A failed figure is a blocking finding. Verdict "fix" if any
figure fails; otherwise "accept".${p.figureNote ? `\n\nFigure note, from the outline: ${p.figureNote}` : ""}${roundTwoNote(p, narrow, "figure verifier")}`;
}

// The prior-art record the structural edit seat and the final reader read quote: the only source
// of their checklist and test wording.
const PRIOR_ART = "docs/superpowers/research/2026-09-30-page-level-review-prior-art.md";

/**
 * The structural edit seat's prompt: the two published blocks the prior-art record adopts in
 * "## b. Structural edit seat", quoted verbatim, with the type's introduction duties from the
 * register's page anatomies. The plan read runs them over the page plan before any prose,
 * grading order, pace, user goal, and the introduction's three parts at the plan level against
 * the outline entry. The page read runs them over the whole page against its plan, since the plan
 * carries the order and its reason, Google's outline "as the narrative for your document".
 * @param {Record<string, any>} p
 * @param {"plan" | "page"} [mode]
 * @param {{ baseline: string, own?: Record<string, any> }} [narrow] - a narrowed round-2 read
 * @returns {string}
 */
function structurePrompt(p, mode = "page", narrow) {
  const bullets = (xs) => (xs || []).map((x) => `- ${x}`).join("\n") || "(none listed)";
  const plan = mode === "plan";
  const crossLinks = OUTLINE
    ? `Cross-links: read the top-level crossLinks array in ${OUTLINE} live, and take every entry
whose "from" is "${slugOf(p)}"; each names a page this one links ("to") and why.`
    : "";
  const head = plan
    ? `Structural edit of the page plan ${planPathFor(p)} for ${p.path} in ${WT}. No prose exists yet:
read the plan, never the page.`
    : `Structural edit of ${p.path} in ${WT}.`;
  const level = plan
    ? `
This is the plan read. The plan is Google's outline, written down (Google Technical Writing Two,
"Organizing large documents", ${GOOGLE_LARGE_DOCS}): "You might find it useful to think of an
outline as the narrative for your document," and "Structure your outline so that your document
introduces information when it's most relevant to your reader." At the plan level, grade order,
pace, user goal, and the introduction's three parts: Block 1's "Information is presented in the
most logical order and location.", "Information is provided at the right pace.", and "The user
goal is clear.", and Block 2's three introduction items, read against the plan's introduction.
`
    : "";
  const entryRead = plan
    ? `How the entry is read, from the same record: "the outline's \`job\` is the page's purpose, \`covers\`
is the list the page must deliver in the order the seat judges (Block 1 "most logical order"), and
\`outOfScope\` is the page's "doesn't cover" list; the seat compares the introduction to those three."`
    : `The page's plan, ${planPathFor(p)}, carries its order and the reason for it, each section's claim,
and each fact's placement: Google's outline "as the narrative for your document" (${GOOGLE_LARGE_DOCS}).
Read it whole before the page, then grade the page against its plan and this checklist, no longer
against the outline's \`covers\` order. The introduction is still compared to the entry: \`job\` is the
page's purpose, \`covers\` what it delivers, and \`outOfScope\` its "doesn't cover" list.`;
  return `${head}

${common}
The level, as the Editorial Freelancers Association defines it: "Developmental editors (also called
'substantive,' 'structural,' or 'content' editors) deal with content, organization, and genre
considerations." Your stance, from ${PRIOR_ART}, "## b. Structural edit seat": "an editor. It reads
the whole page against the outline and this checklist, returns findings with \`file:line\`, and never
edits." Grade the ${plan ? "plan" : "page"} at this level only.
${level}
The page's outline entry${OUTLINE ? `, from ${OUTLINE}` : ""}:
Job: ${p.job}
Page type: ${p.pageType || "(none given)"}
What the page covers:
${bullets(p.covers)}
Out of scope, owned by another page:
${bullets(p.outOfScope)}
${crossLinks}

${entryRead}
${plan ? "" : `${framingReadLine(p)}\n`}${roundTwoNote(p, narrow, "structural edit")}
Cross-links are the page's "See-also, next steps, related resources" ("## e. Outline fields").

Read the section "## The page anatomies" of ${REGISTER}, by its exact heading. The introduction it
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
(${GOOGLE_LARGE_DOCS}). Check the introduction on every page.
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

A checklist item the ${plan ? "plan" : "page"} fails is a blocking finding: its file:line as location, the item it fails
and how, and in rewrite a suggestion for how to address it. Verdict "fix" if any blocking finding
exists; otherwise "accept".`;
}

/**
 * The final reader read's prompt: the Federal Plain Language Guidelines' Part V test as the
 * prior-art record adopts it in "## f. The final reader read", with the agent as the participant.
 * The prompt names the page alone; the job is read from the outline only after the reader has
 * answered, since "the outline is what it is tested against afterward" (the record's table).
 * @param {Record<string, any>} p
 * @param {string} pageType - the page-inputs step's page type, or the outline's
 * @returns {string}
 */
function readerPrompt(p, pageType) {
  const type = String(pageType || "");
  let first;
  if (/concept/i.test(type)) {
    first = `Part V's paraphrase test: "Ask the participant to read to a specific stopping point, known as
a cue. Each time the participant reaches a cue, ask the participant to tell you in his or her own
words what that section means." Each section's end is a cue: write in your own words what that
section means, and keep each paraphrase in summary.`;
  } else if (/task|tutorial|how-to/i.test(type)) {
    first = `DigitalOcean's test for a tutorial: "We ask that you thoroughly test it by reading through it and
following it as a reader would." Follow the page as the cold reader of
docs/superpowers/specs/2026-09-21-draft-docs-design.md does: "an agent with the page and a terminal
only", "logging every point where it had to infer". Run a step only in a scratch directory outside
the worktree; read a step that needs an account, a secret, or a deploy without running it, and log
what you had to infer to understand it.`;
  } else {
    first = `Read the page once, whole.`;
  }
  const job = OUTLINE
    ? `open ${OUTLINE} and read the "job" of the entry in "pages" whose "path" is "${p.path}"`
    : `read the page's job: ${p.job}`;
  return `Final reader read of ${p.path} in ${WT}. You are the reader: you have this page and nothing
else. Read no other file until step 3, and modify no file in the worktree.

The method is the Federal Plain Language Guidelines, Part V, "Test", and you are its participant.

Step 1. ${first}

Step 2. Answer Part V's open-ended questions:
  What would you do if you got this document?
  What do you think the writer was trying to do with this document?
  Thinking of other people you know who might get this document:
    What about the document might work well for them?
    What about the document might cause them problems?
Return your answer to the second question as paraphrase, and the other answers in summary.

Step 3. Only now, ${job}. Each mismatch between your paraphrase and that job is a blocking finding:
"wherever participants misunderstood the message, the document has a problem that you should fix"
(Part V). Each point where you had to infer is a finding, blocking when it is an unstated step or an
unclassified error (the cold-reader row of docs/superpowers/specs/2026-09-21-draft-docs-design.md).
Give each its file:line as location. The method's source is ${PRIOR_ART}, "## f. The final reader
read". Verdict "fix" if any blocking finding exists; otherwise "accept".`;
}

function combined(readList) {
  return readList
    .map(([name, r]) => `## ${name}: ${r.verdict}\n${r.summary}\n` +
      r.findings.map((f) => `- [${f.blocking ? "BLOCKING" : "advisory"}] ${f.location}: ${f.finding}${f.rewrite ? `\n  rewrite: ${f.rewrite}` : ""}`).join("\n"))
    .join("\n\n");
}

/**
 * Every reviewer this page carries: the three standing reads, plus a figure-verifier read for a
 * page with `p.figure` set.
 * @param {{ figure?: boolean }} p
 * @returns {string[]}
 */
function reviewNames(p) {
  return ["structural edit", "register editor", "fact read", ...(p.figure ? ["figure verifier"] : [])];
}

/**
 * Runs the named reads in parallel. `round` is a chain round's number, or "final" for the scoped
 * reads after the final reader read's redraft, which pass that read's findings as `scope`.
 * @param {Record<string, any>} p
 * @param {Record<string, any>} pageInputs
 * @param {number | "final"} round
 * @param {string[]} names
 * @param {string} [scope]
 * @param {string} [where] - where the changed sentences are, for a rework page's scoped reads
 * @param {{ baseline: string, round1: Array<[string, any]> }} [narrowed] - a narrowed round 2: the
 *   redrafter's baseline and the round-1 reads, from which each seat takes its own findings
 * @returns {Promise<{ list: Array<[string, any]>, missing: number, anyFix: boolean }>}
 */
async function runReads(p, pageInputs, round, names, scope, where, narrowed) {
  const tag = round === "final" ? "final" : `r${round}`;
  const phaseName = round === "final" ? "Final read" : "Read";
  const narrow = (name) => {
    if (!narrowed) return undefined;
    const own = narrowed.round1.find(([n]) => n === name);
    return { baseline: narrowed.baseline, own: own ? own[1] : undefined };
  };
  const tasks = [];
  if (names.includes("structural edit")) {
    tasks.push(["structural edit", () => agent(structurePrompt(p, "page", narrow("structural edit")), { label: `structure:${p.id}:${tag}`, phase: phaseName, schema: READ_SCHEMA, model: REVIEWER, agentType: "general-purpose" })]);
  }
  if (names.includes("register editor")) {
    tasks.push(["register editor", () => agent(editorPrompt(p, scope, where, narrow("register editor")), { label: `editor:${p.id}:${tag}`, phase: phaseName, schema: READ_SCHEMA, model: REVIEWER, agentType: "cairn-register-editor" })]);
  }
  if (names.includes("fact read")) {
    tasks.push(["fact read", () => agent(factPrompt(p, pageInputs, scope, where, narrow("fact read")), { label: `facts:${p.id}:${tag}`, phase: phaseName, schema: READ_SCHEMA, model: REVIEWER, agentType: "general-purpose" })]);
  }
  if (names.includes("figure verifier")) {
    tasks.push(["figure verifier", () => agent(figurePrompt(p, narrow("figure verifier")), { label: `figure:${p.id}:${tag}`, phase: phaseName, schema: READ_SCHEMA, model: REVIEWER, agentType: "figure-verifier" })]);
  }
  const results = await parallel(tasks.map(([, fn]) => fn));
  const list = tasks.map(([name], i) => [name, results[i]]).filter(([, r]) => r);
  const missing = tasks.length - list.length;
  const anyFix = list.some(([, r]) => r.verdict === "fix");
  return { list, missing, anyFix };
}

/**
 * One round's read entries for the record. The fact read's `frictionFiled` is copied onto its
 * entry; any other read's is dropped, since only the fact read reports friction.
 * @param {Array<[string, { verdict: string, summary: string, findings: Array<{ blocking: boolean }>, frictionFiled?: string[] }]>} list
 * @returns {object[]}
 */
function readEntries(list) {
  return list.map(([n, r]) => ({
    read: n,
    verdict: r.verdict,
    summary: r.summary,
    blocking: r.findings.filter((f) => f.blocking).length,
    ...(n === "fact read" && Array.isArray(r.frictionFiled) ? { frictionFiled: r.frictionFiled } : {})
  }));
}

// === CROSS-REGRESSION DERIVATION (docs-page-chain-derivation.test.mjs extracts this block) ===
/**
 * Derives the cross-regression flag from a page's round records: a reviewer that accepted in
 * round 1 and returned "fix" in round 2. Pure and self-contained (no closures) so the derivation
 * test can extract it verbatim between these markers and run it without the workflow runtime,
 * which has no filesystem access and no module loader for this script (the file's top-level
 * `return` at its end keeps node from importing it directly).
 * @param {{ rounds: Array<{ reads?: Array<{ read: string, verdict: string }> }> }} record
 * @param {boolean} bothReviewers - args.bothReviewers; true when round 2 re-reads every reviewer
 * @returns {true | false | "not-measured" | undefined} undefined when there was no round 2
 */
function deriveCrossRegression(record, bothReviewers) {
  const rounds = record.rounds || [];
  if (rounds.length < 2) return undefined;
  if (!bothReviewers) return "not-measured";
  const round1 = rounds[0].reads || [];
  const round2 = rounds[1].reads || [];
  return round2.some((read2) => {
    const read1 = round1.find((r) => r.read === read2.read);
    return Boolean(read1) && read1.verdict === "accept" && read2.verdict === "fix";
  });
}
// === END CROSS-REGRESSION DERIVATION ===

/**
 * The final reader read, run on a page every read has accepted. Its "fix" sends the page back for
 * one redraft of the cited spots, which then takes the register editor's and the fact read's
 * reads scoped to the changed sentences; a "fix" or a red gate there escalates. Otherwise the
 * reader re-tests once (Part V: "test again"): "accept" accepts the page, "fix" escalates.
 * @param {Record<string, any>} p
 * @param {Record<string, any>} pageInputs
 * @param {Record<string, any>} record
 * @returns {Promise<Record<string, any>>} the page's final record
 */
async function finalRead(p, pageInputs, record) {
  const fr = await agent(readerPrompt(p, pageInputs.pageType || p.pageType), { label: `reader:${p.id}`, phase: "Final read", schema: READER_SCHEMA, model: REVIEWER, agentType: "general-purpose" });
  if (!fr) return { ...record, status: "escalate", reason: "the final reader read returned nothing" };
  record.finalRead = { verdict: fr.verdict, summary: fr.summary, paraphrase: fr.paraphrase, findings: fr.findings || [] };
  if (fr.verdict !== "fix") return { ...record, status: "accepted" };

  const cited = combined([["final reader read", { ...fr, findings: fr.findings || [] }]]);
  const d = await agent(draftPrompt(p, pageInputs, 2, cited, true), { label: `reader-redraft:${p.id}`, phase: "Final read", schema: DRAFT_SCHEMA, model: DRAFTER, agentType: DRAFTER_TYPE });
  if (!d) return { ...record, status: "escalate", reason: "the redraft after the final reader read returned nothing" };
  record.finalRead.redraft = d;
  if (d.gate !== "pass") {
    return { ...record, status: "escalate", reason: "the gate was red after the final reader read's redraft", findings: `## gate: ${d.gate}\n${d.gateTail || ""}` };
  }
  const r = await runReads(p, pageInputs, "final", ["register editor", "fact read"], cited);
  record.finalRead.reads = readEntries(r.list);
  if (r.missing) return { ...record, status: "escalate", reason: `${r.missing} scoped read(s) returned nothing after the final reader read` };
  if (r.anyFix) {
    return { ...record, status: "escalate", reason: "a scoped read returned fix after the final reader read's redraft", findings: combined(r.list) };
  }

  const rt = await agent(readerPrompt(p, pageInputs.pageType || p.pageType), { label: `reader:${p.id}:retest`, phase: "Final read", schema: READER_SCHEMA, model: REVIEWER, agentType: "general-purpose" });
  if (!rt) return { ...record, status: "escalate", reason: "the final reader read's re-test returned nothing" };
  record.finalRead.retest = { verdict: rt.verdict, summary: rt.summary, paraphrase: rt.paraphrase, findings: rt.findings || [] };
  if (rt.verdict !== "fix") return { ...record, status: "accepted" };
  return { ...record, status: "escalate", reason: "the final reader read's re-test returned fix", findings: combined([["final reader read re-test", record.finalRead.retest]]) };
}

// The scope a rework page's redraft holds to, stated in its findings.
const REWORK_SCOPE = "page-level only (introduction, section order, hand-offs, depth, ending, covers); the page plan governs order, placement, and cuts, and a sentence is kept where the plan keeps it; follow the register's page anatomies.";

/**
 * The page inputs a rework page stands in for the page-inputs step with, built from its committed
 * brief so the fact read's inventory check has a baseline: an id the brief cites is carried; an
 * outline id it does not cite is cut, since the page's own draft cut it.
 * @param {Record<string, any>} p
 * @param {{ cited: string[] }} state - the page's rework-state entry
 * @param {string} head - the worktree's short HEAD sha
 * @returns {Record<string, any>}
 */
function reworkInputs(p, state, head) {
  const outlineIds = p.factIds || [];
  const cited = state.cited || [];
  const ids = [...outlineIds, ...cited.filter((id) => !outlineIds.includes(id))];
  return {
    job: p.job,
    pageType: p.pageType || "",
    factIds: ids,
    claimInventory: ids.map((id) => (cited.includes(id)
      ? { claim: "an outline fact for this page", disposition: "carried", factId: id }
      : { claim: "an outline fact for this page", disposition: "cut", factId: id, reason: `cut at the pilot draft (brief at ${head})` }))
  };
}

/**
 * The plan step and its plan read: one agent writes the page plan, the runner writes the plan's
 * dispositions back into the claim inventory, and the structural edit seat reads the plan before
 * any prose. A "fix" takes one plan revision and one re-read; a second "fix" escalates, the shape
 * of the chain's other seats. A plan that leaves a fact id undisposed escalates before its read.
 * @param {Record<string, any>} p
 * @param {Record<string, any>} pageInputs
 * @param {Record<string, any>} record - gains `planStep`
 * @returns {Promise<{ pageInputs: Record<string, any> } | { escalate: Record<string, any> }>} the
 *   page inputs carrying the plan's dispositions, or the escalated record
 */
async function planStep(p, pageInputs, record) {
  const step = { revised: false, reads: [], couldNotDo: [], frictionFiled: [] };
  record.planStep = step;
  const opts = (label) => ({ label, phase: "Plan", schema: PLAN_SCHEMA, model: PLAN_MODEL, effort: PLAN_EFFORT, agentType: "general-purpose" });
  let plan = await agent(planPrompt(p, pageInputs), opts(`plan:${p.id}`));
  for (const tag of ["plan", "plan2"]) {
    if (!plan) return { escalate: { ...record, status: "escalate", reason: "the plan step returned nothing" } };
    step.couldNotDo = plan.couldNotDo || [];
    step.frictionFiled = [...step.frictionFiled, ...(plan.frictionFiled || [])];
    const applied = applyPlan(p, pageInputs, plan.claimInventory);
    if (applied.undisposed.length) {
      return { escalate: { ...record, status: "escalate", reason: `the plan left fact ids undisposed: ${applied.undisposed.join(", ")}` } };
    }
    const read = await agent(structurePrompt(p, "plan"), { label: `structure:${p.id}:${tag}`, phase: "Plan", schema: READ_SCHEMA, model: REVIEWER, agentType: "general-purpose" });
    if (!read) return { escalate: { ...record, status: "escalate", reason: "the plan read returned nothing" } };
    step.reads.push(...readEntries([["structural edit", read]]));
    if (read.verdict !== "fix") {
      // An id the plan places beyond page inputs' list joins the drafter's fact ids.
      const placed = applied.inventory.filter((c) => c.disposition === "carried" && c.factId).map((c) => c.factId);
      const factIds = [...new Set([...(pageInputs.factIds || []), ...placed])];
      return { pageInputs: { ...pageInputs, factIds, claimInventory: applied.inventory } };
    }
    if (tag === "plan2") {
      return { escalate: { ...record, status: "escalate", reason: "second fix verdict on the plan read", findings: combined([["structural edit", read]]) } };
    }
    step.revised = true;
    plan = await agent(planPrompt(p, pageInputs, combined([["structural edit", read]])), opts(`replan:${p.id}`));
  }
}

async function chain(p) {
  const record = { id: p.id, path: p.path, brief: briefPathFor(p), plan: planPathFor(p), rounds: [] };
  record.framing = framingPathFor(p);
  const rework = typeof p.rework === "string" && p.rework !== "";
  let pageInputs;
  if (rework) {
    record.rework = true;
    const state = REWORK_STATE.pages.find((x) => x.path === p.path);
    if (!state) return { ...record, status: "escalate", reason: `the rework-state probe reported nothing for ${p.path}` };
    if (state.state !== "clean") return { ...record, status: "escalate", reason: `rework needs a committed, unchanged page: ${p.path} is ${state.state}` };
    if (state.error) return { ...record, status: "escalate", reason: state.error };
    pageInputs = reworkInputs(p, state, REWORK_STATE.head);
  } else {
    pageInputs = await agent(pageInputsPrompt(p), { label: `inputs:${p.id}`, phase: "Page inputs", schema: PAGE_INPUTS_SCHEMA, model: PAGE_INPUTS_MODEL, agentType: PAGE_INPUTS_TYPE });
    if (!pageInputs) return { ...record, status: "escalate", reason: "page-inputs step returned nothing" };
    record.pageInputs = pageInputs;
  }
  const planned = await planStep(p, pageInputs, record);
  if (planned.escalate) return planned.escalate;
  pageInputs = planned.pageInputs;
  const framed = await framingStep(p, pageInputs, record);
  if (framed.escalate) return framed.escalate;
  pageInputs = framed.pageInputs;
  // A rework page's reads are scoped to what changed; the structural edit seat reads it whole.
  const reworkFindings = rework ? `## rework\n${p.rework}\n\nScope: ${REWORK_SCOPE}` : undefined;
  const reworkWhere = rework
    ? `The page changed at the page level on the rework findings below; run \`git diff -- ${p.path}\` from the worktree to name the changed sentences. If it prints nothing, the rework changed nothing: return "fix" with that finding`
    : undefined;

  const allNames = reviewNames(p);
  const d1 = rework
    ? await agent(draftPrompt(p, pageInputs, 2, reworkFindings), { label: `rework:${p.id}`, phase: "Draft", schema: DRAFT_SCHEMA, model: DRAFTER, agentType: DRAFTER_TYPE })
    : await agent(draftPrompt(p, pageInputs, 1), { label: `draft:${p.id}`, phase: "Draft", schema: DRAFT_SCHEMA, model: DRAFTER, agentType: DRAFTER_TYPE });
  if (!d1) return { ...record, status: "escalate", reason: "drafter returned nothing" };
  record.rounds.push({ round: 1, draft: d1 });
  const r1 = await runReads(p, pageInputs, 1, allNames, reworkFindings, reworkWhere);
  record.rounds[0].reads = readEntries(r1.list);
  // A read that returned nothing is never a silent accept.
  if (r1.missing) return { ...record, status: "escalate", reason: `${r1.missing} read(s) returned nothing in round 1` };
  if (!r1.anyFix && d1.gate === "pass") return finalRead(p, pageInputs, record);

  const rereviewNames = BOTH_REVIEWERS ? allNames : r1.list.filter(([, r]) => r.verdict === "fix").map(([n]) => n);
  const findings = combined(r1.list) + (d1.gate !== "pass" ? `\n\n## gate: ${d1.gate}\n${d1.gateTail || ""}` : "") +
    (rework ? `\n\nScope: ${REWORK_SCOPE}` : "");
  const d2 = await agent(draftPrompt(p, pageInputs, 2, findings, false, true), { label: `redraft:${p.id}`, phase: "Redraft", schema: DRAFT_SCHEMA, model: DRAFTER, agentType: DRAFTER_TYPE });
  if (!d2) return { ...record, status: "escalate", reason: "redrafter returned nothing" };
  // Round 2 narrows to round-1 fixes and changed sentences when the redrafter recorded the page as
  // round 1 read it; without that baseline it falls back to whole-page reads.
  const shaped = typeof d2.baseline === "string" && /^([0-9a-f]{40}|[0-9a-f]{64})$/.test(d2.baseline.trim()) ? d2.baseline.trim() : "";
  const baseline = shaped && (await probeExists(`probe:${p.id}:baseline`, `git cat-file -e ${shaped}`)) ? shaped : "";
  record.rounds.push({ round: 2, draft: d2, scope: baseline ? `changed sentences since ${baseline}` : "whole page (no baseline from the redraft)" });
  if (!baseline) log(`${p.id}: the redraft reported no baseline that resolves; round 2 reads the whole page`);
  const r2 = baseline
    ? await runReads(p, pageInputs, 2, rereviewNames, undefined, undefined, { baseline, round1: r1.list })
    : await runReads(p, pageInputs, 2, rereviewNames, reworkFindings, reworkWhere);
  record.rounds[1].reads = readEntries(r2.list);
  const cr = deriveCrossRegression(record, BOTH_REVIEWERS);
  if (cr !== undefined) record.crossRegression = cr;
  if (r2.missing) return { ...record, status: "escalate", reason: `${r2.missing} read(s) returned nothing in round 2` };
  if (!r2.anyFix && d2.gate === "pass") return finalRead(p, pageInputs, record);
  return { ...record, status: "escalate", reason: "second fix verdict or red gate", findings: combined(r2.list) };
}

// A small slot pool: at most IN_FLIGHT pages drafted at once, no barrier between pages.
const queue = PAGES.slice();
const results = [];
async function worker(n) {
  while (queue.length) {
    const p = queue.shift();
    log(`worker ${n}: ${p.id}`);
    try {
      results.push(await chain(p));
    } catch (e) {
      results.push({ id: p.id, path: p.path, status: "escalate", reason: String(e && e.message || e) });
    }
  }
}
phase("Page inputs");
await parallel(Array.from({ length: Math.min(IN_FLIGHT, PAGES.length) }, (_, i) => () => worker(i + 1)));

phase("Report");
const accepted = results.filter((r) => r.status === "accepted").length;
log(`${accepted}/${PAGES.length} pages accepted; ${PAGES.length - accepted} escalated`);
const spent = HAS_BUDGET ? budget.spent() - SPENT_AT_START : null;
return { pages: results, accepted, escalated: results.filter((r) => r.status !== "accepted").map((r) => r.id), spent };

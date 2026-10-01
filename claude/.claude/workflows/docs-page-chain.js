// Runs the draft-docs page chain (docs/superpowers/specs/2026-09-26-draft-docs-approach-design.md,
// "The page chain" and "Scoped re-review") over a list of pages, a few pages in flight at once.
//
// Per page: a page-inputs agent writes the page's job, its type, the fact ids the drafter should
// draw on, and a claim inventory with a disposition per claim
// (carried by a cited fact, newly filed, or cut with a reason), filing any new fact itself with
// the Edit tool as `[verified]` (a `Source:` line) or `[external]` (the vendor URL), never
// `[candidate]`. The drafter (`cairn-docs-drafter` by default) writes the page and its
// sentence-to-fact brief from that record and files no fact of its own; it runs the docs gate
// itself as its last act and reports the result; it reads the page's exemplar sources whole. The
// structural edit seat, the register editor, and a fact read run in
// parallel, all Opus 5.5, plus a figure-verifier read for a page carrying a figure. One redraft
// round on the combined findings, gated and re-read; by default only the reviewer(s) that
// returned "fix" re-read (`args.bothReviewers` re-reads every reviewer instead and the record
// carries a cross-regression flag: a reviewer that accepted in round 1 and returned "fix" in
// round 2). A second "fix" from a re-reader escalates to the conductor; there is no third round.
// A page every read accepts then takes the final reader read, last. The conductor reads only the
// per-page records returned.
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
//           outOfScope: ["<a topic another page owns>"]   // optional
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
// Design friction: page inputs, the drafter, and the fact read, the agents that meet the code,
// file a friction entry in `docs/internal/docs-friction-log.md` for a hedge, a caveat, an
// exception, a workaround, a surprising default, or two seams naming or behaving the same thing
// differently, naming the fact ids or `file:line` involved, and list it in `frictionFiled`. An
// entry never blocks the page. The record carries page inputs' list in `record.pageInputs`, the
// drafter's in `rounds[].draft`, and the fact read's, copied by the runner, on its entry in
// `rounds[].reads`; the register editor and the figure verifier do not report friction.
//
// The drafter gets the outline entry's `title` as the page's H1 (the title is in the entry
// checksum), the voice source (the register's drafting brief and its primary exemplar only; the
// page's exemplars give structure and detail per step), and, on a figure page, the
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
// not the page. It
// reads the whole page against its outline entry (`job`, `covers`, `outOfScope`, and the
// outline's `crossLinks` from the page's slug, read live) and the register's page anatomies, and
// never edits. Its findings join the round's like the other reads', and `bothReviewers`
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
// The drafter writes the introduction, the section hand-off lead-ins, and the ending the
// register's page anatomies require, records each `no-claim` in the brief when it carries no
// extractable fact, and cites the fact id when it does.
//
// The return carries `spent`: the runtime `budget.spent()` delta across the run, in its unit of
// output tokens spent across the main loop and all workflows, so a relative measure only and
// never a count against a pass ceiling. It is null when the runtime supplies no `budget`.
//
// Every agent starts with zero context. The runner renders each stage's prompt from args and
// the page record; nothing load-bearing may live only in the conductor's conversation.

export const meta = {
  name: "docs-page-chain",
  description: "Drafts docs pages through page inputs, drafter, gate, register editor, fact read, and one scoped redraft.",
  whenToUse: "A draft-docs pass plan names this workflow for its page tasks.",
  phases: [
    { title: "Outline", detail: "with args.outline, one probe resolves every page's outline entry" },
    { title: "Page inputs", detail: "one agent per page: job, type, fact ids, claim inventory" },
    { title: "Draft", detail: "the drafter writes the page and runs the docs gate itself" },
    { title: "Read", detail: "structural edit, register editor, and fact read in parallel, all Opus, plus a figure read when the page carries one" },
    { title: "Redraft", detail: "one round on the combined findings; only the reviewer(s) that returned fix re-read by default" },
    { title: "Final read", detail: "one reader read on an accepted page; its fix takes one scoped redraft, scoped reads, and one re-test" },
    { title: "Report", detail: "per-page records for the conductor" }
  ]
};

const CLAIM = {
  type: "object",
  properties: {
    claim: { type: "string" },
    disposition: { type: "string", enum: ["carried", "filed", "cut", "re-pointed"] },
    factId: { type: "string" },
    reason: { type: "string" }
  },
  required: ["claim", "disposition"]
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
 * The drafter's prompt for a first draft, the round-2 redraft, or the one redraft after the final
 * reader read, which changes only the spots that read cites.
 * @param {Record<string, any>} p
 * @param {Record<string, any>} pageInputs
 * @param {number} round
 * @param {string} [findings] - the combined findings a redraft works from
 * @param {boolean} [afterFinalRead] - the redraft is the final reader read's
 * @returns {string}
 */
function draftPrompt(p, pageInputs, round, findings, afterFinalRead = false) {
  const head = round === 1
    ? `Draft the page ${p.path} at its final path, from the page inputs below and nothing else.`
    : afterFinalRead
      ? `Redraft ${p.path} once, on the final reader read's findings below. Change only the spots they cite and leave the rest of the page as it stands.`
      : `Redraft ${p.path} once, on the combined findings below. Fix every blocking finding; take a non-blocking one when it is right. Do not widen the page.`;
  const inventory = (pageInputs.claimInventory || [])
    .map((c) => `- [${c.disposition}] ${c.claim}${c.factId ? ` (${c.factId})` : ""}${c.reason ? ` -- ${c.reason}` : ""}`)
    .join("\n");
  return `${head}

${common}
${registerLine(p, "drafter")}
The brief is the source of the page's structure and voice. Voice comes only from the register's
drafting brief and its primary exemplar, docs/extend/choose-an-ai-posture.md; the page's exemplars
below supply structure and detail per step, never voice or wording.
${p.title ? `\nThe page's H1, verbatim: # ${p.title}\n` : ""}
The page's job: ${pageInputs.job}
Page type: ${pageInputs.pageType}
${scopeLines(p)}
Exemplar sources: read each in full, and imitate its anatomy and its detail per step, never its
voice, wording, terms, or product names:
${exemplarList(p) || "(none named; follow the register's anatomy)"}
${p.figure ? `\nThis page carries a figure: read and follow the skill file ~/.claude/skills/cairn-figure/SKILL.md.\n` : ""}${p.figure && p.figureNote ? `\nFigure note, from the outline: ${p.figureNote}\n` : ""}

Fact ids to draw on: ${(pageInputs.factIds || []).join(", ") || "(none)"}
Claim inventory, one disposition per claim (a "cut" or "re-pointed" claim stays off the page):
${inventory || "(the page is new; no prior claims to carry)"}
${p.pinned && p.pinned.length ? `Pinned heading slugs this page must keep, verbatim: ${p.pinned.join(", ")}` : ""}
${round > 1 ? `\nCombined findings from the reads:\n${findings}\n` : ""}
Write the introduction, the section hand-off lead-ins, and the ending the register's page anatomies
require for this page's type; the removal rule does not cut them. In the brief, record each of
those sentences "no-claim" when it carries no extractable fact, and cite its fact id when it does:
scripts/checks/check-provenance.mjs fails a no-claim sentence that holds one ("a no-claim sentence
cites nothing, so any extractable fact in it fails").

Write the page's sentence-to-fact brief alongside the page at ${briefPathFor(p)}, citing only the
fact ids above or "no-claim". File no fact yourself, new or retagged. A claim the page needs whose
fact is not among the ids above is a couldNotDo naming the missing fact; do not draft that claim
and do not file its fact yourself, since the conductor re-runs page inputs for it.

${frictionLine()}

Commit nothing; leave the tree with your edits in place.

${indexLinkLine(p)}
${gateLine(p)}

Return the structured report only.`;
}

/**
 * The scope line for a register or fact read after the final reader read's redraft: the parent
 * spec's rule for an edit after the chain, and the spots that changed. Empty for a chain round.
 * @param {string} [scope] - the final reader read's findings, as the redraft received them
 * @returns {string}
 */
function scopeNote(scope) {
  if (!scope) return "";
  return `
This read is scoped. The page changed only at the spots the final reader read cites below, and
"Changed sentences get both reviews, scoped to those sentences"
(docs/superpowers/specs/2026-09-26-draft-docs-approach-design.md, "Edits after the chain"). Grade
the changed sentences.

${scope}
`;
}

function editorPrompt(p, scope) {
  return `Adversarial register edit of ${p.path} in ${WT}.

${common}
${registerLine(p, "editor")}
${scopeNote(scope)}
Run plain \`vale ${p.path}\` from the worktree, never the docs gate, and read every alert it
prints. Read the page, then grade the brief's structure checklist, the brief's tells, and Vale's
alerts together, with the Names section, logic, and facts-adjacent phrasing. Return ranked
findings with a proposed rewrite each, and a verdict: "fix" if any finding is blocking.`;
}

function factPrompt(p, pageInputs, scope) {
  const inventory = (pageInputs.claimInventory || [])
    .map((c) => `- [${c.disposition}] ${c.claim}${c.factId ? ` (${c.factId})` : ""}${c.reason ? ` -- ${c.reason}` : ""}`)
    .join("\n");
  return `Fact read of ${p.path} in ${WT}. The page's claim inventory from its page-inputs step:
${inventory || "(none recorded)"}
${outlineCoverage(p)}${scopeNote(scope)}
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
 * a reason, and an absorbed page's facts are not silently dropped. Empty without outline ids or
 * absorbed pages.
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
inventory above as "cut" with a reason. An outline id that is neither is a blocking finding.`
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

function figurePrompt(p) {
  return `Verify every figure on ${p.path} in ${WT} against the two figure tests and the
2026-08-15 visual-layer rulings. A failed figure is a blocking finding. Verdict "fix" if any
figure fails; otherwise "accept".${p.figureNote ? `\n\nFigure note, from the outline: ${p.figureNote}` : ""}`;
}

// The prior-art record the structural edit seat and the final reader read quote: the only source
// of their checklist and test wording.
const PRIOR_ART = "docs/superpowers/research/2026-09-30-page-level-review-prior-art.md";

/**
 * The structural edit seat's prompt: the two published blocks the prior-art record adopts in
 * "## b. Structural edit seat", quoted verbatim, run over the whole page against its outline
 * entry, with the type's introduction duties from the register's page anatomies.
 * @param {Record<string, any>} p
 * @returns {string}
 */
function structurePrompt(p) {
  const bullets = (xs) => (xs || []).map((x) => `- ${x}`).join("\n") || "(none listed)";
  const crossLinks = OUTLINE
    ? `Cross-links: read the top-level crossLinks array in ${OUTLINE} live, and take every entry
whose "from" is "${slugOf(p)}"; each names a page this one links ("to") and why.`
    : "";
  return `Structural edit of ${p.path} in ${WT}.

${common}
The level, as the Editorial Freelancers Association defines it: "Developmental editors (also called
'substantive,' 'structural,' or 'content' editors) deal with content, organization, and genre
considerations." Your stance, from ${PRIOR_ART}, "## b. Structural edit seat": "an editor. It reads
the whole page against the outline and this checklist, returns findings with \`file:line\`, and never
edits." Grade the page at this level only.

The page's outline entry${OUTLINE ? `, from ${OUTLINE}` : ""}:
Job: ${p.job}
Page type: ${p.pageType || "(none given)"}
What the page covers:
${bullets(p.covers)}
Out of scope, owned by another page:
${bullets(p.outOfScope)}
${crossLinks}

How the entry is read, from the same record: "the outline's \`job\` is the page's purpose, \`covers\`
is the list the page must deliver in the order the seat judges (Block 1 "most logical order"), and
\`outOfScope\` is the page's "doesn't cover" list; the seat compares the introduction to those three."
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

A checklist item the page fails is a blocking finding: its file:line as location, the item it fails
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
 * @returns {Promise<{ list: Array<[string, any]>, missing: number, anyFix: boolean }>}
 */
async function runReads(p, pageInputs, round, names, scope) {
  const tag = round === "final" ? "final" : `r${round}`;
  const phaseName = round === "final" ? "Final read" : "Read";
  const tasks = [];
  if (names.includes("structural edit")) {
    tasks.push(["structural edit", () => agent(structurePrompt(p), { label: `structure:${p.id}:${tag}`, phase: phaseName, schema: READ_SCHEMA, model: REVIEWER, agentType: "general-purpose" })]);
  }
  if (names.includes("register editor")) {
    tasks.push(["register editor", () => agent(editorPrompt(p, scope), { label: `editor:${p.id}:${tag}`, phase: phaseName, schema: READ_SCHEMA, model: REVIEWER, agentType: "cairn-register-editor" })]);
  }
  if (names.includes("fact read")) {
    tasks.push(["fact read", () => agent(factPrompt(p, pageInputs, scope), { label: `facts:${p.id}:${tag}`, phase: phaseName, schema: READ_SCHEMA, model: REVIEWER, agentType: "general-purpose" })]);
  }
  if (names.includes("figure verifier")) {
    tasks.push(["figure verifier", () => agent(figurePrompt(p), { label: `figure:${p.id}:${tag}`, phase: phaseName, schema: READ_SCHEMA, model: REVIEWER, agentType: "figure-verifier" })]);
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
  const r = await runReads(p, pageInputs, "final", ["register editor", "fact read"], cited);
  record.finalRead.reads = readEntries(r.list);
  if (r.missing) return { ...record, status: "escalate", reason: `${r.missing} scoped read(s) returned nothing after the final reader read` };
  if (r.anyFix || d.gate !== "pass") {
    return { ...record, status: "escalate", reason: "a scoped read returned fix or the gate was red after the final reader read's redraft", findings: combined(r.list) };
  }

  const rt = await agent(readerPrompt(p, pageInputs.pageType || p.pageType), { label: `reader:${p.id}:retest`, phase: "Final read", schema: READER_SCHEMA, model: REVIEWER, agentType: "general-purpose" });
  if (!rt) return { ...record, status: "escalate", reason: "the final reader read's re-test returned nothing" };
  record.finalRead.retest = { verdict: rt.verdict, summary: rt.summary, paraphrase: rt.paraphrase, findings: rt.findings || [] };
  if (rt.verdict !== "fix") return { ...record, status: "accepted" };
  return { ...record, status: "escalate", reason: "the final reader read's re-test returned fix", findings: combined([["final reader read re-test", record.finalRead.retest]]) };
}

async function chain(p) {
  const record = { id: p.id, path: p.path, brief: briefPathFor(p), rounds: [] };
  const pageInputs = await agent(pageInputsPrompt(p), { label: `inputs:${p.id}`, phase: "Page inputs", schema: PAGE_INPUTS_SCHEMA, model: PAGE_INPUTS_MODEL, agentType: PAGE_INPUTS_TYPE });
  if (!pageInputs) return { ...record, status: "escalate", reason: "page-inputs step returned nothing" };
  record.pageInputs = pageInputs;

  const allNames = reviewNames(p);
  const d1 = await agent(draftPrompt(p, pageInputs, 1), { label: `draft:${p.id}`, phase: "Draft", schema: DRAFT_SCHEMA, model: DRAFTER, agentType: DRAFTER_TYPE });
  if (!d1) return { ...record, status: "escalate", reason: "drafter returned nothing" };
  record.rounds.push({ round: 1, draft: d1 });
  const r1 = await runReads(p, pageInputs, 1, allNames);
  record.rounds[0].reads = readEntries(r1.list);
  // A read that returned nothing is never a silent accept.
  if (r1.missing) return { ...record, status: "escalate", reason: `${r1.missing} read(s) returned nothing in round 1` };
  if (!r1.anyFix && d1.gate === "pass") return finalRead(p, pageInputs, record);

  const rereviewNames = BOTH_REVIEWERS ? allNames : r1.list.filter(([, r]) => r.verdict === "fix").map(([n]) => n);
  const findings = combined(r1.list) + (d1.gate !== "pass" ? `\n\n## gate: ${d1.gate}\n${d1.gateTail || ""}` : "");
  const d2 = await agent(draftPrompt(p, pageInputs, 2, findings), { label: `redraft:${p.id}`, phase: "Redraft", schema: DRAFT_SCHEMA, model: DRAFTER, agentType: DRAFTER_TYPE });
  if (!d2) return { ...record, status: "escalate", reason: "redrafter returned nothing" };
  record.rounds.push({ round: 2, draft: d2 });
  const r2 = await runReads(p, pageInputs, 2, rereviewNames);
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

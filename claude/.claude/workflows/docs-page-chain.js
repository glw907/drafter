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
// register editor and a fact read run in
// parallel, both Opus 5.5, plus a figure-verifier read for a page carrying a figure. One redraft
// round on the combined findings, gated and re-read; by default only the reviewer(s) that
// returned "fix" re-read (`args.bothReviewers` re-reads every reviewer instead and the record
// carries a cross-regression flag: a reviewer that accepted in round 1 and returned "fix" in
// round 2). A second "fix" from a re-reader escalates to the conductor; there is no third round.
// The conductor reads only the per-page records returned.
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
// `pages: [{ slug, path, group, order, job, pageType, exemplars: [{ source }], figure, factIds,
// covers, outOfScope, pinned }]`. With it, each entry in `args.pages` needs only `id`, `path`, and
// `track`: its `job`, `pageType`, `exemplarSources`, `factIds`, `covers`, `outOfScope`, `pinned`,
// and `figure` come from the outline page whose `path` matches, and any of those fields given
// inline in `args.pages` overrides the outline's. An exemplar `source` is a capture directory under
// `~/.local/share/cairn/exemplars/`, read as the `page.md` inside it. A page path the outline does
// not carry fails the run by name before any page agent starts.
//
// The workflow runtime has no filesystem or exec access, so the outline is read by the
// `cairn-docs-outline` helper (`~/.local/bin`, tested by
// `~/.dotfiles/tests/docs-page-chain-outline.test.mjs`): one probe agent runs its `resolve`
// command before any page agent and returns the resolved entries, each with a checksum the runner
// recomputes, so a probe that mistranscribes an entry fails the run rather than drafting from it.
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
    { title: "Read", detail: "register editor and fact read in parallel, both Opus, plus a figure read when the page carries one" },
    { title: "Redraft", detail: "one round on the combined findings; only the reviewer(s) that returned fix re-read by default" },
    { title: "Report", detail: "per-page records for the conductor" }
  ]
};

const CLAIM = {
  type: "object",
  properties: {
    claim: { type: "string" },
    disposition: { type: "string", enum: ["carried", "filed", "cut"] },
    factId: { type: "string" },
    reason: { type: "string" }
  },
  required: ["claim", "disposition"]
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
    couldNotDo: { type: "array", items: { type: "string" } }
  },
  required: ["job", "pageType", "factIds", "claimInventory"]
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
    job: { type: "string" },
    pageType: { type: "string" },
    exemplarSources: { type: "array", items: { type: "string" } },
    factIds: { type: "array", items: { type: "string" } },
    covers: { type: "array", items: { type: "string" } },
    outOfScope: { type: "array", items: { type: "string" } },
    pinned: { type: "array", items: { type: "string" } },
    figure: { type: "boolean" },
    checksum: { type: "string" }
  },
  required: ["path", "job", "pageType", "exemplarSources", "factIds", "covers", "outOfScope", "pinned", "figure", "checksum"]
};

const OUTLINE_PROBE_SCHEMA = {
  type: "object",
  properties: {
    ok: { type: "boolean" },
    index: { type: "string" },
    missing: { type: "array", items: { type: "string" } },
    entries: { type: "array", items: OUTLINE_ENTRY },
    error: { type: "string" }
  },
  required: ["ok", "missing", "entries"]
};

const READ_SCHEMA = {
  type: "object",
  properties: {
    verdict: { type: "string", enum: ["accept", "fix"] },
    findings: { type: "array", items: FINDING },
    summary: { type: "string" }
  },
  required: ["verdict", "findings", "summary"]
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
    job: String(e.job || ""),
    pageType: String(e.pageType || ""),
    exemplarSources: list(e.exemplarSources),
    factIds: list(e.factIds),
    covers: list(e.covers),
    outOfScope: list(e.outOfScope),
    pinned: list(e.pinned),
    figure: e.figure === true
  };
}

/**
 * FNV-1a over the canonical entry's JSON, as eight hex digits.
 * @param {Record<string, unknown>} e
 * @returns {string}
 */
function checksumOf(e) {
  const s = JSON.stringify(canonicalEntry(e));
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return (h >>> 0).toString(16).padStart(8, "0");
}

/**
 * Checks the outline probe's report against the requested pages and merges each page's entry
 * under its inline fields: a field `args.pages` gives inline wins over the outline's.
 * @param {Array<Record<string, any>>} pages - args.pages
 * @param {{ ok?: boolean, missing?: string[], entries?: Array<Record<string, any>>, error?: string } | null} probe
 * @param {string} outlinePath - args.outline, for the error message
 * @returns {Array<Record<string, any>>}
 * @throws {Error} naming every page path the outline does not carry, a failed probe, or an entry
 *   whose checksum does not match its fields
 */
function mergeOutline(pages, probe, outlinePath) {
  if (!probe) throw new Error(`docs-page-chain: the outline probe for ${outlinePath} returned nothing`);
  const entries = probe.entries || [];
  const missing = pages.map((p) => p.path).filter((path) => !entries.some((e) => e.path === path));
  for (const path of probe.missing || []) if (!missing.includes(path)) missing.push(path);
  if (missing.length) throw new Error(`docs-page-chain: not in the outline ${outlinePath}: ${missing.join(", ")}`);
  if (!probe.ok) throw new Error(`docs-page-chain: the outline probe failed: ${probe.error || "no error given"}`);
  const fields = ["job", "pageType", "exemplarSources", "factIds", "covers", "outOfScope", "pinned", "figure"];
  return pages.map((p) => {
    const entry = entries.find((e) => e.path === p.path);
    if (checksumOf(entry) !== entry.checksum) {
      throw new Error(`docs-page-chain: the outline probe mistranscribed ${p.path} (checksum ${checksumOf(entry)}, reported ${entry.checksum})`);
    }
    const merged = { ...p };
    for (const f of fields) if (merged[f] === undefined) merged[f] = canonicalEntry(entry)[f];
    return merged;
  });
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
  return { pages: mergeOutline(PAGES_ARG, probe, OUTLINE), index: (probe && probe.index) || "" };
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
${startingInventory(p)}
Read the page at ${p.path} if it already exists on disk, for its claims only, never its prose or
structure: every command, step, transcript, figure, warning, success signal, and prose assertion
is a claim. If the page does not exist yet, there are no claims to inventory; say so and move on.
Give each claim exactly one disposition:
- "carried": a fact bullet in the container already covers it exactly. Cite the fact id.
- "filed": you traced it to code, config, or a vendor doc and filed a new bullet for it with the
  Edit tool (never a shell append), tagged [verified] with a Source: line, or [external] with the
  vendor's URL for a Cloudflare or GitHub step. Cite the new fact id.
- "cut": the claim does not survive the page; give the reason.

A cited fact whose only source is an arm page is retraced to code, config, or a vendor doc first;
when that fails, retag it [candidate] and do not cite it. Never file a fact tagged [candidate] or
[docs-drift] yourself, and never retag any fact except that one narrow case; the independent fact
read does the rest of the retagging, not you.

Inputs to trace claims against (read each in full; the only source of a command, a transcript, a
JSON example, or a fact; never open the old page's own prose for a claim's wording):
${(p.inputs || []).map((i) => `- ${i}`).join("\n")}

Return the structured report only: the page's job (one paragraph), its page type, every fact id
the drafter should draw on, each id you added with why, the full claim inventory, and any new fact
id you filed.`;
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

function draftPrompt(p, pageInputs, round, findings) {
  const head = round === 1
    ? `Draft the page ${p.path} at its final path, from the page inputs below and nothing else.`
    : `Redraft ${p.path} once, on the combined findings below. Fix every blocking finding; take a non-blocking one when it is right. Do not widen the page.`;
  const inventory = (pageInputs.claimInventory || [])
    .map((c) => `- [${c.disposition}] ${c.claim}${c.factId ? ` (${c.factId})` : ""}${c.reason ? ` -- ${c.reason}` : ""}`)
    .join("\n");
  return `${head}

${common}
${registerLine(p, "drafter")}
The brief is the source of the page's structure and voice.

The page's job: ${pageInputs.job}
Page type: ${pageInputs.pageType}
${scopeLines(p)}
Exemplar sources: read each in full, and imitate its anatomy and rhythm, never its wording, terms,
or product names:
${(p.exemplarSources || []).map((e) => `- ${e}`).join("\n") || "(none named; follow the register's anatomy)"}

Fact ids to draw on: ${(pageInputs.factIds || []).join(", ") || "(none)"}
Claim inventory, one disposition per claim:
${inventory || "(the page is new; no prior claims to carry)"}
${p.pinned && p.pinned.length ? `Pinned heading slugs this page must keep, verbatim: ${p.pinned.join(", ")}` : ""}
${round > 1 ? `\nCombined findings from the reads:\n${findings}\n` : ""}
Write the page's sentence-to-fact brief alongside the page at ${briefPathFor(p)}, citing only the
fact ids above or "no-claim". File no fact yourself, new or retagged. A claim the page needs whose
fact is not among the ids above is a couldNotDo naming the missing fact; do not draft that claim
and do not file its fact yourself, since the conductor re-runs page inputs for it. Something no
source can supply at all, a genuine design gap rather than a missing fact, goes to
docs/internal/docs-friction-log.md (name it in frictionFiled). Commit nothing; leave the tree with
your edits in place.

${indexLinkLine(p)}
${gateLine(p)}

Return the structured report only.`;
}

function editorPrompt(p) {
  return `Adversarial register edit of ${p.path} in ${WT}.

${common}
${registerLine(p, "editor")}

Run plain \`vale ${p.path}\` from the worktree, never the docs gate, and read every alert it
prints. Read the page, then grade the brief's structure checklist, the brief's tells, and Vale's
alerts together, with the Names section, logic, and facts-adjacent phrasing. Return ranked
findings with a proposed rewrite each, and a verdict: "fix" if any finding is blocking.`;
}

function factPrompt(p, pageInputs) {
  const inventory = (pageInputs.claimInventory || [])
    .map((c) => `- [${c.disposition}] ${c.claim}${c.factId ? ` (${c.factId})` : ""}`)
    .join("\n");
  return `Fact read of ${p.path} in ${WT}. The page's claim inventory from its page-inputs step:
${inventory || "(none recorded)"}

Check every claim on the page against its cited fact id, retrace every cited fact against its
source and fix or retag it [docs-drift] in the same chain when it no longer matches, and confirm
every claim the inventory marks "carried" or "filed" still appears on the page. A claim on the
page with no cited fact behind it, a cited fact that no longer matches its source, or an inventory
claim the page dropped without a "cut" disposition is a blocking finding (location, claim, what is
wrong). Verdict "fix" if any blocking finding exists; otherwise "accept" with the count of claims
traced.`;
}

function figurePrompt(p) {
  return `Verify every figure on ${p.path} in ${WT} against the two figure tests and the
2026-08-15 visual-layer rulings. A failed figure is a blocking finding. Verdict "fix" if any
figure fails; otherwise "accept".`;
}

function combined(readList) {
  return readList
    .map(([name, r]) => `## ${name}: ${r.verdict}\n${r.summary}\n` +
      r.findings.map((f) => `- [${f.blocking ? "BLOCKING" : "advisory"}] ${f.location}: ${f.finding}${f.rewrite ? `\n  rewrite: ${f.rewrite}` : ""}`).join("\n"))
    .join("\n\n");
}

/**
 * Every reviewer this page carries: the two standing reads, plus a figure-verifier read for a
 * page with `p.figure` set.
 * @param {{ figure?: boolean }} p
 * @returns {string[]}
 */
function reviewNames(p) {
  return ["register editor", "fact read", ...(p.figure ? ["figure verifier"] : [])];
}

async function runReads(p, pageInputs, round, names) {
  const tasks = [];
  if (names.includes("register editor")) {
    tasks.push(["register editor", () => agent(editorPrompt(p), { label: `editor:${p.id}:r${round}`, phase: "Read", schema: READ_SCHEMA, model: REVIEWER, agentType: "cairn-register-editor" })]);
  }
  if (names.includes("fact read")) {
    tasks.push(["fact read", () => agent(factPrompt(p, pageInputs), { label: `facts:${p.id}:r${round}`, phase: "Read", schema: READ_SCHEMA, model: REVIEWER, agentType: "general-purpose" })]);
  }
  if (names.includes("figure verifier")) {
    tasks.push(["figure verifier", () => agent(figurePrompt(p), { label: `figure:${p.id}:r${round}`, phase: "Read", schema: READ_SCHEMA, model: REVIEWER, agentType: "figure-verifier" })]);
  }
  const results = await parallel(tasks.map(([, fn]) => fn));
  const list = tasks.map(([name], i) => [name, results[i]]).filter(([, r]) => r);
  const missing = tasks.length - list.length;
  const anyFix = list.some(([, r]) => r.verdict === "fix");
  return { list, missing, anyFix };
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
  record.rounds[0].reads = r1.list.map(([n, r]) => ({ read: n, verdict: r.verdict, summary: r.summary, blocking: r.findings.filter((f) => f.blocking).length }));
  // A read that returned nothing is never a silent accept.
  if (r1.missing) return { ...record, status: "escalate", reason: `${r1.missing} read(s) returned nothing in round 1` };
  if (!r1.anyFix && d1.gate === "pass") return { ...record, status: "accepted" };

  const rereviewNames = BOTH_REVIEWERS ? allNames : r1.list.filter(([, r]) => r.verdict === "fix").map(([n]) => n);
  const findings = combined(r1.list) + (d1.gate !== "pass" ? `\n\n## gate: ${d1.gate}\n${d1.gateTail || ""}` : "");
  const d2 = await agent(draftPrompt(p, pageInputs, 2, findings), { label: `redraft:${p.id}`, phase: "Redraft", schema: DRAFT_SCHEMA, model: DRAFTER, agentType: DRAFTER_TYPE });
  if (!d2) return { ...record, status: "escalate", reason: "redrafter returned nothing" };
  record.rounds.push({ round: 2, draft: d2 });
  const r2 = await runReads(p, pageInputs, 2, rereviewNames);
  record.rounds[1].reads = r2.list.map(([n, r]) => ({ read: n, verdict: r.verdict, summary: r.summary, blocking: r.findings.filter((f) => f.blocking).length }));
  const cr = deriveCrossRegression(record, BOTH_REVIEWERS);
  if (cr !== undefined) record.crossRegression = cr;
  if (r2.missing) return { ...record, status: "escalate", reason: `${r2.missing} read(s) returned nothing in round 2` };
  if (!r2.anyFix && d2.gate === "pass") return { ...record, status: "accepted" };
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
return { pages: results, accepted, escalated: results.filter((r) => r.status !== "accepted").map((r) => r.id) };

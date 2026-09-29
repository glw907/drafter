// Runs the draft-docs page chain (docs/superpowers/specs/2026-09-26-draft-docs-approach-design.md,
// "The page chain" and "Scoped re-review") over a list of pages, a few pages in flight at once.
//
// Per page: a page-inputs agent writes the page's job, its type, two trimmed exemplar excerpts,
// the fact ids the drafter should draw on, and a claim inventory with a disposition per claim
// (carried by a cited fact, newly filed, or cut with a reason), filing any new fact itself with
// the Edit tool as `[verified]` (a `Source:` line) or `[external]` (the vendor URL), never
// `[candidate]`. The drafter (`cairn-docs-drafter` by default) writes the page and its
// sentence-to-fact brief from that record and files no fact of its own; it runs the docs gate
// itself as its last act and reports the result. The register editor and a fact read run in
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
//       registerPaths: ["docs/internal/docs-register.md#..."],   // what the register editor reads
//       valeErrorRules: "<the error-tier rule list, verbatim>",
//       drafterModel: "claude-opus-5-5",   // optional; default
//       reviewModel: "claude-opus-5-5",    // optional; default, also the page-inputs step's model
//       pageInputsModel: "claude-opus-5-5", // optional; defaults to reviewModel
//       pages: [
//         {
//           id: "is-it-working",
//           path: "docs/admin/is-it-working.md",
//           track: "admin",                // the page's docs track; locates its brief file
//           job: "<the page's job and type, from the stage outline, verbatim>",
//           pageType: "how-to",            // optional hint; the page-inputs agent may confirm or correct it
//           exemplarSources: ["docs/admin/x.md", "docs/editors/y.md"],  // two full pages to excerpt
//           inputs: ["docs/superpowers/plans/<plan>.mining.md#is-it-working", "..."],
//           pinned: ["#slug-one", "#slug-two"],   // optional; slugs the page must keep
//           extraChecks: ["<a sentence the fact read must also verify>"],   // optional
//           figure: false                  // optional; true adds a figure-verifier read
//         }
//       ]
//     }
//   })
//
// Every agent starts with zero context. The runner renders each stage's prompt from args and
// the page record; nothing load-bearing may live only in the conductor's conversation.

export const meta = {
  name: "docs-page-chain",
  description: "Drafts docs pages through page inputs, drafter, gate, register editor, fact read, and one scoped redraft.",
  whenToUse: "A draft-docs pass plan names this workflow for its page tasks.",
  phases: [
    { title: "Page inputs", detail: "one agent per page: job, type, exemplar excerpts, fact ids, claim inventory" },
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
    exemplarExcerpts: {
      type: "array",
      items: {
        type: "object",
        properties: { source: { type: "string" }, excerpt: { type: "string" } },
        required: ["source", "excerpt"]
      }
    },
    factIds: { type: "array", items: { type: "string" } },
    claimInventory: { type: "array", items: CLAIM },
    factsFiled: { type: "array", items: { type: "string" } },
    couldNotDo: { type: "array", items: { type: "string" } }
  },
  required: ["job", "pageType", "exemplarExcerpts", "factIds", "claimInventory"]
};

const DRAFT_SCHEMA = {
  type: "object",
  properties: {
    path: { type: "string" },
    gate: { type: "string", enum: ["pass", "fail", "not run"] },
    gateCommand: { type: "string" },
    gateTail: { type: "string" },
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
const PAGES = a.pages || [];

if (!WT || !GATE || !PAGES.length) {
  throw new Error("docs-page-chain needs args.worktree, args.gate, and args.pages");
}

const common = `Work only in the worktree ${WT}; run every command from there and never cd to another checkout.
The register is ${(a.registerPaths || ["docs/internal/docs-register.md"]).join(", ")}: read the universal
contract and the track section before anything else. The Names convention in that file's "Names"
section is enforced by Vale (Cairn.Names, Cairn.NamesRetired); use the sanctioned name for every
part.

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

Read these two exemplar sources in full and trim each to the excerpt this page's type should
imitate for anatomy, register, and per-step detail, never the exemplar's own content, terms, or
product names:
${(p.exemplarSources || []).map((e) => `- ${e}`).join("\n")}

Inputs to trace claims against (read each in full; the only source of a command, a transcript, a
JSON example, or a fact; never open the old page's own prose for a claim's wording):
${(p.inputs || []).map((i) => `- ${i}`).join("\n")}

Return the structured report only: the page's job (one paragraph), its page type, the two trimmed
exemplar excerpts (with their source path), every fact id the drafter should draw on, the full
claim inventory, and any new fact id you filed.`;
}

function draftPrompt(p, pageInputs, round, findings) {
  const head = round === 1
    ? `Draft the page ${p.path} at its final path, from the page inputs below and nothing else.`
    : `Redraft ${p.path} once, on the combined findings below. Fix every blocking finding; take a non-blocking one when it is right. Do not widen the page.`;
  const excerpts = (pageInputs.exemplarExcerpts || [])
    .map((e) => `<example source="${e.source}">\n${e.excerpt}\n</example>`)
    .join("\n\n");
  const inventory = (pageInputs.claimInventory || [])
    .map((c) => `- [${c.disposition}] ${c.claim}${c.factId ? ` (${c.factId})` : ""}${c.reason ? ` -- ${c.reason}` : ""}`)
    .join("\n");
  return `${head}

${common}
The page's job: ${pageInputs.job}
Page type: ${pageInputs.pageType}

Exemplars to imitate for anatomy and register, never their content, terms, or product names:
${excerpts || "(none named; follow the register's anatomy)"}

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

${gateLine(p)}

Return the structured report only.`;
}

function editorPrompt(p) {
  return `Adversarial register edit of ${p.path} in ${WT}. Read the page, then the register's
universal contract and its track section, then grade. Apply the tell catalogue, the Names
section, logic, and facts-adjacent phrasing. Return ranked findings with a proposed rewrite each,
and a verdict: "fix" if any finding is blocking.`;
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

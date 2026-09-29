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
//       registerPath: "docs/internal/docs-register.md",   // optional; the register the page-inputs agent extracts from
//       valeErrorRules: "<the error-tier rule list, verbatim>",   // the conductor derives it from scripts/checks/promoted-docs.json
//       drafterModel: "claude-opus-5-5",   // optional; default
//       reviewModel: "claude-opus-5-5",    // optional; default, also the page-inputs step's model
//       pageInputsModel: "claude-opus-5-5", // optional; defaults to reviewModel
//       pages: [
//         {
//           id: "is-it-working",
//           path: "docs/admin/is-it-working.md",
//           track: "admin",                // editors | admin | extend | reference | front-door | readme; picks the base guide, the brief, and the track section
//           job: "<the page's job and type, from the stage outline, verbatim>",
//           pageType: "how-to",            // optional hint; the page-inputs agent may confirm or correct it
//           exemplarSources: [   // two full pages to excerpt; each names what it may lend the drafter
//             { path: "docs/admin/x.md", role: "anatomy" },   // section order and step form only
//             { path: "docs/editors/y.md", role: "voice" }    // sentence rhythm and voice as well
//           ],
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
//
// The workflow runtime has no filesystem access, so the page-inputs agent extracts the register
// sections: it runs one fixed shell command per section, rendered here, and returns stdout
// verbatim. The runner validates each return and escalates the page on a rejection. The drafter
// receives its brief, Names, Visuals, track section, and anatomies with quote markers stripped
// and no register path; the register editor also receives the layering sections with markers
// kept. scripts/docs-chain-render.mjs renders either prompt from a checkout for the proof run.

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
        properties: {
          source: { type: "string" },
          excerpt: { type: "string" },
          note: { type: "string" }
        },
        required: ["source", "excerpt"]
      }
    },
    registerSections: {
      type: "array",
      items: {
        type: "object",
        properties: { key: { type: "string" }, output: { type: "string" } },
        required: ["key", "output"]
      }
    },
    factIds: { type: "array", items: { type: "string" } },
    claimInventory: { type: "array", items: CLAIM },
    factsFiled: { type: "array", items: { type: "string" } },
    couldNotDo: { type: "array", items: { type: "string" } }
  },
  required: ["job", "pageType", "exemplarExcerpts", "registerSections", "factIds", "claimInventory"]
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
    blocking: { type: "boolean" },
    source: { type: "string", enum: ["guide", "register"] },
    rule: { type: "string" }
  },
  required: ["location", "finding", "blocking", "source"]
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

const REGISTER = a.registerPath || "docs/internal/docs-register.md";

for (const p of PAGES) {
  for (const e of p.exemplarSources || []) {
    if (!e || typeof e.path !== "string" || (e.role !== "anatomy" && e.role !== "voice")) {
      throw new Error(`page ${p.id}: each exemplarSources entry needs { path, role: "anatomy" | "voice" }`);
    }
  }
}

// === REGISTER EXTRACTION AND COERCION (docs-page-chain-derivation.test.mjs extracts this block) ===
// Everything between these markers is pure and self-contained (no closures over the args), so the
// derivation test and scripts/docs-chain-render.mjs can extract it verbatim and run it without
// the workflow runtime.

/**
 * The base style guide a track writes under: `editors` follows Microsoft, every other track
 * (admin, extend, reference, the front door, the root README) follows Google.
 * @param {string} track
 * @returns {"Microsoft" | "Google"}
 */
function baseGuideFor(track) {
  return track === "editors" ? "Microsoft" : "Google";
}

/**
 * The register sections one page's stages read, each with the exact heading that opens it. The
 * headings are the register's own, byte for byte; a section runs to the next heading of the same
 * or higher level.
 * @param {string} track - editors | admin | extend | reference | front-door | readme
 * @returns {Array<{ key: string, heading: string, level: number, min: number }>}
 * @throws {Error} on a track with no register track section
 */
function sectionSpecsFor(track) {
  const trackHeadings = {
    editors: "### The editor track (`docs/editors/`)",
    admin: "### The admin track (`docs/admin/`)",
    extend: "### The extend track (`docs/extend/`)",
    reference: "## The reference (`docs/reference/`), a shared instrument",
    "front-door": "## The front door (`docs/README.md`, `docs/why-cairn.md`, and the root `README.md`)",
    readme: "## The front door (`docs/README.md`, `docs/why-cairn.md`, and the root `README.md`)"
  };
  if (!Object.hasOwn(trackHeadings, track)) throw new Error(`unknown track "${track}": no register track section`);
  const brief = track === "editors" ? "## Drafting brief: editor docs" : "## Drafting brief: developer docs";
  const rows = [
    ["brief", brief, 200],
    ["names", "## Names", 60],
    ["visuals", "## Visuals (every page that carries one)", 60],
    ["track", trackHeadings[track], 60],
    ["anatomies", "## The page anatomies", 60],
    ["provenance", "## Provenance", 60],
    ["tightening", "## The tightening test", 60],
    ["exceptionsGoogle", "## Recorded exceptions: Google", 60],
    ["exceptionsMicrosoft", "## Recorded exceptions: Microsoft", 60]
  ];
  return rows.map(([key, heading, min]) => ({ key, heading, level: heading.match(/^#+/)[0].length, min }));
}

/**
 * The one fixed shell command that extracts a section: the lines from the exact heading up to the
 * next heading of the same or higher level (fenced blocks skipped), trailing blank lines dropped,
 * ending in a sentinel line that carries the key and the line count. The command is one line,
 * so an agent copies it whole.
 * @param {{ key: string, heading: string, level: number }} spec
 * @param {string} wt - the checkout the command runs in
 * @param {string} registerPath - the register, relative to the checkout
 * @returns {string}
 */
function sectionCommand(spec, wt, registerPath) {
  const q = (t) => `'${String(t).replace(/'/g, `'\\''`)}'`;
  const program = [
    'BEGIN { h = ENVIRON["H"]; lvl = ENVIRON["L"] + 0; on = 0; fence = 0; n = 0 }',
    "{",
    "line = $0;",
    'isfence = (line ~ "^[`][`][`]" || line ~ "^~~~");',
    'if (!fence && !isfence && line ~ "^#+ ") {',
    'm = 0; while (substr(line, m + 1, 1) == "#") m++;',
    "if (on && m <= lvl) exit;",
    "if (!on && line == h) on = 1",
    "}",
    "if (isfence) fence = !fence;",
    "if (on) a[n++] = line",
    "}",
    "END {",
    'while (n > 0 && a[n - 1] == "") n--;',
    "for (i = 0; i < n; i++) print a[i];",
    'print "=== cairn-section " ENVIRON["K"] " lines=" n " ==="',
    "}"
  ].join(" ");
  return `cd ${q(wt)} && H=${q(spec.heading)} L=${spec.level} K=${q(spec.key)} awk '${program}' ${q(registerPath)}`;
}

/**
 * Checks one section's stdout as the page-inputs agent returned it. Rejects an empty return, an
 * absent or mismatched sentinel, a line-count mismatch, a missing heading, a wrong first line, a
 * body under the section's minimum length, and a brief with no `q:` marker.
 * @param {{ key: string, heading: string, min: number }} spec
 * @param {unknown} output
 * @returns {{ ok: true, body: string } | { ok: false, reason: string }}
 */
function validateSection(spec, output) {
  if (typeof output !== "string" || !output.trim()) return { ok: false, reason: "empty return" };
  const lines = output.replace(/\r/g, "").replace(/\n+$/, "").split("\n");
  const last = lines[lines.length - 1];
  const sentinel = last.match(/^=== cairn-section (\S+) lines=(\d+) ===$/);
  if (!sentinel || sentinel[1] !== spec.key) return { ok: false, reason: `sentinel line for "${spec.key}" absent or not last` };
  const body = lines.slice(0, -1);
  const declared = Number(sentinel[2]);
  if (body.length !== declared) return { ok: false, reason: `line count mismatch: sentinel says ${declared}, body has ${body.length}` };
  if (declared === 0) return { ok: false, reason: `heading absent from the register: ${spec.heading}` };
  if (body[0] !== spec.heading) return { ok: false, reason: `wrong first line: expected "${spec.heading}", got "${body[0]}"` };
  const text = body.join("\n");
  if (text.length < spec.min) return { ok: false, reason: `body under ${spec.min} characters (${text.length})` };
  if (spec.key === "brief" && !/<!--\s*q:[a-z0-9-]+\s*-->/.test(text)) return { ok: false, reason: "brief carries no q: marker" };
  return { ok: true, body: text };
}

/**
 * Validates the page-inputs agent's full return for a track and gathers the section bodies.
 * @param {string} track
 * @param {Array<{ key: string, output: string }> | undefined} returned
 * @returns {{ ok: true, sections: Record<string, string> } | { ok: false, reason: string }}
 */
function collectSections(track, returned) {
  const sections = {};
  for (const spec of sectionSpecsFor(track)) {
    const entry = (returned || []).find((e) => e && e.key === spec.key);
    if (!entry) return { ok: false, reason: `${spec.key}: missing from the page-inputs return` };
    const v = validateSection(spec, entry.output);
    if (!v.ok) return { ok: false, reason: `${spec.key}: ${v.reason}` };
    sections[spec.key] = v.body;
  }
  return { ok: true, sections };
}

/**
 * Removes the `q:` and `x:` markers, which belong to the reviewers' view of the register.
 * @param {string} text
 * @returns {string}
 */
function stripMarkers(text) {
  return text.replace(/[ \t]*<!--\s*[qx]:[a-z0-9-]+\s*-->/g, "");
}

/**
 * The guide-quote ids a brief carries, each as `q:<id>`.
 * @param {string} briefText
 * @returns {Set<string>}
 */
function briefRuleIds(briefText) {
  return new Set([...String(briefText).matchAll(/<!--\s*(q:[a-z0-9-]+)\s*-->/g)].map((m) => m[1]));
}

/**
 * Applies the finding rules to a register-editor read. A missing or unknown `source` becomes
 * `guide`. A `guide` finding whose `rule` is a `q:` id present in the brief is forced blocking;
 * any other finding keeps the reviewer's value. Any blocking finding then sets the verdict to
 * `fix`. Returns a new read and leaves the input untouched.
 * @param {{ verdict: string, findings: Array<object> }} read
 * @param {string} briefText - the brief with its markers kept
 * @returns {{ verdict: string, findings: Array<object> }}
 */
function coerceRead(read, briefText) {
  const ids = briefRuleIds(briefText);
  const findings = (read.findings || []).map((f) => {
    const source = f.source === "register" ? "register" : "guide";
    const forced = source === "guide" && typeof f.rule === "string" && ids.has(f.rule);
    return { ...f, source, blocking: forced || f.blocking === true };
  });
  return { ...read, findings, verdict: findings.some((f) => f.blocking) ? "fix" : read.verdict };
}

/**
 * Renders the excerpts the page-inputs agent trimmed, each with the role its source was given:
 * an anatomy excerpt lends section order and step form only, a voice excerpt lends its voice too.
 * @param {Array<{ source: string, excerpt: string, note?: string }>} excerpts
 * @param {Array<{ path: string, role: string }>} sources - the page's `exemplarSources`
 * @returns {string}
 */
function renderExemplars(excerpts, sources) {
  return (excerpts || []).map((e) => {
    const listed = (sources || []).find((s) => s.path === e.source);
    const role = listed && listed.role === "voice" ? "voice" : "anatomy";
    const rule = role === "voice"
      ? "Role voice: imitate its sentence rhythm and voice, never its content, terms, or product names."
      : "Role anatomy: imitate the section order and step form only; its sentences are not the voice.";
    const note = e.note ? `\nDeparts from the base guide or the cairn docs voice: ${e.note}` : "";
    return `<example source="${e.source}" role="${role}">\n${rule}${note}\n---\n${e.excerpt}\n</example>`;
  }).join("\n\n");
}

/**
 * The drafter prompt: the page's job and inputs, plus its brief, Names, Visuals, track section,
 * and page anatomies with quote markers stripped. It names no register path and carries no
 * layering.
 * @param {{ wt: string, valeRules?: string, page: { path: string, track: string, pinned?: string[] }, pageInputs: object, sections: Record<string, string>, exemplarSources?: Array<{ path: string, role: string }>, briefPath: string, round: number, findings?: string, gateText: string }} ctx
 * @returns {string}
 */
function renderDrafterPrompt(ctx) {
  const { wt, page: p, pageInputs, sections, round, findings } = ctx;
  const head = round === 1
    ? `Draft the page ${p.path} at its final path, from the page inputs below and nothing else.`
    : `Redraft ${p.path} once, on the combined findings below. Fix every blocking finding; take a non-blocking one when it is right. Do not widen the page.`;
  const inventory = (pageInputs.claimInventory || [])
    .map((c) => `- [${c.disposition}] ${c.claim}${c.factId ? ` (${c.factId})` : ""}${c.reason ? ` -- ${c.reason}` : ""}`)
    .join("\n");
  const guide = baseGuideFor(p.track);
  const drafting = ["brief", "names", "visuals", "track", "anatomies"]
    .map((k) => stripMarkers(sections[k]))
    .join("\n\n");
  return `${head}

Work only in the worktree ${wt}; run every command from there and never cd to another checkout.
This page follows the ${guide} style guide; the drafting brief below is its structural and voice
source. Use the sanctioned name for every part (the Names section is enforced by Vale).

Error-tier Vale rules, verbatim:

${ctx.valeRules || "(run `npm run check:vale` and fix every error-tier finding)"}

${drafting}

The page's job: ${pageInputs.job}
Page type: ${pageInputs.pageType}

Exemplars, each with the role that says what it may lend; never their content, terms, or product names:
${renderExemplars(pageInputs.exemplarExcerpts, ctx.exemplarSources) || "(none named; follow the page anatomies)"}

Fact ids to draw on: ${(pageInputs.factIds || []).join(", ") || "(none)"}
Claim inventory, one disposition per claim:
${inventory || "(the page is new; no prior claims to carry)"}
${p.pinned && p.pinned.length ? `Pinned heading slugs this page must keep, verbatim: ${p.pinned.join(", ")}` : ""}
${round > 1 ? `\nCombined findings from the reads:\n${findings}\n` : ""}
Write the page's sentence-to-fact brief alongside the page at ${ctx.briefPath}, citing only the
fact ids above or "no-claim". File no fact yourself, new or retagged. A claim the page needs whose
fact is not among the ids above is a couldNotDo naming the missing fact; do not draft that claim
and do not file its fact yourself, since the conductor re-runs page inputs for it. Something no
source can supply at all, a genuine design gap rather than a missing fact, goes to
docs/internal/docs-friction-log.md (name it in frictionFiled). Commit nothing; leave the tree with
your edits in place.

${ctx.gateText}

Return the structured report only.`;
}

/**
 * The register-editor prompt: the drafter's sections with markers kept, plus Provenance, the
 * tightening test, and both exceptions sections. Grading starts with guide conformance.
 * @param {{ wt: string, valeRules?: string, page: { path: string, track: string }, sections: Record<string, string> }} ctx
 * @returns {string}
 */
function renderEditorPrompt(ctx) {
  const { wt, page: p, sections } = ctx;
  const guide = baseGuideFor(p.track);
  const material = ["brief", "names", "visuals", "track", "anatomies", "provenance", "tightening", "exceptionsGoogle", "exceptionsMicrosoft"]
    .map((k) => sections[k])
    .join("\n\n");
  return `Adversarial edit of ${p.path} in ${wt}. Read the page, then grade in this order.

1. Guide conformance, first. The base guide for this page is ${guide}. The drafting brief below
quotes it, each quotation carrying a q:<id> marker whose source text and URL sit in the Provenance
section. A page that departs from a quoted rule is a structural defect. Return it as a finding
with source "guide" and rule set to that q:<id>. A departure that a Recorded exceptions row governs
(the row names the x:<id> marker in the brief) is recorded, not a defect. A brief passage or page
rule that loosens the guide with no exception row is itself a finding with source "guide".
2. The register after that: the brief's Voice and Tells, the Names section, the track section, the
page anatomies, logic, and facts-adjacent phrasing, each as a finding with source "register". The
tightening test decides whether a register rule is a tightening or an override.

Every finding carries source ("guide" or "register") and, for a guide finding, rule. Return ranked
findings with a proposed rewrite each, and a verdict: "fix" if any finding is blocking. Every
guide finding that cites a q:<id> of the brief is blocking.

${material}`;
}
// === END REGISTER EXTRACTION AND COERCION ===

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
  const commands = sectionSpecsFor(p.track)
    .map((spec) => `- ${spec.key}:\n  ${sectionCommand(spec, WT, REGISTER)}`)
    .join("\n");
  return `Page inputs for ${p.path} in ${WT}. You are step 1 of the docs page chain: you do not
draft the page and you do not run a gate.

Work only in the worktree ${WT}; run every command from there and never cd to another checkout.

This page's job, from the stage outline, verbatim:

${p.job}
${p.pageType ? `The outline's page type: ${p.pageType}` : ""}

Register sections. Run each command below exactly as written, once, and return its stdout
verbatim as that section's "output" in registerSections, with the key. Do not edit, trim, summarize,
or re-run with changes; a section whose command prints no heading fails the page.
${commands}

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
imitate. An "anatomy" source lends section order and step form only; a "voice" source lends its
sentence rhythm and voice as well. Never take an exemplar's own content, terms, or product names.
In each excerpt's "note", name every way the excerpt departs from the ${baseGuideFor(p.track)} style guide or from the cairn
docs voice, so the drafter does not copy it; leave the note empty when there is none.
${(p.exemplarSources || []).map((e) => `- ${e.path} (role: ${e.role})`).join("\n")}

Inputs to trace claims against (read each in full; the only source of a command, a transcript, a
JSON example, or a fact; never open the old page's own prose for a claim's wording):
${(p.inputs || []).map((i) => `- ${i}`).join("\n")}

Return the structured report only: the page's job (one paragraph), its page type, the two trimmed
exemplar excerpts (with their source path and note), the register sections, every fact id the
drafter should draw on, the full claim inventory, and any new fact id you filed.`;
}

function draftPrompt(p, pageInputs, sections, round, findings) {
  return renderDrafterPrompt({
    wt: WT, valeRules: a.valeErrorRules, page: p, pageInputs, sections,
    exemplarSources: p.exemplarSources, briefPath: briefPathFor(p),
    round, findings, gateText: gateLine(p)
  });
}

function editorPrompt(p, sections) {
  return renderEditorPrompt({ wt: WT, valeRules: a.valeErrorRules, page: p, sections });
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
      r.findings.map((f) => `- [${f.blocking ? "BLOCKING" : "advisory"}${f.source ? `, ${f.source}${f.rule ? ` ${f.rule}` : ""}` : ""}] ${f.location}: ${f.finding}${f.rewrite ? `\n  rewrite: ${f.rewrite}` : ""}`).join("\n"))
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

async function runReads(p, pageInputs, sections, round, names) {
  const tasks = [];
  if (names.includes("register editor")) {
    tasks.push(["register editor", () => agent(editorPrompt(p, sections), { label: `editor:${p.id}:r${round}`, phase: "Read", schema: READ_SCHEMA, model: REVIEWER, agentType: "cairn-register-editor" })]);
  }
  if (names.includes("fact read")) {
    tasks.push(["fact read", () => agent(factPrompt(p, pageInputs), { label: `facts:${p.id}:r${round}`, phase: "Read", schema: READ_SCHEMA, model: REVIEWER, agentType: "general-purpose" })]);
  }
  if (names.includes("figure verifier")) {
    tasks.push(["figure verifier", () => agent(figurePrompt(p), { label: `figure:${p.id}:r${round}`, phase: "Read", schema: READ_SCHEMA, model: REVIEWER, agentType: "figure-verifier" })]);
  }
  const results = await parallel(tasks.map(([, fn]) => fn));
  // The register editor's findings are coerced here, so a forced-blocking guide finding sets the
  // verdict to "fix" before anyFix is computed.
  const list = tasks.map(([name], i) => [name, name === "register editor" && results[i] ? coerceRead(results[i], sections.brief) : results[i]]).filter(([, r]) => r);
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
  const collected = collectSections(p.track, pageInputs.registerSections);
  if (!collected.ok) return { ...record, status: "escalate", reason: `register extraction rejected: ${collected.reason}` };
  const sections = collected.sections;
  const { registerSections: _sections, ...pageInputsRecord } = pageInputs;
  record.pageInputs = pageInputsRecord;

  const allNames = reviewNames(p);
  const d1 = await agent(draftPrompt(p, pageInputs, sections, 1), { label: `draft:${p.id}`, phase: "Draft", schema: DRAFT_SCHEMA, model: DRAFTER, agentType: DRAFTER_TYPE });
  if (!d1) return { ...record, status: "escalate", reason: "drafter returned nothing" };
  record.rounds.push({ round: 1, draft: d1 });
  const r1 = await runReads(p, pageInputs, sections, 1, allNames);
  record.rounds[0].reads = r1.list.map(([n, r]) => ({ read: n, verdict: r.verdict, summary: r.summary, blocking: r.findings.filter((f) => f.blocking).length }));
  // A read that returned nothing is never a silent accept.
  if (r1.missing) return { ...record, status: "escalate", reason: `${r1.missing} read(s) returned nothing in round 1` };
  if (!r1.anyFix && d1.gate === "pass") return { ...record, status: "accepted" };

  const rereviewNames = BOTH_REVIEWERS ? allNames : r1.list.filter(([, r]) => r.verdict === "fix").map(([n]) => n);
  const findings = combined(r1.list) + (d1.gate !== "pass" ? `\n\n## gate: ${d1.gate}\n${d1.gateTail || ""}` : "");
  const d2 = await agent(draftPrompt(p, pageInputs, sections, 2, findings), { label: `redraft:${p.id}`, phase: "Redraft", schema: DRAFT_SCHEMA, model: DRAFTER, agentType: DRAFTER_TYPE });
  if (!d2) return { ...record, status: "escalate", reason: "redrafter returned nothing" };
  record.rounds.push({ round: 2, draft: d2 });
  const r2 = await runReads(p, pageInputs, sections, 2, rereviewNames);
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

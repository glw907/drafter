// Runs the draft-docs page chain from `docs/superpowers/specs/2026-09-21-draft-docs-design.md`
// ("How a page gets made", "The chain, per page") over a list of pages, a few pages in flight at
// once. Per page: a Sonnet drafter writes the page at its final path and runs the docs gate; the
// register editor, a profile grader, and a fact read run together, each on Opus with zero
// context; one redraft round on the combined findings, gated and re-read; a second `fix` from any
// read escalates to the conductor. The conductor reads only the per-page records returned.
//
// Invocation from the conductor session (copy this file to the session scratchpad first; the
// Workflow tool refuses a `~/.claude/workflows` scriptPath):
//
//   Workflow({
//     scriptPath: "<scratchpad>/docs-page-chain.js",
//     args: {
//       worktree: "/home/glw907/Projects/cairn-cms/.claude/worktrees/<name>",
//       gate: "npm run check:docs && npm run check:vale && ...",   // the docs gate string
//       gateLane: "light",                 // optional; "light" prefixes CAIRN_GATE_LANE=light
//       inFlight: 3,                       // optional; pages drafted at once, default 3
//       toolGate: "make -C <worktree>/tool check",   // optional; appended for a page with `pinned`
//       drafterType: "cairn-implementer",  // optional; the drafter's agent type
//       profile: "<the track profile, printed verbatim>",
//       registerPaths: ["docs/internal/docs-register.md#..."],   // what every agent reads
//       valeErrorRules: "<the error-tier rule list, verbatim>",
//       drafterModel: "claude-opus-5-5",   // optional; published pages draft on Opus 5.5 (2026-09-22)
//       reviewModel: "claude-opus-5-5",      // optional
//       pages: [
//         {
//           id: "is-it-working",
//           path: "docs/admin/is-it-working.md",
//           brief: "<the brief from the plan, verbatim>",
//           inputs: ["docs/superpowers/plans/<plan>.mining.md#is-it-working", "..."],
//           exemplar: "docs/admin/<page>.md",
//           pinned: ["#slug-one", "#slug-two"],   // optional; slugs the page must keep
//           extraChecks: ["<a sentence the fact read or grader must also verify>"]   // optional
//         }
//       ]
//     }
//   })
//
// Every agent starts with zero context. The runner renders each stage's prompt from args and
// the page record; nothing load-bearing may live only in the conductor's conversation.

export const meta = {
  name: "docs-page-chain",
  description: "Drafts docs pages through drafter, gate, register editor, profile grader, fact read, and one redraft.",
  whenToUse: "A draft-docs pass plan names this workflow for its page tasks.",
  phases: [
    { title: "Draft", detail: "one Sonnet drafter per page, docs gate inside" },
    { title: "Read", detail: "register editor, profile grader, fact read, each on Opus" },
    { title: "Redraft", detail: "one round on the combined findings, then the reads again" },
    { title: "Report", detail: "per-page records for the conductor" }
  ]
};

const DRAFT_SCHEMA = {
  type: "object",
  properties: {
    path: { type: "string" },
    gate: { type: "string", enum: ["pass", "fail", "not run"] },
    gateCommand: { type: "string" },
    gateTail: { type: "string" },
    bulletsFiled: { type: "array", items: { type: "string" } },
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

const GRADER_SCHEMA = {
  type: "object",
  properties: {
    verdict: { type: "string", enum: ["accept", "fix"] },
    elements: {
      type: "array",
      items: {
        type: "object",
        properties: {
          element: { type: "string" },
          verdict: { type: "string", enum: ["pass", "fail"] },
          evidence: { type: "string" }
        },
        required: ["element", "verdict", "evidence"]
      }
    },
    findings: { type: "array", items: FINDING },
    summary: { type: "string" }
  },
  required: ["verdict", "elements", "findings", "summary"]
};

const a = args || {};
const WT = a.worktree;
const GATE = a.gate;
const LANE = a.gateLane === "light" ? "CAIRN_GATE_LANE=light " : "";
const IN_FLIGHT = Number.isInteger(a.inFlight) && a.inFlight > 0 ? a.inFlight : 3;
const DRAFTER = a.drafterModel || "claude-opus-5-5";
const DRAFTER_TYPE = a.drafterType || "cairn-implementer";
const TOOL_GATE = a.toolGate || null;   // appended for a page carrying `pinned`, e.g. "make -C <wt>/tool check"
const REVIEWER = a.reviewModel || "claude-opus-5-5";
const PAGES = a.pages || [];

if (!WT || !GATE || !a.profile || !PAGES.length) {
  throw new Error("docs-page-chain needs args.worktree, args.gate, args.profile, and args.pages");
}

const common = `Work only in the worktree ${WT}; run every command from there and never cd to another checkout.
The register is ${(a.registerPaths || ["docs/internal/docs-register.md"]).join(", ")}: read the universal
contract and the track section before anything else. The Names convention in that file's "Names"
section is enforced by Vale (Cairn.Names, Cairn.NamesRetired); use the sanctioned name for every
part. The track profile, verbatim:

${a.profile}

Error-tier Vale rules, verbatim:

${a.valeErrorRules || "(run `npm run check:vale` and fix every error-tier finding)"}
`;

const gateFor = (p) => (p.pinned && p.pinned.length && TOOL_GATE) ? `${GATE} && ${TOOL_GATE}` : GATE;
const gateLine = (p) => `Run the docs gate through the gate runner, exactly:
  ${LANE}cairn-run-gate '${gateFor(p)}'
On exit 75, re-issue the same command until it prints \`gate exit:\`; never poll a log. Report
the exact command and the last twenty lines. A gate that is red on a rule the plan says a later
task closes (for example check:arm-indexes before the index task) is reported as "fail" with the
failing rule named, and is not a reason to edit an index page this task does not own.`;

function draftPrompt(p, round, findings) {
  const head = round === 1
    ? `Draft the page ${p.path} at its final path, from the brief and the inputs below and nothing else.`
    : `Redraft ${p.path} once, on the combined findings below. Fix every blocking finding; take a non-blocking one when it is right. Do not widen the page.`;
  return `${head}

${common}
The brief, verbatim:

${p.brief}

Inputs (read each in full; the manifests and bullets are the only source of a command, a
transcript, a JSON example, or a fact; never compose one and never open the old page or a
tool/docs/ original):
${(p.inputs || []).map((i) => `- ${i}`).join("\n")}

Exemplar to imitate for anatomy and register: ${p.exemplar || "(none named; follow the register's anatomy)"}
${p.pinned && p.pinned.length ? `Pinned heading slugs this page must keep, verbatim: ${p.pinned.join(", ")}` : ""}
${round > 1 ? `\nCombined findings from the reads:\n${findings}\n` : ""}
A fact the page needs that no input records is filed as a [candidate] bullet against code on
main in the facts container (name it in bulletsFiled); something no source can supply goes to
docs/internal/docs-friction-log.md (name it in frictionFiled). Commit nothing; leave the tree
with your edits in place.

${gateLine(p)}

Return the structured report only.`;
}

function editorPrompt(p) {
  return `Adversarial register edit of ${p.path} in ${WT}. Read the page, then the register's
universal contract and its track section, then grade. Your report MUST carry a section titled
"Profile" grading the page against this profile, one line per element:

${a.profile}

Also apply the tell catalogue, the Names section, logic, and facts-adjacent phrasing. Return
ranked findings with a proposed rewrite each, and a verdict: "fix" if any finding is blocking.`;
}

function graderPrompt(p) {
  return `You are a fresh-context profile grader for ${p.path} in ${WT}. Read only that page, the
register file(s) ${(a.registerPaths || ["docs/internal/docs-register.md"]).join(", ")}, and any
page it links. Grade it against this profile, verbatim, one verdict per element with the evidence
(a quoted line or its absence):

${a.profile}

${p.extraChecks && p.extraChecks.length ? `Also verify each of these, as further elements:\n${p.extraChecks.map((c) => `- ${c}`).join("\n")}\n` : ""}
A failed element is a blocking finding with the location and what would pass. Verdict "fix" if
any element fails.`;
}

function factPrompt(p) {
  return `Fact read of ${p.path} in ${WT}. Trace every claim on the page (each step, command,
transcript, figure, warning, success signal, and prose assertion) to one of these sources:
${(p.inputs || []).map((i) => `- ${i}`).join("\n")}
A claim with no bullet or ratified disposition behind it is a blocking finding (location, claim,
what the source lacks). A bullet or skeleton step in the sources that the page dropped is a
blocking finding naming the bullet. A command or example that differs from its manifest entry by
one character is blocking. Verdict "fix" if any blocking finding exists; otherwise "accept" with
the count of claims traced.`;
}

function combined(readList) {
  return readList
    .map(([name, r]) => `## ${name}: ${r.verdict}\n${r.summary}\n` +
      r.findings.map((f) => `- [${f.blocking ? "BLOCKING" : "advisory"}] ${f.location}: ${f.finding}${f.rewrite ? `\n  rewrite: ${f.rewrite}` : ""}`).join("\n"))
    .join("\n\n");
}

async function reads(p, round) {
  const ph = `Read`;
  const [editor, grader, fact] = await parallel([
    () => agent(editorPrompt(p), { label: `editor:${p.id}:r${round}`, phase: ph, schema: READ_SCHEMA, model: REVIEWER, agentType: "cairn-register-editor" }),
    () => agent(graderPrompt(p), { label: `grader:${p.id}:r${round}`, phase: ph, schema: GRADER_SCHEMA, model: REVIEWER, agentType: "general-purpose" }),
    () => agent(factPrompt(p), { label: `facts:${p.id}:r${round}`, phase: ph, schema: READ_SCHEMA, model: REVIEWER, agentType: "general-purpose" })
  ]);
  const list = [["register editor", editor], ["profile grader", grader], ["fact read", fact]]
    .filter(([, r]) => r);
  const missing = 3 - list.length;
  const anyFix = list.some(([, r]) => r.verdict === "fix");
  return { list, missing, anyFix };
}

async function chain(p) {
  const record = { id: p.id, path: p.path, rounds: [] };
  const d1 = await agent(draftPrompt(p, 1), { label: `draft:${p.id}`, phase: "Draft", schema: DRAFT_SCHEMA, model: DRAFTER, agentType: DRAFTER_TYPE });
  if (!d1) return { ...record, status: "escalate", reason: "drafter returned nothing" };
  record.rounds.push({ round: 1, draft: d1 });
  const r1 = await reads(p, 1);
  record.rounds[0].reads = r1.list.map(([n, r]) => ({ read: n, verdict: r.verdict, summary: r.summary, blocking: r.findings.filter((f) => f.blocking).length }));
  // A read that returned nothing is never a silent accept.
  if (r1.missing) return { ...record, status: "escalate", reason: `${r1.missing} read(s) returned nothing in round 1` };
  if (!r1.anyFix && d1.gate === "pass") return { ...record, status: "accepted" };

  const findings = combined(r1.list) + (d1.gate !== "pass" ? `\n\n## gate: ${d1.gate}\n${d1.gateTail || ""}` : "");
  const d2 = await agent(draftPrompt(p, 2, findings), { label: `redraft:${p.id}`, phase: "Redraft", schema: DRAFT_SCHEMA, model: DRAFTER, agentType: DRAFTER_TYPE });
  if (!d2) return { ...record, status: "escalate", reason: "redrafter returned nothing" };
  record.rounds.push({ round: 2, draft: d2 });
  const r2 = await reads(p, 2);
  record.rounds[1].reads = r2.list.map(([n, r]) => ({ read: n, verdict: r.verdict, summary: r.summary, blocking: r.findings.filter((f) => f.blocking).length }));
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
phase("Draft");
await parallel(Array.from({ length: Math.min(IN_FLIGHT, PAGES.length) }, (_, i) => () => worker(i + 1)));

phase("Report");
const accepted = results.filter((r) => r.status === "accepted").length;
log(`${accepted}/${PAGES.length} pages accepted; ${PAGES.length - accepted} escalated`);
return { pages: results, accepted, escalated: results.filter((r) => r.status !== "accepted").map((r) => r.id) };

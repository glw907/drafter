// Tests docs-page-chain.js's drafter hooks: the default-behavior snapshot (with `backHalf` unset the
// chain sends what it sent before the hooks existed), the fidelity of `DRAFTER_SEATS` against the
// prompts the chain itself sends, the `// === HANDOFF ===` block, and `backHalf: "drafter"`.
// Run with: node tests/docs-page-chain-handoff.test.mjs
// UPDATE_SNAPSHOTS=1 rewrites the snapshots from the current chain, on purpose.
// Exits 0 with "ALL PASS" on success; prints failures and exits 1 otherwise.
import { readFileSync, writeFileSync, mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { execSync } from "node:child_process";
import assert from "node:assert/strict";

const HERE = dirname(fileURLToPath(import.meta.url));
const RUNNER_PATH = join(HERE, "..", "claude", ".claude", "workflows", "docs-page-chain.js");
const HELPER_PATH = join(HERE, "..", "bin", ".local", "bin", "cairn-docs-outline");
const RUNNER_SRC = readFileSync(RUNNER_PATH, "utf8");
const FIXTURES = join(HERE, "fixtures", "drafter");
const SNAPSHOTS = join(HERE, "fixtures", "docs-page-chain");

const failures = [];
const pending = [];
function check(name, fn) {
  pending.push({ name, fn });
}

const scratch = [];
function tempDir(prefix) {
  const d = mkdtempSync(join(tmpdir(), prefix));
  scratch.push(d);
  return d;
}

function write(root, rel, text) {
  mkdirSync(dirname(join(root, rel)), { recursive: true });
  writeFileSync(join(root, rel), text);
}

// The runner is a workflow script with top-level await and a top-level return, so it compiles as
// an async function body with the runtime's ambient globals as parameters.
const AsyncFunction = (async () => {}).constructor;
const runnerBody = RUNNER_SRC.replace(/^export const meta/m, "const meta");
const runRunner = new AsyncFunction("args", "agent", "parallel", "log", "phase", "budget", "DRAFTER_HOST", runnerBody);

const parallel = (fns) => Promise.all(fns.map((f) => f()));
const noop = () => {};

// -------------------------------------------------------------------------------------------
// Fixtures: an outline of three synthetic pages, one per page type, and a temp worktree for it.
// -------------------------------------------------------------------------------------------

const OUTLINE_PATH = "docs/internal/outlines/guide.json";
const OUTLINE = {
  index: "docs/guide/README.md",
  groups: [{ id: "start", title: "Start", pages: ["install-widget", "how-widgets-work", "widget-options"] }],
  pages: [
    {
      slug: "install-widget", title: "Install the widget", path: "docs/guide/install-widget.md", group: "start", order: 1,
      job: "Install the widget CLI and run it once.", pageType: "task guide",
      exemplars: [{ source: "tool/widget-quickstart/", take: "Take the step cadence." }], figure: false,
      factIds: ["f:aaa111"], covers: ["The install command."], outOfScope: ["Plugin authoring (plugins)."], pinned: []
    },
    {
      slug: "how-widgets-work", title: "How widgets work", path: "docs/guide/how-widgets-work.md", group: "start", order: 2,
      job: "Learn the widget's model.", pageType: "concept",
      exemplars: [{ source: "tool/widget-model/", take: "Take the diagram's order." }], figure: false,
      factIds: ["f:bbb222"], covers: ["The three parts."], outOfScope: [], pinned: []
    },
    {
      slug: "widget-options", title: "Widget options", path: "docs/guide/widget-options.md", group: "start", order: 3,
      job: "Look up a widget option.", pageType: "reference", exemplars: [], figure: false,
      factIds: ["f:ccc333"], covers: ["Every option."], outOfScope: [], pinned: []
    }
  ]
};
const INDEX = "# Guide\n\n## Start\n";
const PAGE_TYPES = { "install-widget": "task guide", "how-widgets-work": "concept", "widget-options": "reference" };

function makeHome() {
  const home = tempDir("dph-home-");
  const root = join(home, ".local", "share", "cairn", "exemplars");
  write(root, "tool/widget-quickstart/page.md", "# Quickstart\n");
  write(root, "tool/widget-model/page.md", "# Model\n");
  return home;
}

function makeWorktree() {
  const wt = tempDir("dph-wt-");
  write(wt, OUTLINE_PATH, JSON.stringify(OUTLINE, null, 1));
  write(wt, "docs/guide/README.md", INDEX);
  return wt;
}

/** Runs one helper command line the runner rendered, with the helper file standing in for PATH. */
function runHelperLine(line, home) {
  const cmd = line.trim().replace(/^cairn-docs-outline\b/, `node '${HELPER_PATH}'`);
  try {
    return execSync(cmd, { shell: "/bin/bash", env: { ...process.env, HOME: home }, encoding: "utf8" });
  } catch (e) {
    return e.stdout || "";
  }
}

function helperLine(prompt, verb) {
  return prompt.split("\n").find((l) => l.trim().startsWith(`cairn-docs-outline ${verb} `));
}

const ACCEPT = { verdict: "accept", findings: [], summary: "ok" };
const FIX = { verdict: "fix", findings: [{ location: "l1", finding: "f1", blocking: true }], summary: "fix" };

/** The fact ids a plan prompt asks the plan to dispose, read from its one list line. */
function planIds(prompt) {
  const m = prompt.match(/Fact ids the plan disposes, every one: ([^\n]*)/);
  return m ? m[1].split(", ").filter((x) => x.startsWith("f:")) : [];
}

/**
 * A stubbed agent for one run: the outline probe runs its rendered helper command; page inputs
 * return the page's type from `PAGE_TYPES`; the plan and framing steps write their files; every
 * read accepts unless `overrides` says otherwise. The drafter writes the page.
 */
function makeAgent(wt, home, calls, overrides = {}) {
  return async (prompt, opts) => {
    calls.push({ label: opts.label, prompt, opts });
    const kind = opts.label.split(":")[0];
    if (overrides[kind]) {
      const v = await overrides[kind](prompt, opts);
      if (v !== undefined) return v;
    }
    const slug = opts.label.split(":")[1];
    if (kind === "outline") return JSON.parse(runHelperLine(helperLine(prompt, "resolve"), home));
    if (kind === "inputs") {
      return {
        job: `the job of ${slug}`, pageType: PAGE_TYPES[slug] || "concept", factIds: [`f:${slug.slice(0, 3)}000`],
        claimInventory: [{ claim: `a claim of ${slug}`, disposition: "carried", factId: `f:${slug.slice(0, 3)}000` }],
        rowsReceived: [], rowsDisposed: []
      };
    }
    if (kind === "plan" || kind === "replan") {
      const plan = prompt.match(/docs\/internal\/briefs\/\S+?\.plan\.(?:md|yaml)/)[0];
      write(wt, plan, "# Plan\n");
      return {
        plan, claimInventory: planIds(prompt).map((factId) => ({ claim: "a planned fact", disposition: "carried", factId, section: "Section A" })),
        checkPlan: { exit: 0, output: "" }
      };
    }
    if (kind === "probe") return { exists: true };
    if (kind === "framing") return { framing: prompt.match(/docs\/internal\/briefs\/\S+?\.framing\.md/)[0], readers: ["a reader"], factIds: [] };
    if (kind === "draft" || kind === "redraft" || kind === "reader-redraft") {
      const path = prompt.match(/(?:Draft the page|Redraft) (\S+)/)[1];
      write(wt, path, "# Page\n\nBody.\n");
      return { path, gate: "pass", gateCommand: "stub", baseline: "0123456789abcdef0123456789abcdef01234567" };
    }
    return ACCEPT;
  };
}

function pagesFor(withOutline) {
  return OUTLINE.pages.map((e) => (withOutline
    ? { id: e.slug, path: e.path, track: "extend" }
    : { id: e.slug, path: e.path, track: "extend", job: e.job, pageType: e.pageType, title: e.title, factIds: e.factIds, covers: e.covers, outOfScope: e.outOfScope }));
}

function runArgs(wt, withOutline, extra = {}) {
  return {
    worktree: wt, gate: "npm run check:docs-gate -- --page {page} --brief {brief}", pages: pagesFor(withOutline),
    inFlight: 1, ...(withOutline ? { outline: OUTLINE_PATH } : {}), ...extra
  };
}

/** One run of the chain with stubbed agents; the calls and the return, with temp paths replaced. */
async function run(withOutline, { overrides = {}, extra = {}, host, home = makeHome(), wt = makeWorktree() } = {}) {
  const calls = [];
  const out = await runRunner(runArgs(wt, withOutline, extra), makeAgent(wt, home, calls, overrides), parallel, noop, noop, undefined, host);
  const scrub = (s) => s.split(wt).join("<WORKTREE>").split(home).join("<HOME>");
  return { out, calls, wt, home, scrub };
}

/** The snapshot of a run: every agent call (label, prompt, options) and the return object. */
function snapshotOf(r) {
  return JSON.parse(r.scrub(JSON.stringify({
    calls: r.calls.map((c) => ({ label: c.label, prompt: c.prompt, opts: c.opts })),
    returned: r.out
  })));
}

// -------------------------------------------------------------------------------------------
// With `backHalf` unset the chain sends exactly what it sent before the drafter hooks existed.
// -------------------------------------------------------------------------------------------

for (const [name, withOutline, overrides] of [
  ["with-outline", true, {}],
  ["no-outline", false, {}],
  ["with-outline-round2", true, { structure: async (p, o) => (o.label.endsWith(":r1") ? FIX : ACCEPT) }]
]) {
  check(`snapshot: the ${name} run matches its recorded agent calls and return byte for byte`, async () => {
    const snap = snapshotOf(await run(withOutline, { overrides }));
    const path = join(SNAPSHOTS, `default-run.${name}.json`);
    if (process.env.UPDATE_SNAPSHOTS) {
      mkdirSync(SNAPSHOTS, { recursive: true });
      writeFileSync(path, `${JSON.stringify(snap, null, 1)}\n`);
    }
    assert.deepEqual(snap, JSON.parse(readFileSync(path, "utf8")));
  });
}

// -------------------------------------------------------------------------------------------
// The handoff block, extracted by its markers and run alone.
// -------------------------------------------------------------------------------------------

const HANDOFF_START = "// === HANDOFF (docs-page-chain-handoff.test.mjs extracts this block) ===";
const HANDOFF_END = "// === END HANDOFF ===";

/** The marked block's source; a missing or reordered marker is an error. */
function extractBlock(src) {
  const start = src.indexOf(HANDOFF_START);
  const end = src.indexOf(HANDOFF_END);
  if (start === -1) throw new Error("the HANDOFF start marker is missing");
  if (end === -1) throw new Error("the HANDOFF end marker is missing");
  if (end < start) throw new Error("the HANDOFF markers are reordered");
  return src.slice(start + HANDOFF_START.length, end);
}

const handoffFor = new Function(`${extractBlock(RUNNER_SRC)}\nreturn handoffFor;`)();
const CHAIN_HANDOFF = JSON.parse(readFileSync(join(FIXTURES, "chain-handoff.json"), "utf8"));

check("handoff block: a missing or reordered marker fails the extraction", () => {
  assert.throws(() => extractBlock(RUNNER_SRC.replace(HANDOFF_START, "")), /start marker is missing/);
  assert.throws(() => extractBlock(RUNNER_SRC.replace(HANDOFF_END, "")), /end marker is missing/);
  const swapped = RUNNER_SRC.replace(HANDOFF_START, "@@S").replace(HANDOFF_END, HANDOFF_START).replace("@@S", HANDOFF_END);
  assert.throws(() => extractBlock(swapped), /reordered/);
});

check("handoff block: handoffFor on a recorded synthetic page deep-equals chain-handoff.json", () => {
  const [page] = CHAIN_HANDOFF.resolved.pages;
  const [entry] = CHAIN_HANDOFF.pages;
  const got = handoffFor(
    CHAIN_HANDOFF.args,
    CHAIN_HANDOFF.resolved,
    CHAIN_HANDOFF.worktree,
    [{
      page: { ...page, exemplarSources: entry.inputs.exemplarSources },
      pageInputs: { job: entry.inputs.job, pageType: entry.inputs.pageType, factIds: entry.inputs.factIds, claimInventory: entry.inputs.claimInventory },
      brief: entry.page.brief,
      plan: entry.plan,
      framing: entry.framing,
      registerSections: entry.inputs.registerSections
    }]
  );
  assert.deepEqual(got, CHAIN_HANDOFF);
});

check("handoff block: page.type maps the chain's page type in the order the reader prompt branches", () => {
  const typeFor = (pageType, fallback) => handoffFor({}, { pages: [], index: "" }, "/w", [{
    page: { path: "p.md", pageType: fallback }, pageInputs: { pageType }, brief: "b", plan: "p", framing: "f", registerSections: []
  }]).pages[0].page.type;
  assert.equal(typeFor("concept"), "concept");
  assert.equal(typeFor("task guide"), "task-guide");
  assert.equal(typeFor("Tutorial"), "task-guide");
  assert.equal(typeFor("how-to"), "task-guide");
  assert.equal(typeFor("reference"), "reference");
  assert.equal(typeFor("anything else"), "reference");
  assert.equal(typeFor("", "concept"), "concept", "the outline's type backs a page-inputs record with none");
});

// -------------------------------------------------------------------------------------------
// backHalf.
// -------------------------------------------------------------------------------------------

const HALF_FORBIDDEN = /^(draft|redraft|reader|reader-redraft|rework|structure:[^:]+:r\d|editor|facts|figure)\b/;

for (const withOutline of [true, false]) {
  check(`backHalf drafter (${withOutline ? "with" : "no"} outline): each page stops after framing and the return holds the handoff`, async () => {
    const r = await run(withOutline, { extra: { backHalf: "drafter" } });
    const labels = r.calls.map((c) => c.label);
    assert.ok(labels.includes("probe:install-widget:framing"), "the framing step ran");
    assert.deepEqual(labels.filter((l) => HALF_FORBIDDEN.test(l)), [], "no draft, read, or reader agent ran");
    assert.deepEqual(Object.keys(r.out).sort(), ["escalated", "handoff"]);
    assert.deepEqual(r.out.escalated, []);
    const h = r.out.handoff;
    assert.equal(h.version, 1);
    assert.equal(h.worktree, r.wt);
    assert.deepEqual(h.args, runArgs(r.wt, withOutline, { backHalf: "drafter" }));
    assert.deepEqual(h.pages.map((p) => p.page.path), OUTLINE.pages.map((e) => e.path));
    assert.deepEqual(h.pages.map((p) => p.page.type), ["task-guide", "concept", "reference"]);
    assert.deepEqual(h.pages.map((p) => p.inputs.pageType), ["task guide", "concept", "reference"]);
    assert.equal(h.pages[0].plan, "docs/internal/briefs/extend/install-widget.plan.yaml");
    assert.equal(h.pages[0].framing, "docs/internal/briefs/extend/install-widget.framing.md");
    assert.equal(h.pages[0].page.brief, "docs/internal/briefs/extend/install-widget.json");
    assert.ok(h.pages[0].inputs.registerSections.includes("## The page anatomies"));
    assert.equal(h.resolved.index, withOutline ? "docs/guide/README.md" : "");
    assert.deepEqual(h.resolved.pages.map((p) => p.path), OUTLINE.pages.map((e) => e.path));
  });
}

check("backHalf drafter: a page that escalates in the plan step lands in escalated, not in the handoff", async () => {
  const r = await run(true, { extra: { backHalf: "drafter" }, overrides: { plan: async (prompt, opts) => (opts.label === "plan:how-widgets-work" ? null : undefined) } });
  assert.deepEqual(r.out.escalated.map((x) => x.id), ["how-widgets-work"]);
  assert.equal(r.out.escalated[0].status, "escalate");
  assert.deepEqual(r.out.handoff.pages.map((p) => p.page.path), ["docs/guide/install-widget.md", "docs/guide/widget-options.md"]);
});

check("backHalf drafter: a figure page and a rework page are refused by name, before any page agent", async () => {
  for (const withOutline of [true, false]) {
    const pages = pagesFor(withOutline);
    pages[0] = { ...pages[0], figure: true };
    pages[2] = { ...pages[2], rework: "REWORK-TEXT" };
    const calls = [];
    const wt = makeWorktree();
    await assert.rejects(
      runRunner(runArgs(wt, withOutline, { backHalf: "drafter", pages }), makeAgent(wt, makeHome(), calls), parallel, noop, noop, undefined, undefined),
      (e) => /docs\/guide\/install-widget\.md \(figure: true\)/.test(e.message) && /docs\/guide\/widget-options\.md \(rework\)/.test(e.message)
    );
    assert.deepEqual(calls.map((c) => c.label), withOutline ? ["outline"] : []);
  }
});

check("backHalf: any value but \"drafter\" throws", async () => {
  for (const bad of ["other", "", true]) {
    const wt = makeWorktree();
    await assert.rejects(runRunner(runArgs(wt, true, { backHalf: bad }), makeAgent(wt, makeHome(), []), parallel, noop, noop, undefined, undefined), /backHalf/);
  }
});

// -------------------------------------------------------------------------------------------
// DRAFTER_HOST: the seats.
// -------------------------------------------------------------------------------------------

/** The resolved pages the chain itself used, from a backHalf run, and the seats built over them. */
async function seatsFor(withOutline) {
  const half = await run(withOutline, { extra: { backHalf: "drafter" } });
  const host = { resolved: half.out.handoff.resolved };
  const r = await run(withOutline, { host, wt: half.wt, home: half.home });
  return { half, host, r, seats: r.out, wt: r.wt, home: r.home };
}

check("DRAFTER_HOST: the script returns the five seats and calls no agent", async () => {
  const { r, seats } = await seatsFor(true);
  assert.deepEqual(Object.keys(seats).sort(), ["draft", "editor", "fact", "reader", "structure"]);
  assert.deepEqual(r.calls, []);
  for (const f of Object.values(seats)) assert.equal(typeof f, "function");
});

const BASE = "0123456789abcdef0123456789abcdef01234567";
const FIX_STRUCT = { verdict: "fix", findings: [{ location: "s:1", finding: "STRUCT-FINDING", blocking: true, rewrite: "STRUCT-REWRITE" }], summary: "structure fix" };
const FIX_EDITOR = { verdict: "fix", findings: [{ location: "e:2", finding: "EDITOR-FINDING", blocking: false }], summary: "editor fix" };

/** The chain's own combined-findings text for named reads, written out for the test. */
function combinedOf(list) {
  return list
    .map(([name, r]) => `## ${name}: ${r.verdict}\n${r.summary}\n` +
      r.findings.map((f) => `- [${f.blocking ? "BLOCKING" : "advisory"}] ${f.location}: ${f.finding}${f.rewrite ? `\n  rewrite: ${f.rewrite}` : ""}`).join("\n"))
    .join("\n\n");
}

const SNAPSHOT_TEXT = `
Before you edit anything, run exactly this from the worktree:
  git hash-object -w -- PATH
and report the hex object name it prints, verbatim, as baseline: the page as the round-1 reads saw
it, which the round-2 reads diff against. Run it once, before your first edit.
`;

/** Every seat call of one chain run, against the context object the seat gets for it. */
function assertSeats(r, seats, hostPages, handoffPages, scenario) {
  for (const [i, hp] of hostPages.entries()) {
    const id = hp.path.replace(/^.*\/|\.md$/g, "");
    const page = hp;
    const inputs = handoffPages[i].inputs;
    const prompt = (label) => r.calls.find((c) => c.label === label).prompt;
    const ctx = (extra) => ({ page, inputs, round: 1, findings: "", afterFinalRead: false, scope: "", where: "", narrow: null, facts: true, ...extra });
    const same = (seat, label, c, hint) => assert.equal(seats[seat](c), prompt(label), `${hint || seat} at ${label}`);

    if (scenario === "narrowed") {
      const r1 = [["structural edit", FIX_STRUCT], ["register editor", FIX_EDITOR], ["fact read", ACCEPT]];
      same("draft", `draft:${id}`, ctx({}));
      same("structure", `structure:${id}:r1`, ctx({}));
      same("editor", `editor:${id}:r1`, ctx({}));
      same("fact", `facts:${id}:r1`, ctx({}));
      const findings = combinedOf(r1);
      const redraft = prompt(`redraft:${id}`).replace(SNAPSHOT_TEXT.replace("PATH", page.path), "");
      assert.notEqual(redraft, prompt(`redraft:${id}`), "the chain's redraft carries the snapshot instruction");
      assert.equal(seats.draft(ctx({ round: 2, findings })), redraft, `draft at round 2 (no snapshot)`);
      const narrow = (own) => ({ baseline: BASE, own });
      same("structure", `structure:${id}:r2`, ctx({ round: 2, narrow: narrow(FIX_STRUCT) }));
      same("editor", `editor:${id}:r2`, ctx({ round: 2, narrow: narrow(FIX_EDITOR) }));
      same("fact", `facts:${id}:r2`, ctx({ round: 2, narrow: narrow(ACCEPT) }));
    } else if (scenario === "whole") {
      same("structure", `structure:${id}:r2`, ctx({ round: 2 }));
      same("editor", `editor:${id}:r2`, ctx({ round: 2 }));
      same("fact", `facts:${id}:r2`, ctx({ round: 2 }));
    } else if (scenario === "final") {
      const cited = combinedOf([["final reader read", READER_FIX]]);
      same("draft", `reader-redraft:${id}`, ctx({ round: "final", findings: cited, afterFinalRead: true }));
      same("editor", `editor:${id}:final`, ctx({ round: "final", scope: cited }));
      same("fact", `facts:${id}:final`, ctx({ round: "final", scope: cited }));
    }
  }
}

const READER_FIX = { verdict: "fix", findings: [{ location: "p:3", finding: "READER-FINDING", blocking: true }], summary: "reader fix", paraphrase: "PARA" };

const SCENARIOS = {
  narrowed: { overrides: { structure: async (p, o) => (o.label.endsWith(":r1") ? FIX_STRUCT : ACCEPT), editor: async (p, o) => (o.label.endsWith(":r1") ? FIX_EDITOR : ACCEPT) }, extra: { bothReviewers: true } },
  whole: {
    overrides: {
      structure: async (p, o) => (o.label.endsWith(":r1") ? FIX_STRUCT : ACCEPT),
      redraft: async (p) => ({ path: p.match(/Redraft (\S+)/)[1], gate: "pass", gateCommand: "stub" })
    },
    extra: { bothReviewers: true }
  },
  final: { overrides: { reader: async (p, o) => (o.label === "reader:install-widget" || o.label === "reader:how-widgets-work" || o.label === "reader:widget-options" ? READER_FIX : { ...ACCEPT, paraphrase: "PARA" }) }, extra: {} }
};

for (const withOutline of [true, false]) {
  for (const [name, sc] of Object.entries(SCENARIOS)) {
    check(`fidelity (${withOutline ? "with" : "no"} outline, ${name}): DRAFTER_SEATS equals the prompt the chain sent at each point`, async () => {
      const { half, seats, wt, home } = await seatsFor(withOutline);
      const r = await run(withOutline, { overrides: sc.overrides, extra: sc.extra, wt, home });
      assertSeats(r, seats, half.out.handoff.resolved.pages, half.out.handoff.pages, name);
    });
  }

  check(`fidelity (${withOutline ? "with" : "no"} outline): the reader's two turns are the chain's prompt in two parts`, async () => {
    const { half, seats, wt, home } = await seatsFor(withOutline);
    const r = await run(withOutline, { wt, home });
    for (const [i, page] of half.out.handoff.resolved.pages.entries()) {
      const id = page.path.replace(/^.*\/|\.md$/g, "");
      const chainPrompt = r.calls.find((c) => c.label === `reader:${id}`).prompt;
      const inputs = half.out.handoff.pages[i].inputs;
      const ctx = { page, inputs, round: "final", findings: "", afterFinalRead: false, scope: "", where: "", narrow: null, facts: true };
      const turn1 = seats.reader({ ...ctx, turn: 1 });
      const turn2 = seats.reader({ ...ctx, turn: 2 });
      const cut = chainPrompt.indexOf("\n\nStep 3. Only now,");
      assert.ok(cut > 0);
      assert.equal(turn1, `${chainPrompt.slice(0, cut)}\n\nStep 3 follows in the next turn.`);
      assert.ok(!turn1.includes(page.job), "the first turn holds no job text");
      const step3 = chainPrompt.slice(cut + 2);
      const inlined = withOutline
        ? step3.replace(`open docs/internal/outlines/guide.json and read the "job" of the entry in "pages" whose "path" is "${page.path}"`, `read the page's job: ${page.job}`)
        : step3;
      assert.notEqual(inlined.includes("outlines/guide.json"), true, "the outline file is replaced by the job");
      assert.ok(turn2.endsWith(inlined), "turn 2 ends with the chain's step 3, the job inlined");
      assert.ok(turn2.includes(page.path) && turn2.includes(page.job));
      assert.throws(() => seats.reader({ ...ctx, turn: 3 }), /turn 1 or 2/);
    }
  });
}

for (const { name, fn } of pending) {
  try {
    await fn();
    console.log(`ok - ${name}`);
  } catch (e) {
    failures.push(name);
    console.log(`FAIL - ${name}`);
    console.log(`  ${(e && e.stack) || e}`);
  }
}
for (const d of scratch) rmSync(d, { recursive: true, force: true });

console.log("");
if (failures.length) {
  console.log(`${failures.length} FAILING: ${failures.join(", ")}`);
  process.exit(1);
}
console.log("ALL PASS");

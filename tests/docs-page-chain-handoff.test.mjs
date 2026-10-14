// Tests docs-page-chain.js's drafter hooks: the default-behavior snapshot (with `backHalf` unset the
// chain sends what it sent before the hooks existed), the fidelity of `DRAFTER_SEATS` against the
// prompts the chain itself sends, the `// === HANDOFF ===` block, and `backHalf: "drafter"`.
// Run with: node tests/docs-page-chain-handoff.test.mjs
// UPDATE_SNAPSHOTS=1 rewrites the snapshots from the current chain, on purpose.
// Exits 0 with "ALL PASS" on success; prints failures and exits 1 otherwise.
import { readFileSync, writeFileSync, mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { createRequire } from "node:module";
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

const helper = createRequire(import.meta.url)(HELPER_PATH);
void helper;

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
    if (overrides[kind]) return overrides[kind](prompt, opts);
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
async function run(withOutline, { overrides = {}, extra = {}, host } = {}) {
  const home = makeHome();
  const wt = makeWorktree();
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

// DRAFTER-HOOKS-TESTS

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
void FIXTURES;

console.log("");
if (failures.length) {
  console.log(`${failures.length} FAILING: ${failures.join(", ")}`);
  process.exit(1);
}
console.log("ALL PASS");

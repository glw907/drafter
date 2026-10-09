// Tests docs-page-chain.js's outline support and its cairn-docs-outline helper: the helper's pure
// functions (outline lookup, exemplar mapping, index link insertion), the runner's extracted
// merge block, and dry runs of the whole runner body with agent, parallel, log, phase, and budget stubbed.
// The stubbed probe and drafter run the exact helper commands the runner renders into their
// prompts, against a temp worktree, so a dry run proves the prompts carry a runnable command.
// Run with: node tests/docs-page-chain-outline.test.mjs
// Exits 0 with "ALL PASS" on success; prints failures and exits 1 otherwise.
import { readFileSync, writeFileSync, mkdirSync, mkdtempSync, rmSync, existsSync, readdirSync, utimesSync } from "node:fs";
import * as fs from "node:fs";
import { execSync, spawn } from "node:child_process";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import assert from "node:assert/strict";

const HERE = dirname(fileURLToPath(import.meta.url));
const RUNNER_PATH = join(HERE, "..", "claude", ".claude", "workflows", "docs-page-chain.js");
const HELPER_PATH = join(HERE, "..", "bin", ".local", "bin", "cairn-docs-outline");
const RUNNER_SRC = readFileSync(RUNNER_PATH, "utf8");
const helper = createRequire(import.meta.url)(HELPER_PATH);

const failures = [];
const pending = [];
function check(name, fn) {
  pending.push({ name, fn });
}

// -------------------------------------------------------------------------------------------
// Fixtures: a temp HOME holding the exemplar captures, and a temp worktree holding an outline
// and an interim index shaped like the extend arm's.
// -------------------------------------------------------------------------------------------

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

const OUTLINE = {
  index: "docs/extend/README.md",
  groups: [
    { id: "start", title: "Start", pages: ["architecture", "add-cairn", "what-the-scaffold-wrote"] },
    { id: "operate", title: "Operate", pages: ["debug-your-site"] }
  ],
  pages: [
    {
      slug: "architecture", title: "Architecture", path: "docs/extend/architecture.md", group: "start", order: 1,
      job: "Learn where cairn ends and your site begins.", pageType: "concept",
      exemplars: [{ source: "core/rust-analyzer-architecture/", take: "Take the code map." }, { source: "evaluators/litestream/" }],
      figure: true, figureNote: "One diagram of the write path.", absorbs: ["docs/extend/data-tiers.md"],
      factIds: ["f:a7qx4m", "f:0duu5p"], covers: ["The export map."],
      outOfScope: ["Security properties (security-model)."], pinned: []
    },
    {
      slug: "add-cairn", title: "Add cairn", path: "docs/extend/add-cairn.md", group: "start", order: 2,
      job: "Add cairn to an app.", pageType: "tutorial", exemplars: [], figure: false,
      factIds: [], covers: [], outOfScope: [], pinned: ["#milestone-1"]
    },
    {
      slug: "what-the-scaffold-wrote", path: "docs/extend/what-the-scaffold-wrote.md", group: "start", order: 3,
      job: "Read what the scaffold wrote.", pageType: "reference", exemplars: [], figure: false,
      factIds: [], covers: [], outOfScope: [], pinned: []
    },
    {
      slug: "debug-your-site", path: "docs/extend/debug-your-site.md", group: "operate", order: 23,
      job: "Debug a running site.", pageType: "how-to", exemplars: [], figure: false,
      factIds: ["f:zzzzzz"], covers: [], outOfScope: [], pinned: []
    }
  ]
};

const INDEX = `# Extend cairn

This arm is being rebuilt.

## Start

## Operate

- [Choose an AI posture](./choose-an-ai-posture.md)
- [Upgrade cairn](./upgrade-cairn.md)
`;

function makeHome() {
  const home = tempDir("dpc-home-");
  const root = join(home, ".local", "share", "cairn", "exemplars");
  write(root, "core/rust-analyzer-architecture/page.md", "# RA\n");
  write(root, "evaluators/litestream/page.md", "# LS\n");
  return home;
}

function makeWorktree(outline = OUTLINE) {
  const wt = tempDir("dpc-wt-");
  write(wt, "docs/internal/outlines/extend.json", JSON.stringify(outline, null, 1));
  write(wt, "docs/extend/README.md", INDEX);
  return wt;
}

// -------------------------------------------------------------------------------------------
// Runner harness: the runner is a workflow script with top-level await and a top-level return,
// so it compiles as an async function body with the runtime's ambient globals as parameters.
// -------------------------------------------------------------------------------------------

const AsyncFunction = (async () => {}).constructor;
const runnerBody = RUNNER_SRC.replace(/^export const meta/m, "const meta");
// eslint-disable-next-line no-new-func -- running the runner body with injected globals is the
// documented pattern for a workflow script node cannot import as a module.
const runRunner = new AsyncFunction("args", "agent", "parallel", "log", "phase", "budget", runnerBody);

/** Runs one helper command line the runner rendered, with the helper file standing in for PATH. */
function runHelperLine(line, home) {
  const cmd = line.trim().replace(/^cairn-docs-outline\b/, `node '${HELPER_PATH}'`);
  try {
    return execSync(cmd, { shell: "/bin/bash", env: { ...process.env, HOME: home }, encoding: "utf8" });
  } catch (e) {
    return e.stdout || "";
  }
}

/** The one prompt line that runs the helper's `verb`, or undefined. */
function helperLine(prompt, verb) {
  return prompt.split("\n").find((l) => l.trim().startsWith(`cairn-docs-outline ${verb} `));
}

const ACCEPT = { verdict: "accept", findings: [], summary: "ok" };

/** The fact ids a plan prompt asks the plan to dispose, read from its one list line. */
function planIds(prompt) {
  const m = prompt.match(/Fact ids the plan disposes, every one: ([^\n]*)/);
  return m ? m[1].split(", ").filter((x) => x.startsWith("f:")) : [];
}

/** A stubbed plan step: writes the plan file and places every id it was asked to dispose. */
function stubPlan(wt, prompt) {
  const plan = prompt.match(/docs\/internal\/briefs\/\S+?\.plan\.md/)[0];
  write(wt, plan, "# Plan\n");
  return { plan, claimInventory: planIds(prompt).map((factId) => ({ claim: "a planned fact", disposition: "carried", factId, section: "Section A" })) };
}

/**
 * A stubbed agent: the outline probe runs its rendered command and returns the JSON verbatim;
 * page inputs echo the page's outline job; the drafter writes the page and runs its rendered
 * link command (a redraft after the final reader read included); every read accepts.
 */
function makeAgent(wt, home, calls, overrides = {}) {
  return async (prompt, opts) => {
    calls.push({ label: opts.label, prompt, opts });
    const kind = opts.label.split(":")[0];
    if (overrides[kind]) return overrides[kind](prompt, opts);
    if (kind === "outline") return JSON.parse(runHelperLine(helperLine(prompt, "resolve"), home));
    if (kind === "rework-state") return JSON.parse(runHelperLine(helperLine(prompt, "rework-state"), home));
    if (kind === "inputs") return { job: "the job", pageType: "concept", factIds: ["f:a7qx4m"], claimInventory: [] };
    if (kind === "plan" || kind === "replan") return stubPlan(wt, prompt);
    if (kind === "probe") return { exists: true };
    if (kind === "framing") return { framing: prompt.match(/docs\/internal\/briefs\/\S+?\.framing\.md/)[0], readers: ["a reader"], factIds: [] };
    if (kind === "draft" || kind === "redraft" || kind === "reader-redraft" || kind === "rework") {
      const path = prompt.match(/(?:Draft the page|Redraft) (\S+)/)[1];
      write(wt, path, "# Architecture of cairn\n\nBody.\n");
      const link = helperLine(prompt, "link");
      return { path, gate: "pass", gateCommand: "stub", indexLink: link ? runHelperLine(link, home).trim() : undefined };
    }
    return ACCEPT;
  };
}

const parallel = (fns) => Promise.all(fns.map((f) => f()));
const noop = () => {};

function baseArgs(wt, pages, extra = {}) {
  return { worktree: wt, gate: "npm run check:docs-gate -- --page {page} --brief {brief}", pages, ...extra };
}

// -------------------------------------------------------------------------------------------
// The runner's extracted merge block, and checksum parity with the helper.
// -------------------------------------------------------------------------------------------

const START = "// === OUTLINE MERGE (docs-page-chain-outline.test.mjs extracts this block) ===";
const END = "// === END OUTLINE MERGE ===";
const startAt = RUNNER_SRC.indexOf(START);
const endAt = RUNNER_SRC.indexOf(END);
let runnerMerge = null;
if (startAt !== -1 && endAt > startAt) {
  // eslint-disable-next-line no-new-func -- same extraction pattern as the derivation test.
  runnerMerge = new Function(`${RUNNER_SRC.slice(startAt, endAt)}\nreturn { canonicalEntry, fnv, checksumOf, probeChecksum, mergeOutline, reworkChecksum: typeof reworkChecksum === "function" ? reworkChecksum : undefined };`)();
}

check("the merge markers are present, in order", () => {
  assert.ok(runnerMerge, "OUTLINE MERGE markers missing or out of order");
});

check("the runner and the helper hash every entry identically", () => {
  const home = makeHome();
  const exemplarRoot = join(home, ".local", "share", "cairn", "exemplars");
  const r = helper.resolveEntries(OUTLINE, OUTLINE.pages.map((p) => p.path), { exemplarRoot, exists: () => true });
  for (const e of r.entries) {
    assert.equal(runnerMerge.checksumOf(e), e.checksum, e.path);
    assert.equal(helper.checksumOf(e), e.checksum, e.path);
  }
  assert.equal(runnerMerge.canonicalEntry.toString(), helper.canonicalEntry.toString());
  assert.equal(runnerMerge.checksumOf.toString(), helper.checksumOf.toString());
  assert.equal(runnerMerge.fnv.toString(), helper.fnv.toString());
  assert.equal(runnerMerge.probeChecksum.toString(), helper.probeChecksum.toString());
  assert.equal(runnerMerge.probeChecksum(r), r.checksum);
});

// -------------------------------------------------------------------------------------------
// Outline lookup.
// -------------------------------------------------------------------------------------------

check("resolve returns the matching entry with exemplar captures mapped to page.md", () => {
  const r = helper.resolveEntries(OUTLINE, ["docs/extend/architecture.md"], { exemplarRoot: "/ex", exists: () => true });
  assert.equal(r.ok, true);
  assert.equal(r.index, "docs/extend/README.md");
  assert.deepEqual(r.missing, []);
  const e = r.entries[0];
  assert.deepEqual(e.exemplarSources, ["/ex/core/rust-analyzer-architecture/page.md", "/ex/evaluators/litestream/page.md"]);
  assert.deepEqual(e.factIds, ["f:a7qx4m", "f:0duu5p"]);
  assert.deepEqual(e.covers, ["The export map."]);
  assert.deepEqual(e.outOfScope, ["Security properties (security-model)."]);
  assert.equal(e.figure, true);
  assert.equal(e.job, "Learn where cairn ends and your site begins.");
  assert.equal(e.pageType, "concept");
});

check("resolve carries each exemplar's take line, the figure note, and the absorbed pages", () => {
  const r = helper.resolveEntries(OUTLINE, ["docs/extend/architecture.md"], { exemplarRoot: "/ex", exists: () => true });
  const e = r.entries[0];
  assert.deepEqual(e.exemplarTakes, ["Take the code map.", ""]);
  assert.equal(e.figureNote, "One diagram of the write path.");
  assert.deepEqual(e.absorbs, ["docs/extend/data-tiers.md"]);
});

check("resolve carries the entry's title, and an entry with none carries an empty one", () => {
  const r = helper.resolveEntries(OUTLINE, ["docs/extend/architecture.md", "docs/extend/debug-your-site.md"], { exemplarRoot: "/ex", exists: () => true });
  assert.equal(r.entries[0].title, "Architecture");
  assert.equal(r.entries[1].title, "");
});

check("resolve names a path the outline does not carry and is not ok", () => {
  const r = helper.resolveEntries(OUTLINE, ["docs/extend/architecture.md", "docs/extend/nope.md"], { exemplarRoot: "/ex", exists: () => true });
  assert.equal(r.ok, false);
  assert.deepEqual(r.missing, ["docs/extend/nope.md"]);
  assert.match(r.error, /nope\.md/);
});

check("resolve fails an exemplar capture with no page file", () => {
  const r = helper.resolveEntries(OUTLINE, ["docs/extend/architecture.md"], { exemplarRoot: "/ex", exists: (f) => !f.includes("litestream") });
  assert.equal(r.ok, false);
  assert.match(r.error, /exemplar file not found: \/ex\/evaluators\/litestream\/page\.md/);
});

// -------------------------------------------------------------------------------------------
// Index link insertion.
// -------------------------------------------------------------------------------------------

const orders = { "./a.md": 1, "./b.md": 2, "./c.md": 3, "./d.md": 23 };
function spec(heading, order, dest) {
  return {
    heading, order, line: `- [${dest}](${dest})`,
    isPage: (x) => x === dest,
    orderOf: (x) => orders[x]
  };
}

check("a link lands under its group heading in an empty section, before the next heading", () => {
  const r = helper.insertIndexLink(INDEX, spec("Start", 2, "./b.md"));
  assert.equal(r.status, "added");
  assert.match(r.text, /## Start\n\n- \[\.\/b\.md\]\(\.\/b\.md\)\n\n## Operate/);
});

check("links keep outline order whatever order they arrive in", () => {
  let text = INDEX;
  for (const [d, o] of [["./c.md", 3], ["./a.md", 1], ["./b.md", 2]]) text = helper.insertIndexLink(text, spec("Start", o, d)).text;
  assert.match(text, /## Start\n\n- \[\.\/a\.md\]\(\.\/a\.md\)\n- \[\.\/b\.md\]\(\.\/b\.md\)\n- \[\.\/c\.md\]\(\.\/c\.md\)\n\n## Operate/);
});

check("a section holding only kept pages takes the new link after them", () => {
  const r = helper.insertIndexLink(INDEX, spec("Operate", 23, "./d.md"));
  assert.match(r.text, /- \[Upgrade cairn\]\(\.\/upgrade-cairn\.md\)\n- \[\.\/d\.md\]\(\.\/d\.md\)\n$/);
});

check("a list item's continuation lines stay with it", () => {
  const text = "## Start\n\n- [A](./a.md): one\n  continued\n\n## Next\n";
  const r = helper.insertIndexLink(text, spec("Start", 2, "./b.md"));
  assert.equal(r.text, "## Start\n\n- [A](./a.md): one\n  continued\n- [./b.md](./b.md)\n\n## Next\n");
});

check("a page the index already links is present, and the text is unchanged", () => {
  const once = helper.insertIndexLink(INDEX, spec("Start", 2, "./b.md")).text;
  const twice = helper.insertIndexLink(once, spec("Start", 2, "./b.md"));
  assert.equal(twice.status, "present");
  assert.equal(twice.text, once);
});

check("an index with no heading for the group throws, naming the heading", () => {
  assert.throws(() => helper.insertIndexLink(INDEX, spec("Model content", 4, "./x.md")), /no heading "Model content"/);
});

check("the page title skips frontmatter and falls back to null", () => {
  assert.equal(helper.pageTitle("---\ntitle: x\n---\n# Real title\n\nBody"), "Real title");
  assert.equal(helper.pageTitle("no heading"), null);
});

// A race, so a regression here is probabilistic: with the lock removed, the helper lost a link in
// about two runs of three when this was written, never all of them.
check("four link processes at once leave all four links, in outline order", async () => {
  const wt = makeWorktree();
  write(wt, "docs/extend/architecture.md", "# Architecture\n");
  write(wt, "docs/extend/add-cairn.md", "# Add cairn\n");
  write(wt, "docs/extend/what-the-scaffold-wrote.md", "# What the scaffold wrote\n");
  write(wt, "docs/extend/debug-your-site.md", "# Debug your site\n");
  const run = (page) => new Promise((res, rej) => {
    const c = spawn("node", [HELPER_PATH, "link", "--worktree", wt, "--outline", "docs/internal/outlines/extend.json", "--page", page]);
    c.on("exit", (code) => (code === 0 ? res() : rej(new Error(`exit ${code}`))));
  });
  await Promise.all(["add-cairn", "debug-your-site", "what-the-scaffold-wrote", "architecture"].map((s) => run(`docs/extend/${s}.md`)));
  const index = readFileSync(join(wt, "docs/extend/README.md"), "utf8");
  assert.match(index, /## Start\n\n- \[Architecture\]\(\.\/architecture\.md\)\n- \[Add cairn\]\(\.\/add-cairn\.md\)\n- \[What the scaffold wrote\]\(\.\/what-the-scaffold-wrote\.md\)\n\n## Operate/);
  assert.match(index, /- \[Upgrade cairn\]\(\.\/upgrade-cairn\.md\)\n- \[Debug your site\]\(\.\/debug-your-site\.md\)\n$/);
});

function staleLock() {
  const dir = tempDir("dpc-lock-");
  const lock = join(dir, "x.lock");
  writeFileSync(lock, "");
  const old = new Date(Date.now() - 5 * 60_000);
  utimesSync(lock, old, old);
  return { dir, lock };
}

check("a stale lock is broken once: the first breaker wins, a second finds nothing to break", () => {
  const { dir, lock } = staleLock();
  assert.equal(helper.breakStaleLock(lock, 60_000), true);
  assert.equal(existsSync(lock), false);
  assert.equal(helper.breakStaleLock(lock, 60_000), false);
  assert.deepEqual(readdirSync(dir), []);
});

check("a fresh lock is never broken", () => {
  const dir = tempDir("dpc-lock-");
  const lock = join(dir, "x.lock");
  writeFileSync(lock, "live");
  assert.equal(helper.breakStaleLock(lock, 60_000), false);
  assert.equal(readFileSync(lock, "utf8"), "live");
});

check("a live lock taken between the stale check and the rename is restored, not broken", () => {
  const { dir, lock } = staleLock();
  let swapped = false;
  const racing = {
    ...fs,
    statSync: (p, ...rest) => {
      const st = fs.statSync(p, ...rest);
      if (!swapped && p === lock) {
        swapped = true;
        rmSync(lock);
        writeFileSync(lock, "live");
      }
      return st;
    }
  };
  assert.equal(helper.breakStaleLock(lock, 60_000, racing), false);
  assert.equal(readFileSync(lock, "utf8"), "live");
  assert.deepEqual(readdirSync(dir), ["x.lock"]);
});

// -------------------------------------------------------------------------------------------
// The runner's merge.
// -------------------------------------------------------------------------------------------

function probeFor(paths) {
  return helper.resolveEntries(OUTLINE, paths, { exemplarRoot: "/ex", exists: () => true });
}

check("an inline field overrides the outline's; an absent one comes from the outline", () => {
  const { pages: [p] } = runnerMerge.mergeOutline(
    [{ id: "architecture", path: "docs/extend/architecture.md", track: "extend", figure: false, job: "inline job" }],
    probeFor(["docs/extend/architecture.md"]), "o.json");
  assert.equal(p.figure, false);
  assert.equal(p.job, "inline job");
  assert.equal(p.pageType, "concept");
  assert.deepEqual(p.factIds, ["f:a7qx4m", "f:0duu5p"]);
  assert.equal(p.id, "architecture");
});

check("a page the probe reports no entry for fails, named, even when the probe says ok", () => {
  const probe = { ...probeFor(["docs/extend/architecture.md"]), ok: true };
  assert.throws(
    () => runnerMerge.mergeOutline([{ path: "docs/extend/architecture.md" }, { path: "docs/extend/gone.md" }], probe, "o.json"),
    /not in the outline o\.json: docs\/extend\/gone\.md/);
});

for (const [field, value] of [["exemplarTakes", ["x", ""]], ["figureNote", "other"], ["absorbs", []], ["title", "Other title"]]) {
  check(`the entry checksum covers ${field}`, () => {
    const probe = probeFor(["docs/extend/architecture.md"]);
    probe.entries[0][field] = value;
    assert.throws(() => runnerMerge.mergeOutline([{ path: "docs/extend/architecture.md" }], probe, "o.json"), /mistranscribed docs\/extend\/architecture\.md/);
  });
}

check("a probe that drops the index fails the run", () => {
  const probe = probeFor(["docs/extend/architecture.md"]);
  probe.index = "";
  assert.throws(() => runnerMerge.mergeOutline([{ path: "docs/extend/architecture.md" }], probe, "o.json"), /mistranscribed its report/);
});

check("a probe that flips ok and drops its error fails the run", () => {
  const probe = helper.resolveEntries(OUTLINE, ["docs/extend/architecture.md"], { exemplarRoot: "/ex", exists: () => false });
  assert.equal(probe.ok, false);
  probe.ok = true;
  delete probe.error;
  assert.throws(() => runnerMerge.mergeOutline([{ path: "docs/extend/architecture.md" }], probe, "o.json"), /mistranscribed its report/);
});

check("the merge returns the verified index", () => {
  assert.equal(runnerMerge.mergeOutline([{ path: "docs/extend/architecture.md" }], probeFor(["docs/extend/architecture.md"]), "o.json").index, "docs/extend/README.md");
});

check("inline exemplar sources drop the outline's take lines", () => {
  const { pages: [p] } = runnerMerge.mergeOutline([{ path: "docs/extend/architecture.md", exemplarSources: ["/x/page.md"] }], probeFor(["docs/extend/architecture.md"]), "o.json");
  assert.deepEqual(p.exemplarSources, ["/x/page.md"]);
  assert.deepEqual(p.exemplarTakes, []);
});

check("a mistranscribed entry fails its checksum, naming the page", () => {
  const probe = probeFor(["docs/extend/architecture.md"]);
  probe.entries[0].factIds = ["f:a7qx4m"];
  assert.throws(() => runnerMerge.mergeOutline([{ path: "docs/extend/architecture.md" }], probe, "o.json"), /mistranscribed docs\/extend\/architecture\.md/);
});

// -------------------------------------------------------------------------------------------
// Dry runs of the whole runner.
// -------------------------------------------------------------------------------------------

check("dry run: one outline page resolves its entry, feeds the prompts, and links the index once", async () => {
  const home = makeHome();
  const wt = makeWorktree();
  const page = { id: "architecture", path: "docs/extend/architecture.md", track: "extend" };
  const args = baseArgs(wt, [page], { outline: "docs/internal/outlines/extend.json" });

  const calls = [];
  const out = await runRunner(args, makeAgent(wt, home, calls), parallel, noop, noop);
  assert.equal(out.accepted, 1);
  assert.deepEqual(calls.map((c) => c.label), ["outline", "inputs:architecture", "plan:architecture", "structure:architecture:plan", "framing:architecture", "probe:architecture:framing", "draft:architecture", "structure:architecture:r1", "editor:architecture:r1", "facts:architecture:r1", "figure:architecture:r1", "reader:architecture"]);

  const inputs = calls.find((c) => c.label.startsWith("inputs:")).prompt;
  assert.match(inputs, /Learn where cairn ends and your site begins\./);
  assert.match(inputs, /The outline's page type: concept/);
  assert.match(inputs, /traced to this page's sources: f:a7qx4m, f:0duu5p/);
  assert.match(inputs, /- The export map\./);
  assert.match(inputs, /- Security properties \(security-model\)\./);

  assert.match(inputs, /rust-analyzer-architecture\/page\.md\n  Take: Take the code map\./);

  const draft = calls.find((c) => c.label.startsWith("draft:")).prompt;
  assert.match(draft, /rust-analyzer-architecture\/page\.md\n  Take: Take the code map\.\n- \S+litestream\/page\.md\n/);
  assert.match(draft, /Figure note, from the outline: One diagram of the write path\./);

  const facts = calls.find((c) => c.label.startsWith("facts:")).prompt;
  assert.match(facts, /The outline's fact ids for this page: f:a7qx4m, f:0duu5p/);
  assert.match(facts, /## docs\/extend\/data-tiers\.md/);

  const figure = calls.find((c) => c.label.startsWith("figure:")).prompt;
  assert.match(figure, /Figure note, from the outline: One diagram of the write path\./);

  assert.ok(draft.includes(`- ${join(home, ".local/share/cairn/exemplars/core/rust-analyzer-architecture/page.md")}`));
  assert.match(draft, /Out of scope, owned by another page; keep it off this page:\n- Security properties/);
  assert.ok(draft.indexOf("cairn-docs-outline link") < draft.indexOf("cairn-run-gate"), "the link step precedes the gate");

  const record = out.pages[0];
  assert.match(record.rounds[0].draft.indexLink, /^added docs\/extend\/README\.md: \.\/architecture\.md under "Start"$/);
  const first = readFileSync(join(wt, "docs/extend/README.md"), "utf8");
  assert.match(first, /## Start\n\n- \[Architecture of cairn\]\(\.\/architecture\.md\)\n\n## Operate/);

  // A second run on the same page (a redraft, or a re-run of the chain) adds no second link.
  const calls2 = [];
  const out2 = await runRunner(args, makeAgent(wt, home, calls2), parallel, noop, noop);
  assert.match(out2.pages[0].rounds[0].draft.indexLink, /^present /);
  assert.equal(readFileSync(join(wt, "docs/extend/README.md"), "utf8"), first);
});

check("dry run: a redraft round runs the link step again and adds no second link", async () => {
  const home = makeHome();
  const wt = makeWorktree();
  const args = baseArgs(wt, [{ id: "debug", path: "docs/extend/debug-your-site.md", track: "extend" }], { outline: "docs/internal/outlines/extend.json" });
  let editorRound = 0;
  const calls = [];
  const agent = makeAgent(wt, home, calls, {
    editor: async () => (++editorRound === 1 ? { verdict: "fix", findings: [{ location: "l1", finding: "f", blocking: true }], summary: "fix" } : ACCEPT)
  });
  const out = await runRunner(args, agent, parallel, noop, noop);
  assert.equal(out.accepted, 1);
  assert.match(out.pages[0].rounds[0].draft.indexLink, /^added /);
  assert.match(out.pages[0].rounds[1].draft.indexLink, /^present /);
  const index = readFileSync(join(wt, "docs/extend/README.md"), "utf8");
  assert.equal(index.split("./debug-your-site.md").length - 1, 1);
});

check("dry run: a page path missing from the outline fails the run, named, before any page agent", async () => {
  const home = makeHome();
  const wt = makeWorktree();
  const args = baseArgs(wt, [
    { id: "architecture", path: "docs/extend/architecture.md", track: "extend" },
    { id: "ghost", path: "docs/extend/ghost.md", track: "extend" }
  ], { outline: "docs/internal/outlines/extend.json" });
  const calls = [];
  await assert.rejects(runRunner(args, makeAgent(wt, home, calls), parallel, noop, noop), /not in the outline docs\/internal\/outlines\/extend\.json: docs\/extend\/ghost\.md/);
  assert.deepEqual(calls.map((c) => c.label), ["outline"]);
});

check("dry run: without an outline there is no probe and no link step", async () => {
  const home = makeHome();
  const wt = makeWorktree();
  const args = baseArgs(wt, [{ id: "x", path: "docs/extend/architecture.md", track: "extend", job: "inline job" }]);
  const calls = [];
  const out = await runRunner(args, makeAgent(wt, home, calls), parallel, noop, noop);
  assert.equal(out.accepted, 1);
  assert.equal(calls[0].label, "inputs:x");
  assert.ok(!calls.some((c) => c.prompt.includes("cairn-docs-outline")));
});

check("dry run: a page with no job, inline or from an outline, fails before any agent", async () => {
  const calls = [];
  await assert.rejects(runRunner(baseArgs("/wt", [{ id: "x", path: "docs/extend/x.md", track: "extend" }]), makeAgent("/wt", "/h", calls), parallel, noop, noop), /has no job/);
  assert.equal(calls.length, 0);
});

// -------------------------------------------------------------------------------------------
// Dry runs of the map rows, the friction route, the title, the voice line, figures, and spent.
// -------------------------------------------------------------------------------------------

const MAP_PATH = "docs/internal/option-map.json";
const FIX = { verdict: "fix", findings: [{ location: "l1", finding: "f", blocking: true }], summary: "fix" };

/** A stubbed budget whose spent() climbs by `step` per call, starting at `start`. */
function makeBudget(start, step) {
  let n = start - step;
  return { spent: () => (n += step) };
}

/** The pilot-page dry run: every agent that may report friction reports a stub value. */
async function frictionDryRun(extraArgs = {}, budget) {
  const home = makeHome();
  const wt = makeWorktree();
  const args = baseArgs(wt, [{ id: "architecture", path: "docs/extend/architecture.md", track: "extend" }], { outline: "docs/internal/outlines/extend.json", ...extraArgs });
  const calls = [];
  const inner = makeAgent(wt, home, []);
  const agent = makeAgent(wt, home, calls, {
    inputs: async () => ({
      job: "the job", pageType: "concept", factIds: ["f:a7qx4m"], claimInventory: [],
      rowsReceived: [{ key: "CairnAdapter.editor.nav", value: "pending architecture" }],
      rowsDisposed: [{ key: "CairnAdapter.editor.nav", disposition: "filed", value: "f:new001" }],
      frictionFiled: ["inputs-friction"]
    }),
    draft: async (prompt, opts) => ({ ...(await inner(prompt, opts)), frictionFiled: ["drafter-friction"] }),
    facts: async () => ({ ...ACCEPT, frictionFiled: ["fact-read-friction"] }),
    editor: async () => ({ ...ACCEPT, frictionFiled: ["editor-friction"] })
  });
  const out = await runRunner(args, agent, parallel, noop, noop, budget);
  const prompt = (kind) => calls.find((c) => c.label.startsWith(`${kind}:`)).prompt;
  return { out, calls, prompt, flat: (kind) => prompt(kind).replace(/\s+/g, " ") };
}

check("dry run: page inputs gets the map path, the page's slug, and the row selection rule", async () => {
  const { flat } = await frictionDryRun();
  const inputs = flat("inputs");
  assert.ok(inputs.includes(MAP_PATH), "the map path");
  assert.match(inputs, /This page's slug is "architecture"/);
  assert.match(inputs, /every row whose value is "pending architecture"/);
  assert.match(inputs, /every row whose value is one of these fact ids: f:a7qx4m, f:0duu5p/);
  assert.match(inputs, /read them live from the file/);
  assert.match(inputs, /"re-pointed"/);
});

check("dry run: page inputs and the fact read carry the retag order, the fact read's as a blocking finding", async () => {
  const { flat: prompt } = await frictionDryRun();
  const order = /raise pendingCount once, by the number of those rows; then rewrite every one of those rows to "pending architecture"; and only then retag the fact, once/;
  const inputs = prompt("inputs");
  assert.match(inputs, order);
  assert.match(inputs, /retag it \[candidate\]/);
  const facts = prompt("facts");
  assert.match(facts, order);
  assert.match(facts, /\[docs-drift\]/);
  assert.ok(facts.includes(MAP_PATH), "the fact read names the map");
  assert.match(facts, /Each row you rewrote is a blocking finding, located at docs\/internal\/option-map\.json/);
});

check("dry run: the fact read blocks on any row still pending this page's slug in the live map, in every round", async () => {
  const home = makeHome();
  const wt = makeWorktree();
  const args = baseArgs(wt, [{ id: "debug", path: "docs/extend/debug-your-site.md", track: "extend" }], { outline: "docs/internal/outlines/extend.json" });
  let round = 0;
  const calls = [];
  const agent = makeAgent(wt, home, calls, { facts: async () => (++round === 1 ? FIX : ACCEPT) });
  await runRunner(args, agent, parallel, noop, noop);
  const factPrompts = calls.filter((c) => c.label.startsWith("facts:")).map((c) => c.prompt.replace(/\s+/g, " "));
  assert.equal(factPrompts.length, 2);
  for (const facts of factPrompts) {
    assert.match(facts, /Read docs\/internal\/option-map\.json live now: every row whose value still reads "pending debug-your-site" is a blocking finding/);
  }
});

check("dry run: the drafter, page inputs, and the fact read carry the friction smells; the editor does not", async () => {
  const { flat: prompt } = await frictionDryRun();
  const smells = /a hedge, a caveat, an exception, a workaround, a surprising default, or two seams naming or behaving the same thing differently/;
  for (const kind of ["inputs", "draft", "facts"]) {
    assert.match(prompt(kind), smells, kind);
    assert.match(prompt(kind), /fact ids or file:line/, kind);
    assert.match(prompt(kind), /docs\/internal\/docs-friction-log\.md/, kind);
  }
  assert.doesNotMatch(prompt("editor"), smells);
  assert.doesNotMatch(prompt("draft"), /a genuine design gap/);
});

check("dry run: the record carries the rows and friction from page inputs, the drafter, and the fact read, none from the editor", async () => {
  const { out } = await frictionDryRun();
  const record = out.pages[0];
  assert.deepEqual(record.pageInputs.rowsReceived, [{ key: "CairnAdapter.editor.nav", value: "pending architecture" }]);
  assert.deepEqual(record.pageInputs.rowsDisposed, [{ key: "CairnAdapter.editor.nav", disposition: "filed", value: "f:new001" }]);
  assert.deepEqual(record.pageInputs.frictionFiled, ["inputs-friction"]);
  assert.deepEqual(record.rounds[0].draft.frictionFiled, ["drafter-friction"]);
  const reads = record.rounds[0].reads;
  assert.deepEqual(reads.find((r) => r.read === "fact read").frictionFiled, ["fact-read-friction"]);
  assert.equal(reads.find((r) => r.read === "register editor").frictionFiled, undefined);
  assert.ok(!JSON.stringify(out).includes("editor-friction"), "the editor's friction reaches no record");
});

check("dry run: the fact read's friction reaches the record in a redraft round too", async () => {
  const home = makeHome();
  const wt = makeWorktree();
  const args = baseArgs(wt, [{ id: "debug", path: "docs/extend/debug-your-site.md", track: "extend" }], { outline: "docs/internal/outlines/extend.json" });
  let round = 0;
  const agent = makeAgent(wt, home, [], {
    facts: async () => (++round === 1 ? { ...FIX, frictionFiled: ["r1"] } : { ...ACCEPT, frictionFiled: ["r2"] })
  });
  const out = await runRunner(args, agent, parallel, noop, noop);
  const record = out.pages[0];
  assert.equal(record.status, "accepted");
  assert.deepEqual(record.rounds[0].reads.find((r) => r.read === "fact read").frictionFiled, ["r1"]);
  assert.deepEqual(record.rounds[1].reads.find((r) => r.read === "fact read").frictionFiled, ["r2"]);
});

check("dry run: the drafter gets the entry's title as the H1, and the voice source line", async () => {
  const { prompt, flat } = await frictionDryRun();
  assert.match(prompt("draft"), /The page's H1, verbatim: # Architecture\n/);
  const draft = flat("draft");
  assert.match(draft, /Voice comes only from the register's drafting brief and its primary exemplar, docs\/extend\/choose-an-ai-posture\.md/);
  assert.match(draft, /supply structure and detail per step, never voice or wording/);
});

check("dry run: a page with no outline title gets no H1 line", async () => {
  const home = makeHome();
  const wt = makeWorktree();
  const calls = [];
  await runRunner(baseArgs(wt, [{ id: "debug", path: "docs/extend/debug-your-site.md", track: "extend" }], { outline: "docs/internal/outlines/extend.json" }), makeAgent(wt, home, calls), parallel, noop, noop);
  assert.doesNotMatch(calls.find((c) => c.label.startsWith("draft:")).prompt, /The page's H1/);
});

check("dry run: a figure page's drafter prompt names the cairn-figure skill; a page without one does not", async () => {
  const { prompt } = await frictionDryRun();
  assert.ok(prompt("draft").includes("~/.claude/skills/cairn-figure/SKILL.md"));
  const home = makeHome();
  const wt = makeWorktree();
  const calls = [];
  await runRunner(baseArgs(wt, [{ id: "debug", path: "docs/extend/debug-your-site.md", track: "extend" }], { outline: "docs/internal/outlines/extend.json" }), makeAgent(wt, home, calls), parallel, noop, noop);
  assert.ok(!calls.find((c) => c.label.startsWith("draft:")).prompt.includes("cairn-figure"));
});

check("dry run: the return carries spent, the budget delta across the run, and null with no budget", async () => {
  const { out } = await frictionDryRun({}, makeBudget(100, 42));
  assert.equal(out.spent, 42);
  const bare = await frictionDryRun();
  assert.equal(bare.out.spent, null);
});

// -------------------------------------------------------------------------------------------
// Dry runs of the structural edit seat, the final reader read, and the drafter's anatomy line.
// -------------------------------------------------------------------------------------------

const RECORD = "docs/superpowers/research/2026-09-30-page-level-review-prior-art.md";
const STRUCT_FIX = { verdict: "fix", findings: [{ location: "docs/extend/architecture.md:3", finding: "STRUCT-FINDING intro omits scope", blocking: true }], summary: "structure fix" };
const READER_FIX = { verdict: "fix", findings: [{ location: "docs/extend/architecture.md:9", finding: "READER-FINDING had to infer the seam", blocking: true }], summary: "reader fix", paraphrase: "READER-PARAPHRASE" };

/** One outline page through the runner with stubbed agents; returns its calls and its record. */
async function seatDryRun(pagePath, overrides = {}, extraArgs = {}) {
  const home = makeHome();
  const wt = makeWorktree();
  const id = pagePath.replace(/^.*\/|\.md$/g, "");
  const calls = [];
  const out = await runRunner(baseArgs(wt, [{ id, path: pagePath, track: "extend" }], { outline: "docs/internal/outlines/extend.json", ...extraArgs }), makeAgent(wt, home, calls, overrides), parallel, noop, noop);
  const labels = calls.map((c) => c.label);
  const prompt = (label) => calls.find((c) => c.label === label).prompt;
  const opts = (label) => calls.find((c) => c.label === label).opts;
  return { out, record: out.pages[0], labels, prompt, opts, flat: (label) => prompt(label).replace(/\s+/g, " ") };
}

check("dry run: the structural edit seat runs in round 1 with the outline entry and the published checklist", async () => {
  const { labels, flat } = await seatDryRun("docs/extend/architecture.md");
  assert.ok(labels.includes("structure:architecture:r1"));
  const s = flat("structure:architecture:r1");
  assert.match(s, /Learn where cairn ends and your site begins\./, "the outline job");
  assert.match(s, /- The export map\./, "the outline covers");
  assert.match(s, /- Security properties \(security-model\)\./, "the outline's out of scope");
  assert.match(s, /crossLinks.*"from" is "architecture"/, "the outline's cross-links for this slug");
  assert.ok(s.includes("docs/internal/outlines/extend.json"), "the outline path");
  for (const item of ["Module types are not mixed.", "Module types are used correctly.", "Information is provided at the right pace.",
    "Information is presented in the most logical order and location.", "Cross-references are used appropriately and only when useful.",
    "The user goal is clear.", "Tasks reflect the intended goal of the user.", "Troubleshooting and error recognition steps are included where appropriate.",
    "What the document covers.", "What prior knowledge you expect readers to have.", "What the document doesn't cover.",
    "Does your introduction provide an accurate overview of the topics you cover?",
    "Most readers appreciate at least a brief introduction under each heading to provide some context."]) {
    assert.ok(s.includes(item), `checklist item: ${item}`);
  }
  assert.ok(!s.includes("Tags and entities are used correctly."), "the inapplicable AsciiDoc item is left out");
  assert.ok(!s.includes("- a table of contents menu"), "the table of contents item is not a checklist line");
  assert.match(s, /"a table of contents menu that shows users where they are in the document" is left out: the site renderer \(cairn\.pub\) supplies the table of contents, not the page\./, "marked inapplicable, with the reason");
  assert.match(s, /## The page anatomies/, "the register's page anatomies, by heading");
  assert.ok(s.includes(RECORD), "the prior-art record");
  assert.match(s, /never edits/);
});

check("dry run: the structural seat's fix reaches the redraft prompt and it re-reads in lean mode; the editor does not", async () => {
  let n = 0;
  const { labels, flat, record } = await seatDryRun("docs/extend/architecture.md", { structure: async (p, o) => (o.label.endsWith(":r1") && ++n === 1 ? STRUCT_FIX : ACCEPT) });
  assert.match(flat("redraft:architecture"), /## structural edit: fix/);
  assert.match(flat("redraft:architecture"), /STRUCT-FINDING intro omits scope/);
  assert.ok(labels.includes("structure:architecture:r2"));
  assert.ok(!labels.includes("editor:architecture:r2"));
  assert.equal(record.rounds[0].reads.find((r) => r.read === "structural edit").verdict, "fix");
  assert.equal(record.status, "accepted");
});

check("dry run: with bothReviewers, the structural seat runs again in round 2 beside every reviewer", async () => {
  const { labels, record } = await seatDryRun("docs/extend/debug-your-site.md", { editor: async (p, o) => (o.label.endsWith(":r1") ? FIX : ACCEPT) }, { bothReviewers: true });
  for (const l of ["structure:debug-your-site:r2", "editor:debug-your-site:r2", "facts:debug-your-site:r2"]) assert.ok(labels.includes(l), l);
  assert.equal(record.crossRegression, false);
});

check("dry run: the final reader read runs last, after every read accepts, and its verdict lands in the record", async () => {
  const { labels, record, prompt, flat } = await seatDryRun("docs/extend/architecture.md", { reader: async () => ({ ...ACCEPT, paraphrase: "READER-PARAPHRASE" }) });
  assert.equal(labels[labels.length - 1], "reader:architecture");
  assert.equal(labels.filter((l) => l.startsWith("reader")).length, 1);
  assert.equal(record.status, "accepted");
  assert.equal(record.finalRead.verdict, "accept");
  assert.equal(record.finalRead.paraphrase, "READER-PARAPHRASE");
  assert.deepEqual(record.finalRead.findings, []);
  assert.ok(!prompt("reader:architecture").includes("Learn where cairn ends"), "the job is not in the reader's prompt");
  const r = flat("reader:architecture");
  assert.ok(!r.includes("The export map."), "the covers are not in the reader's prompt");
  assert.ok(r.includes("What do you think the writer was trying to do with this document?"));
  assert.match(r, /tell you in his or her own words what that section means/, "a concept page's paraphrase test");
  assert.ok(r.indexOf("What do you think the writer") < r.indexOf("docs/internal/outlines/extend.json"), "the outline is named only after the paraphrase step");
});

check("dry run: a task page's reader follows it as a reader would and logs each inference", async () => {
  const { flat } = await seatDryRun("docs/extend/debug-your-site.md", { inputs: async () => ({ job: "j", pageType: "task guide", factIds: [], claimInventory: [], rowsReceived: [], rowsDisposed: [] }) });
  const r = flat("reader:debug-your-site");
  assert.ok(r.includes("following it as a reader would"));
  assert.ok(r.includes("logging every point where it had to infer"));
  assert.doesNotMatch(r, /his or her own words/);
});

check("dry run: the final reader read never runs before acceptance, and never on an escalated page", async () => {
  const { labels, record } = await seatDryRun("docs/extend/debug-your-site.md", { editor: async () => FIX });
  assert.equal(record.status, "escalate");
  assert.ok(!labels.some((l) => l.startsWith("reader")));
  const second = await seatDryRun("docs/extend/debug-your-site.md", { editor: async (p, o) => (o.label.endsWith(":r1") ? FIX : ACCEPT) });
  assert.ok(second.labels.indexOf("reader:debug-your-site") > second.labels.indexOf("editor:debug-your-site:r2"), "after the round-2 reads");
});

check("dry run: the final reader read's fix drives one scoped redraft, scoped register and fact reads, and a re-test that accepts", async () => {
  let n = 0;
  const { labels, record, flat } = await seatDryRun("docs/extend/architecture.md", { reader: async () => (++n === 1 ? READER_FIX : { ...ACCEPT, paraphrase: "RETEST-PARAPHRASE" }) });
  const tail = labels.slice(labels.indexOf("reader:architecture"));
  assert.deepEqual(tail, ["reader:architecture", "reader-redraft:architecture", "editor:architecture:final", "facts:architecture:final", "reader:architecture:retest"]);
  assert.equal(record.status, "accepted");
  assert.equal(record.finalRead.retest.verdict, "accept");
  assert.equal(record.finalRead.retest.paraphrase, "RETEST-PARAPHRASE");
  assert.equal(record.finalRead.verdict, "fix");
  assert.deepEqual(record.finalRead.findings, READER_FIX.findings);
  assert.equal(record.finalRead.paraphrase, "READER-PARAPHRASE");
  assert.equal(record.finalRead.redraft.gate, "pass");
  assert.deepEqual(record.finalRead.reads.map((r) => r.read), ["register editor", "fact read"]);
  assert.equal(record.rounds.length, 1, "the scoped redraft is not a chain round");
  const redraft = flat("reader-redraft:architecture");
  assert.match(redraft, /READER-FINDING had to infer the seam/);
  assert.match(redraft, /only the spots/);
  for (const l of ["editor:architecture:final", "facts:architecture:final"]) {
    assert.match(flat(l), /Changed sentences get both reviews, scoped to those sentences/, l);
    assert.match(flat(l), /READER-FINDING had to infer the seam/, l);
  }
});

check("dry run: a re-test that returns fix escalates the page, with exactly two reader calls and one redraft", async () => {
  const { labels, record } = await seatDryRun("docs/extend/architecture.md", { reader: async () => READER_FIX });
  assert.equal(record.status, "escalate");
  assert.deepEqual(labels.filter((l) => l.startsWith("reader:")), ["reader:architecture", "reader:architecture:retest"]);
  assert.equal(labels.filter((l) => l.startsWith("reader-redraft:")).length, 1);
  assert.equal(labels[labels.length - 1], "reader:architecture:retest");
  assert.equal(record.finalRead.retest.verdict, "fix");
  assert.deepEqual(record.finalRead.retest.findings, READER_FIX.findings);
});

check("dry run: a red gate on the final read's redraft escalates at once, with no scoped reads and no re-test", async () => {
  const { labels, record } = await seatDryRun("docs/extend/architecture.md", {
    reader: async () => READER_FIX,
    "reader-redraft": async (prompt) => ({ path: prompt.match(/Redraft (\S+)/)[1], gate: "fail", gateCommand: "stub", gateTail: "GATE-TAIL vale error" })
  });
  assert.equal(record.status, "escalate");
  assert.deepEqual(labels.filter((l) => l.startsWith("reader:")), ["reader:architecture"]);
  assert.ok(!labels.includes("reader:architecture:retest"));
  assert.ok(!labels.includes("editor:architecture:final"));
  assert.ok(!labels.includes("facts:architecture:final"));
  assert.match(record.findings, /## gate: fail\nGATE-TAIL vale error/);
});

check("dry run: a scoped read's fix after the final read's redraft escalates, with no re-test", async () => {
  const { labels, record } = await seatDryRun("docs/extend/architecture.md", {
    reader: async () => READER_FIX,
    editor: async (p, o) => (o.label.endsWith(":final") ? FIX : ACCEPT)
  });
  assert.equal(record.status, "escalate");
  assert.equal(labels.filter((l) => l.startsWith("reader:")).length, 1);
  assert.equal(labels.filter((l) => l.startsWith("reader-redraft:")).length, 1);
});

check("dry run: the drafter prompt carries the anatomy pointer and the no-claim instruction", async () => {
  const { flat } = await seatDryRun("docs/extend/architecture.md");
  const d = flat("draft:architecture");
  assert.match(d, /the introduction, the section hand-off lead-ins, and the ending the register's page anatomies require/);
  assert.match(d, /"no-claim" when it carries no extractable fact/);
  assert.ok(d.includes("scripts/checks/check-provenance.mjs"));
  assert.ok(d.includes("a no-claim sentence cites nothing, so any extractable fact in it fails"));
});

// -------------------------------------------------------------------------------------------
// Dry runs of the rework entry point: a committed page reworked at the page level.
// -------------------------------------------------------------------------------------------

const REWORK = "REWORK-TEXT the page opens on a meta sentence; add an introduction.";
const REWORK_SCOPE = "page-level only (introduction, section order, hand-offs, depth, ending, covers); the page plan governs order, placement, and cuts, and a sentence is kept where the plan keeps it; follow the register's page anatomies.";

const BRIEF_PATH = "docs/internal/briefs/extend/architecture.json";

/** Runs git in a fixture worktree with a fixed identity. */
function git(wt, args) {
  return execSync(`git -c user.name=t -c user.email=t@example.com ${args}`, { cwd: wt, encoding: "utf8" }).trim();
}

/**
 * A worktree that is a git repo holding the architecture page and its brief, the brief citing
 * `cited`. `state` leaves the page committed and unchanged ("clean"), never added ("untracked"),
 * or committed then edited ("modified").
 */
function reworkWorktree({ outline = OUTLINE, cited = ["f:a7qx4m", "f:0duu5p"], state = "clean" } = {}) {
  const wt = makeWorktree(outline);
  write(wt, BRIEF_PATH, JSON.stringify({ page: "docs/extend/architecture.md", sentences: cited.map((id) => ({ text: `s ${id}`, id })).concat([{ text: "intro", id: "no-claim" }]) }));
  git(wt, "init -q");
  if (state !== "untracked") write(wt, "docs/extend/architecture.md", "# Architecture\n\nCommitted body.\n");
  git(wt, "add -A");
  git(wt, "commit -q -m fixture");
  if (state === "untracked") write(wt, "docs/extend/architecture.md", "# Architecture\n\nNew.\n");
  if (state === "modified") write(wt, "docs/extend/architecture.md", "# Architecture\n\nEdited.\n");
  return wt;
}

/** One outline page carrying `rework` through the runner with stubbed agents. */
async function reworkDryRun(overrides = {}, extraArgs = {}, fixture = {}) {
  const home = makeHome();
  const wt = reworkWorktree(fixture);
  const calls = [];
  const page = { id: "architecture", path: "docs/extend/architecture.md", track: "extend", rework: REWORK };
  const out = await runRunner(baseArgs(wt, [page], { outline: "docs/internal/outlines/extend.json", ...extraArgs }), makeAgent(wt, home, calls, overrides), parallel, noop, noop);
  const labels = calls.map((c) => c.label);
  const flat = (label) => calls.find((c) => c.label === label).prompt.replace(/\s+/g, " ");
  return { record: out.pages[0], labels, flat, wt };
}

check("dry run: a rework page skips page inputs and the round-1 draft; its first draft call carries the rework text and the scope line", async () => {
  const { labels, flat } = await reworkDryRun();
  assert.ok(!labels.some((l) => l.startsWith("inputs:")), "no page-inputs call");
  assert.ok(!labels.some((l) => l.startsWith("draft:")), "no round-1 draft call");
  assert.equal(labels[1], "rework-state", "one probe reads the page's git state and brief");
  assert.deepEqual(labels.slice(2, 7), ["plan:architecture", "structure:architecture:plan", "framing:architecture", "probe:architecture:framing", "rework:architecture"], "the plan and its read, the framing step and its probe, then the rework redraft as the first drafter call");
  const d = flat("rework:architecture");
  assert.match(d, /^Redraft docs\/extend\/architecture\.md once, on the combined findings below\./);
  assert.ok(d.includes(REWORK), "the rework text");
  assert.ok(d.includes(REWORK_SCOPE), "the scope line");
});

check("helper: rework-state reports HEAD, the page's git state, and the ids its brief cites, under a checksum", () => {
  const wt = reworkWorktree({ cited: ["f:a7qx4m"] });
  const r = helper.reworkState(wt, ["docs/extend/architecture.md"], [BRIEF_PATH]);
  assert.equal(r.ok, true);
  assert.equal(r.head, git(wt, "rev-parse --short HEAD"));
  assert.deepEqual(r.pages, [{ path: "docs/extend/architecture.md", brief: BRIEF_PATH, state: "clean", cited: ["f:a7qx4m"] }]);
  assert.equal(r.checksum, helper.reworkChecksum(r));
  assert.equal(runnerMerge.reworkChecksum.toString(), helper.reworkChecksum.toString());
  assert.equal(helper.reworkState(reworkWorktree({ state: "untracked" }), ["docs/extend/architecture.md"], [BRIEF_PATH]).pages[0].state, "untracked");
  assert.equal(helper.reworkState(reworkWorktree({ state: "modified" }), ["docs/extend/architecture.md"], [BRIEF_PATH]).pages[0].state, "modified");
});

check("dry run: a rework page's inventory carries the outline ids its brief cites and cuts the rest, so the fact read has no coverage gap", async () => {
  const outline = { ...OUTLINE, pages: OUTLINE.pages.map((p) => (p.slug === "architecture" ? { ...p, factIds: ["f:aaaaaa", "f:bbbbbb", "f:cccccc"] } : p)) };
  const { flat, wt } = await reworkDryRun({}, {}, { outline, cited: ["f:aaaaaa", "f:bbbbbb"] });
  const head = git(wt, "rev-parse --short HEAD");
  const f = flat("facts:architecture:r1");
  assert.match(f, /- \[carried\] [^-]*\(f:aaaaaa\)/);
  assert.match(f, /- \[carried\] [^-]*\(f:bbbbbb\)/);
  assert.ok(f.includes(`- [cut] an outline fact for this page (f:cccccc) -- cut at the pilot draft (brief at ${head})`), "C is cut with its reason");
  assert.doesNotMatch(f, /\[carried\] [^-]*\(f:cccccc\)/);
  assert.match(f, /or appear in the claim inventory above as "cut" with a reason/, "the coverage rule C satisfies as cut");
});

/** Runs the real rework-state command the probe prompt names, then lets `edit` alter its report. */
function tamperedState(home, edit) {
  return async (prompt) => {
    const r = JSON.parse(runHelperLine(helperLine(prompt, "rework-state"), home));
    edit(r);
    return r;
  };
}

check("dry run: a rework-state report with a tampered checksum fails the run as mistranscribed", async () => {
  const home = makeHome();
  const wt = reworkWorktree();
  const page = { id: "architecture", path: "docs/extend/architecture.md", track: "extend", rework: REWORK };
  const agent = makeAgent(wt, home, [], { "rework-state": tamperedState(home, (r) => { r.head = "0000000"; }) });
  await assert.rejects(runRunner(baseArgs(wt, [page], { outline: "docs/internal/outlines/extend.json" }), agent, parallel, noop, noop), /mistranscribed its report/);
});

check("dry run: a rework-state report with ok false and a valid checksum fails the run", async () => {
  const home = makeHome();
  const wt = reworkWorktree();
  const page = { id: "architecture", path: "docs/extend/architecture.md", track: "extend", rework: REWORK };
  const agent = makeAgent(wt, home, [], {
    "rework-state": tamperedState(home, (r) => {
      r.ok = false;
      r.error = "no HEAD";
      r.checksum = helper.reworkChecksum(r);
    })
  });
  await assert.rejects(runRunner(baseArgs(wt, [page], { outline: "docs/internal/outlines/extend.json" }), agent, parallel, noop, noop), /rework-state probe failed: no HEAD/);
});

check("dry run: a clean rework page whose state carries an error escalates with that reason and makes no rework call", async () => {
  const home = makeHome();
  const wt = reworkWorktree();
  const page = { id: "architecture", path: "docs/extend/architecture.md", track: "extend", rework: REWORK };
  const calls = [];
  const agent = makeAgent(wt, home, calls, {
    "rework-state": tamperedState(home, (r) => {
      r.pages[0].error = "brief not readable: BRIEF-ERROR";
      r.checksum = helper.reworkChecksum(r);
    })
  });
  const out = await runRunner(baseArgs(wt, [page], { outline: "docs/internal/outlines/extend.json" }), agent, parallel, noop, noop);
  assert.equal(out.pages[0].status, "escalate");
  assert.equal(out.pages[0].reason, "brief not readable: BRIEF-ERROR");
  assert.equal(out.pages[0].rework, true);
  assert.ok(!calls.some((c) => c.label.startsWith("rework:")), "no rework call");
  assert.equal(out.pages[0].rounds.length, 0);
});

check("dry run: a rework page that is untracked or has uncommitted changes is not run; its record escalates with the reason", async () => {
  for (const state of ["untracked", "modified"]) {
    const { labels, record } = await reworkDryRun({}, {}, { state });
    assert.equal(record.status, "escalate", state);
    assert.match(record.reason, new RegExp(`docs/extend/architecture\\.md is ${state}`), state);
    assert.deepEqual(labels, ["outline", "rework-state"], `${state}: no page agent runs`);
  }
});

check("dry run: a rework page's fact read carries the outline factIds as the carried inventory", async () => {
  const { flat } = await reworkDryRun();
  const f = flat("facts:architecture:r1");
  assert.match(f, /- \[carried\] [^-]*\(f:a7qx4m\)/);
  assert.match(f, /- \[carried\] [^-]*\(f:0duu5p\)/);
  assert.doesNotMatch(f, /\(none recorded\)/);
});

check("dry run: a rework page's register and fact reads carry the scope note; the structural seat does not", async () => {
  const { flat } = await reworkDryRun();
  for (const l of ["editor:architecture:r1", "facts:architecture:r1"]) {
    assert.match(flat(l), /Changed sentences get both reviews, scoped to those sentences/, l);
    assert.ok(flat(l).includes("git diff -- docs/extend/architecture.md"), `${l} names the changed sentences by the diff`);
    assert.match(flat(l), /If it prints nothing, the rework changed nothing: return "fix" with that finding\./, `${l} fails an empty rework`);
  }
  assert.doesNotMatch(flat("structure:architecture:r1"), /Changed sentences get both reviews/);
  assert.doesNotMatch(flat("structure:architecture:r1"), /git diff/);
});

check("dry run: a rework page's fix takes the round-2 redraft, still page-level, with scoped reads", async () => {
  const { labels, flat, record } = await reworkDryRun({ editor: async (p, o) => (o.label.endsWith(":r1") ? FIX : ACCEPT) });
  assert.ok(labels.includes("redraft:architecture"));
  assert.ok(flat("redraft:architecture").includes(REWORK_SCOPE));
  assert.match(flat("editor:architecture:r2"), /Changed sentences get both reviews/);
  assert.equal(record.status, "accepted");
});

check("dry run: a rework page reaches the final read on acceptance", async () => {
  const { labels, record } = await reworkDryRun();
  assert.equal(labels[labels.length - 1], "reader:architecture");
  assert.equal(record.status, "accepted");
  assert.equal(record.rework, true);
  assert.equal(record.finalRead.verdict, "accept");
});

check("dry run: a page without rework produces the same call labels as before, with no scope note", async () => {
  const { labels, flat } = await seatDryRun("docs/extend/architecture.md");
  assert.deepEqual(labels, ["outline", "inputs:architecture", "plan:architecture", "structure:architecture:plan", "framing:architecture", "probe:architecture:framing", "draft:architecture", "structure:architecture:r1", "editor:architecture:r1", "facts:architecture:r1", "figure:architecture:r1", "reader:architecture"]);
  assert.doesNotMatch(flat("editor:architecture:r1"), /This read is scoped/);
  assert.doesNotMatch(flat("facts:architecture:r1"), /This read is scoped/);
});

// -------------------------------------------------------------------------------------------
// Dry runs of the page plan: Google's outline, written down, read before any prose.
// -------------------------------------------------------------------------------------------

const PLAN_PATH = "docs/internal/briefs/extend/architecture.plan.md";
const GOOGLE_LARGE_DOCS = "https://developers.google.com/tech-writing/two/large-docs";
const SUBORDINATED = "subordinated: `docs/reference/sveltekit.md`, section Hooks, states it";

/** A plan step that places f:a7qx4m under "Seams" and subordinates f:0duu5p. */
async function placingPlan() {
  return {
    plan: PLAN_PATH,
    claimInventory: [
      { claim: "the seam", disposition: "carried", factId: "f:a7qx4m", section: "Seams" },
      { claim: "the hook", disposition: "cut", factId: "f:0duu5p", reason: SUBORDINATED }
    ],
    couldNotDo: ["PLAN-COULD-NOT-DO docs/reference/sveltekit.md does not state f:zzz"],
    frictionFiled: ["plan-friction"]
  };
}

check("dry run: the plan step runs between page inputs and the draft, on Opus 5.5 at high by default", async () => {
  const { labels, opts, flat } = await seatDryRun("docs/extend/architecture.md");
  const at = labels.indexOf("plan:architecture");
  assert.ok(at > labels.indexOf("inputs:architecture"), "after page inputs");
  assert.ok(at < labels.indexOf("draft:architecture"), "before the draft");
  assert.equal(opts("plan:architecture").model, "claude-opus-5-5");
  assert.equal(opts("plan:architecture").effort, "high");
  const p = flat("plan:architecture");
  assert.ok(p.includes(PLAN_PATH), "the plan path");
  assert.ok(p.includes("The page's job, from the stage outline: the job"), "the job");
  assert.ok(p.includes("Page type: concept"), "the page type");
  assert.ok(p.includes("## The page anatomies"), "the anatomy, by heading");
  assert.ok(p.includes("Take: Take the code map."), "the exemplar takes");
  assert.ok(p.includes("Fact ids the plan disposes, every one: f:a7qx4m, f:0duu5p"), "every fact id");
  assert.ok(p.includes("Claim inventory"), "the claim inventory");
  assert.ok(p.includes(GOOGLE_LARGE_DOCS), "Google's lesson, cited");
  assert.ok(p.includes("think of an outline as the narrative for your document"));
  for (const part of ["What the document covers.", "What prior knowledge you expect readers to have.", "What the document doesn't cover."]) {
    assert.ok(p.includes(part), `the introduction's part: ${part}`);
  }
  assert.match(p, /its heading, the one sentence a reader takes from it, the fact ids it draws on, and its hand-off/);
  assert.match(p, /the ending section the anatomy requires/i);
  assert.match(p, /placed in a section; subordinated, a link to the reference page or entry that states it, named; or cut with a reason/);
  assert.match(p, /a couldNotDo naming the reference page/);
  assert.match(p, /never kept on the page/);
});

check("dry run: planModel and planEffort set the plan step's seat", async () => {
  const { opts } = await seatDryRun("docs/extend/architecture.md", {}, { planModel: "fable", planEffort: "max" });
  assert.equal(opts("plan:architecture").model, "fable");
  assert.equal(opts("plan:architecture").effort, "max");
});

check("dry run: the structural seat reads the plan before any prose, at the plan level", async () => {
  const { labels, flat } = await seatDryRun("docs/extend/architecture.md");
  assert.ok(labels.indexOf("structure:architecture:plan") > labels.indexOf("plan:architecture"));
  assert.ok(labels.indexOf("structure:architecture:plan") < labels.indexOf("draft:architecture"));
  const s = flat("structure:architecture:plan");
  assert.ok(s.includes(PLAN_PATH), "the plan path");
  assert.match(s, /order, pace, user goal, and the introduction's three parts/);
  assert.match(s, /Learn where cairn ends and your site begins\./, "the outline entry");
  for (const item of ["Information is provided at the right pace.", "Information is presented in the most logical order and location.",
    "The user goal is clear.", "What the document covers.", "What prior knowledge you expect readers to have.", "What the document doesn't cover."]) {
    assert.ok(s.includes(item), `checklist item: ${item}`);
  }
  assert.ok(s.includes("in the order the seat judges"), "the plan read judges the order");
});

check("dry run: a plan read fix drives one plan revision and one re-read, then the draft", async () => {
  const { labels, flat, record } = await seatDryRun("docs/extend/architecture.md", { structure: async (p, o) => (o.label.endsWith(":plan") ? STRUCT_FIX : ACCEPT) });
  const at = labels.indexOf("plan:architecture");
  assert.deepEqual(labels.slice(at, at + 7), ["plan:architecture", "structure:architecture:plan", "replan:architecture", "structure:architecture:plan2", "framing:architecture", "probe:architecture:framing", "draft:architecture"]);
  const r = flat("replan:architecture");
  assert.match(r, /^Revise the page plan/);
  assert.ok(r.includes("STRUCT-FINDING intro omits scope"), "the plan read's findings");
  assert.ok(r.includes(PLAN_PATH));
  assert.deepEqual(record.planStep.reads.map((x) => x.verdict), ["fix", "accept"]);
  assert.equal(record.planStep.revised, true);
  assert.equal(record.status, "accepted");
});

check("dry run: a second plan read fix escalates the page before any prose", async () => {
  const { labels, record } = await seatDryRun("docs/extend/architecture.md", { structure: async (p, o) => (/:plan2?$/.test(o.label) ? STRUCT_FIX : ACCEPT) });
  assert.equal(record.status, "escalate");
  assert.match(record.reason, /plan read/);
  assert.match(record.findings, /STRUCT-FINDING/);
  assert.equal(labels[labels.length - 1], "structure:architecture:plan2");
  assert.equal(labels.filter((l) => l.startsWith("replan:")).length, 1);
  assert.ok(!labels.some((l) => l.startsWith("draft:")));
});

check("dry run: a plan that leaves an inventory fact id undisposed escalates before the plan read", async () => {
  const { labels, record } = await seatDryRun("docs/extend/architecture.md", {
    plan: async () => ({ plan: PLAN_PATH, claimInventory: [{ claim: "c", disposition: "carried", factId: "f:a7qx4m" }] })
  });
  assert.equal(record.status, "escalate");
  assert.match(record.reason, /f:a7qx4m, f:0duu5p/, "a carried id with no section is undisposed too");
  assert.ok(!labels.some((l) => l.startsWith("structure:") || l.startsWith("draft:")));
});

check("dry run: the drafter drafts from the plan, with its dispositions in the inventory and the brief contract", async () => {
  const { flat } = await seatDryRun("docs/extend/architecture.md", { plan: placingPlan });
  const d = flat("draft:architecture");
  assert.ok(d.includes(PLAN_PATH), "the plan path");
  assert.match(d, /source of the page's order, each section's claim, and each fact's placement/);
  assert.match(d, /The register's drafting brief is the source of voice/);
  assert.ok(d.includes('- [carried] the seam (f:a7qx4m) in section "Seams"'), "a placed fact");
  assert.ok(d.includes(`- [cut] the hook (f:0duu5p) -- ${SUBORDINATED}`), "a subordinated fact");
  assert.match(d, /an array of the fact ids it synthesizes/);
  assert.match(d, /top-level "cuts" array/);
});

check("dry run: a fact id the plan places beyond the inventory reaches the drafter's fact ids", async () => {
  const { flat, prompt } = await seatDryRun("docs/extend/architecture.md", {
    plan: async (prompt) => {
      const r = stubPlan(makeWorktree(), prompt);
      r.claimInventory.push({ claim: "an added fact", disposition: "carried", factId: "f:extra1", section: "Seams" });
      return r;
    }
  });
  const ids = prompt("draft:architecture").match(/Fact ids to draw on: ([^\n]*)/)[1].split(", ");
  assert.ok(ids.includes("f:extra1"), `the drafter's fact ids: ${ids.join(", ")}`);
  assert.ok(flat("draft:architecture").includes('- [carried] an added fact (f:extra1) in section "Seams"'));
});

check("dry run: the fact read's inventory carries the plan's dispositions, and its coverage rule reads them", async () => {
  const { flat } = await seatDryRun("docs/extend/architecture.md", { plan: placingPlan });
  const f = flat("facts:architecture:r1");
  assert.ok(f.includes('- [carried] the seam (f:a7qx4m) in section "Seams"'));
  assert.ok(f.includes(`- [cut] the hook (f:0duu5p) -- ${SUBORDINATED}`));
  assert.ok(f.includes(PLAN_PATH), "the plan path");
  assert.match(f, /An outline id the plan subordinates or cuts with a reason is disposed, not dropped/);
  assert.match(f, /an id the plan places in a section that the page omits is a blocking finding/);
});

check("dry run: the page-level structural read grades the page against its plan, not the outline's cover order", async () => {
  const { flat } = await seatDryRun("docs/extend/architecture.md");
  const s = flat("structure:architecture:r1");
  assert.ok(s.includes(PLAN_PATH), "the plan path");
  assert.match(s, /grade the page against its plan and this checklist/);
  assert.ok(!s.includes("in the order the seat judges"), "no cover order");
});

check("dry run: the record carries the plan path beside the page and brief, and the plan's report", async () => {
  const { record } = await seatDryRun("docs/extend/architecture.md", { plan: placingPlan });
  assert.equal(record.path, "docs/extend/architecture.md");
  assert.equal(record.brief, "docs/internal/briefs/extend/architecture.json");
  assert.equal(record.plan, PLAN_PATH);
  assert.deepEqual(record.planStep.couldNotDo, ["PLAN-COULD-NOT-DO docs/reference/sveltekit.md does not state f:zzz"]);
  assert.deepEqual(record.planStep.frictionFiled, ["plan-friction"]);
  assert.equal(record.planStep.revised, false);
});

check("dry run: a rework page runs the plan step, with its rework text, before its rework draft", async () => {
  const { labels, flat } = await reworkDryRun();
  assert.ok(labels.indexOf("plan:architecture") < labels.indexOf("rework:architecture"));
  assert.ok(flat("plan:architecture").includes(REWORK), "the rework text");
  assert.ok(flat("rework:architecture").includes(PLAN_PATH), "the rework draft drafts from the plan");
});

check("helper: rework-state counts every id a multi-id brief sentence cites", () => {
  const wt = reworkWorktree({ cited: [] });
  write(wt, BRIEF_PATH, JSON.stringify({ page: "docs/extend/architecture.md", sentences: [{ text: "s", id: ["f:aaaaaa", "f:bbbbbb"] }, { text: "t", id: "f:cccccc" }], cuts: [] }));
  git(wt, "add -A");
  git(wt, "commit -q -m brief");
  const r = helper.reworkState(wt, ["docs/extend/architecture.md"], [BRIEF_PATH]);
  assert.deepEqual(r.pages[0].cited, ["f:aaaaaa", "f:bbbbbb", "f:cccccc"]);
});

// -------------------------------------------------------------------------------------------
// The framing step and the narrowed round-2 reads.
// -------------------------------------------------------------------------------------------

const FRAMING_PATH = "docs/internal/briefs/extend/architecture.framing.md";

check("dry run: the framing step runs after the plan read and before the draft, on claude-opus-5-5 at xhigh, from the doc set's map", async () => {
  const { labels, opts, flat, record } = await seatDryRun("docs/extend/architecture.md");
  const at = labels.indexOf("framing:architecture");
  assert.ok(at > labels.indexOf("structure:architecture:plan"), "after the plan read");
  assert.ok(at < labels.indexOf("draft:architecture"), "before the draft");
  assert.equal(opts("framing:architecture").model, "claude-opus-5-5");
  assert.equal(opts("framing:architecture").effort, "xhigh");
  const f = flat("framing:architecture");
  assert.ok(f.includes(FRAMING_PATH), "the record path");
  assert.ok(f.includes("docs/internal/outlines/extend.json whole"), "the outline as the doc set's map");
  assert.ok(f.includes("### The introduction"), "the register's introduction section");
  assert.ok(f.includes("docs/internal/briefs/extend/architecture.plan.md"), "the page plan");
  assert.ok(f.includes("never on an imperative"));
  assert.ok(f.includes("## Who arrives, from where, and why"));
  assert.equal(record.framing, FRAMING_PATH);
  assert.deepEqual(record.framingStep.readers, ["a reader"]);
});

check("dry run: the drafter writes the intro from the framing record, and the structural and register reads grade it against the record", async () => {
  const { flat } = await seatDryRun("docs/extend/architecture.md");
  assert.ok(flat("draft:architecture").includes(`The framing record at ${FRAMING_PATH} is the source of the introduction`));
  for (const l of ["structure:architecture:r1", "editor:architecture:r1"]) assert.ok(flat(l).includes(`The framing record ${FRAMING_PATH} decides the introduction`), l);
  assert.ok(!flat("structure:architecture:plan").includes(FRAMING_PATH), "the plan read precedes the record");
});

check("dry run: the framing record's fact ids join the drafter's ids and the inventory in the introduction, a plan cut re-placed", async () => {
  const { flat } = await seatDryRun("docs/extend/architecture.md", {
    plan: async (prompt) => ({ plan: "docs/internal/briefs/extend/architecture.plan.md", claimInventory: [{ claim: "c", disposition: "carried", factId: "f:a7qx4m", section: "S" }, { claim: "d", disposition: "cut", factId: "f:0duu5p", reason: "subordinated" }] }),
    framing: async () => ({ framing: FRAMING_PATH, readers: ["r"], factIds: ["f:0duu5p", "f:newbg1", "f:a7qx4m"] })
  });
  const d = flat("draft:architecture");
  assert.ok(/Fact ids to draw on: [^\n]*f:newbg1/.test(d), "a filed background id joins the drafter's ids");
  assert.ok(d.includes('(f:0duu5p) in section "Introduction"'), "the cut id is re-placed in the introduction");
  assert.ok(d.includes('(f:newbg1) in section "Introduction"'), "a new id is carried in the introduction");
  assert.ok(d.includes('(f:a7qx4m) in section "S"'), "a carried id keeps its section");
  assert.ok(flat("facts:architecture:r1").includes('(f:0duu5p) in section "Introduction"'), "the fact read holds the intro to it");
});

check("dry run: a framing step that returns nothing escalates the page before the draft", async () => {
  const { labels, record } = await seatDryRun("docs/extend/architecture.md", { framing: async () => null });
  assert.equal(record.status, "escalate");
  assert.match(record.reason, /framing step/);
  assert.ok(!labels.some((l) => l.startsWith("draft:")));
});

const BASELINE = "0123456789abcdef0123456789abcdef01234567";
const withBaseline = (base) => async (prompt) => {
  const r = await base(prompt);
  return { ...r, baseline: BASELINE };
};

check("dry run: round 2 is narrowed to the seat's round-1 findings and the diff since the redrafter's baseline", async () => {
  const home = makeHome();
  const wt = makeWorktree();
  const inner = makeAgent(wt, home, []);
  const EFIX = { verdict: "fix", findings: [{ location: "docs/extend/architecture.md:4", finding: "EDITOR-R1-FINDING", blocking: true }], summary: "fix" };
  const { labels, flat, record } = await seatDryRun("docs/extend/architecture.md", {
    editor: async (p, o) => (o.label.endsWith(":r1") ? EFIX : ACCEPT),
    redraft: withBaseline((prompt) => inner(prompt, { label: "redraft:architecture" }))
  });
  assert.ok(flat("redraft:architecture").includes("git hash-object -w -- docs/extend/architecture.md"), "the redrafter records the baseline first");
  assert.ok(!flat("draft:architecture").includes("git hash-object"), "the round-1 draft takes no baseline");
  const e = flat("editor:architecture:r2");
  assert.ok(e.includes(`git cat-file blob ${BASELINE} | diff -u --label round-1 --label round-2 - docs/extend/architecture.md`));
  assert.ok(e.includes("EDITOR-R1-FINDING"), "the seat's own round-1 finding");
  assert.ok(e.includes("Do not re-read the whole page"));
  assert.ok(!labels.includes("structure:architecture:r2"), "lean mode re-reads only the fix seat");
  assert.equal(record.rounds[1].scope, `changed sentences since ${BASELINE}`);
  assert.equal(record.status, "accepted");
});

check("dry run: under bothReviewers a round-1 accepter reads the changed sentences for a fix's regression, and the flag still measures", async () => {
  const home = makeHome();
  const wt = makeWorktree();
  const inner = makeAgent(wt, home, []);
  const { flat, record } = await seatDryRun("docs/extend/debug-your-site.md", {
    editor: async (p, o) => (o.label.endsWith(":r1") ? FIX : ACCEPT),
    structure: async (p, o) => (o.label.endsWith(":r2") ? FIX : ACCEPT),
    redraft: withBaseline((prompt) => inner(prompt, { label: "redraft:debug-your-site" }))
  }, { bothReviewers: true });
  const s = flat("structure:debug-your-site:r2");
  assert.ok(s.includes("You accepted in round 1. Read only the changed sentences, for a defect the fixes introduced"));
  assert.ok(s.includes(`git cat-file blob ${BASELINE}`));
  assert.equal(record.crossRegression, true);
});

check("dry run: a redraft with no baseline falls back to whole-page round-2 reads", async () => {
  const { flat, record } = await seatDryRun("docs/extend/architecture.md", { editor: async (p, o) => (o.label.endsWith(":r1") ? FIX : ACCEPT) });
  assert.ok(!flat("editor:architecture:r2").includes("git cat-file blob"));
  assert.equal(record.rounds[1].scope, "whole page (no baseline from the redraft)");
});

check("dry run: a framing record the probe finds missing on disk escalates the page before the draft", async () => {
  const { labels, flat, record } = await seatDryRun("docs/extend/architecture.md", { probe: async (p, o) => ({ exists: !o.label.endsWith(":framing") }) });
  assert.ok(flat("probe:architecture:framing").includes(`test -f '${FRAMING_PATH}'`));
  assert.equal(record.status, "escalate");
  assert.match(record.reason, /not on disk/);
  assert.ok(!labels.some((l) => l.startsWith("draft:")));
});

check("dry run: a well-shaped baseline that does not resolve takes the whole-page fallback; a misshapen one is never probed", async () => {
  const home = makeHome();
  const wt = makeWorktree();
  const inner = makeAgent(wt, home, []);
  const { flat, labels, record } = await seatDryRun("docs/extend/architecture.md", {
    editor: async (p, o) => (o.label.endsWith(":r1") ? FIX : ACCEPT),
    probe: async (p, o) => ({ exists: !o.label.endsWith(":baseline") }),
    redraft: withBaseline((prompt) => inner(prompt, { label: "redraft:architecture" }))
  });
  assert.ok(flat("probe:architecture:baseline").includes(`git cat-file -e ${BASELINE}`));
  assert.ok(labels.indexOf("probe:architecture:baseline") < labels.indexOf("editor:architecture:r2"));
  assert.ok(!flat("editor:architecture:r2").includes("git cat-file blob"));
  assert.equal(record.rounds[1].scope, "whole page (no baseline from the redraft)");
  const odd = await seatDryRun("docs/extend/architecture.md", {
    editor: async (p, o) => (o.label.endsWith(":r1") ? FIX : ACCEPT),
    redraft: async (prompt) => ({ ...(await inner(prompt, { label: "redraft:architecture" })), baseline: `${BASELINE}0` })
  });
  assert.ok(!odd.labels.includes("probe:architecture:baseline"), "a 41-digit baseline is rejected on shape");
  assert.equal(odd.record.rounds[1].scope, "whole page (no baseline from the redraft)");
});

// -------------------------------------------------------------------------------------------

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

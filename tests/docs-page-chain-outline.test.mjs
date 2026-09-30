// Tests docs-page-chain.js's outline support and its cairn-docs-outline helper: the helper's pure
// functions (outline lookup, exemplar mapping, index link insertion), the runner's extracted
// merge block, and dry runs of the whole runner body with agent, parallel, log, and phase stubbed.
// The stubbed probe and drafter run the exact helper commands the runner renders into their
// prompts, against a temp worktree, so a dry run proves the prompts carry a runnable command.
// Run with: node tests/docs-page-chain-outline.test.mjs
// Exits 0 with "ALL PASS" on success; prints failures and exits 1 otherwise.
import { readFileSync, writeFileSync, mkdirSync, mkdtempSync, rmSync } from "node:fs";
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
      slug: "architecture", path: "docs/extend/architecture.md", group: "start", order: 1,
      job: "Learn where cairn ends and your site begins.", pageType: "concept",
      exemplars: [{ source: "core/rust-analyzer-architecture/", take: "t" }, { source: "evaluators/litestream/" }],
      figure: true, factIds: ["f:a7qx4m", "f:0duu5p"], covers: ["The export map."],
      outOfScope: ["Security properties (security-model)."], pinned: []
    },
    {
      slug: "add-cairn", path: "docs/extend/add-cairn.md", group: "start", order: 2,
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
const runRunner = new AsyncFunction("args", "agent", "parallel", "log", "phase", runnerBody);

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

/**
 * A stubbed agent: the outline probe runs its rendered command and returns the JSON verbatim;
 * page inputs echo the page's outline job; the drafter writes the page and runs its rendered
 * link command; every read accepts.
 */
function makeAgent(wt, home, calls, overrides = {}) {
  return async (prompt, opts) => {
    calls.push({ label: opts.label, prompt });
    const kind = opts.label.split(":")[0];
    if (overrides[kind]) return overrides[kind](prompt, opts);
    if (kind === "outline") return JSON.parse(runHelperLine(helperLine(prompt, "resolve"), home));
    if (kind === "inputs") return { job: "the job", pageType: "concept", factIds: ["f:a7qx4m"], claimInventory: [] };
    if (kind === "draft" || kind === "redraft") {
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
  runnerMerge = new Function(`${RUNNER_SRC.slice(startAt, endAt)}\nreturn { canonicalEntry, checksumOf, mergeOutline };`)();
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

// -------------------------------------------------------------------------------------------
// The runner's merge.
// -------------------------------------------------------------------------------------------

function probeFor(paths) {
  return helper.resolveEntries(OUTLINE, paths, { exemplarRoot: "/ex", exists: () => true });
}

check("an inline field overrides the outline's; an absent one comes from the outline", () => {
  const [p] = runnerMerge.mergeOutline(
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
  assert.deepEqual(calls.map((c) => c.label), ["outline", "inputs:architecture", "draft:architecture", "editor:architecture:r1", "facts:architecture:r1", "figure:architecture:r1"]);

  const inputs = calls.find((c) => c.label.startsWith("inputs:")).prompt;
  assert.match(inputs, /Learn where cairn ends and your site begins\./);
  assert.match(inputs, /The outline's page type: concept/);
  assert.match(inputs, /traced to this page's sources: f:a7qx4m, f:0duu5p/);
  assert.match(inputs, /- The export map\./);
  assert.match(inputs, /- Security properties \(security-model\)\./);

  const draft = calls.find((c) => c.label.startsWith("draft:")).prompt;
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

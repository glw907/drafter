// Tests docs-page-chain.js's blueprint mode, the mode drafter loads it in for a blueprint page (no
// chain args), against tests/fixtures/drafter/blueprint-matrix.json; that handoff mode still
// renders byte for byte as B3c.0's golden captured it; and the blueprint seat's bar paragraph.
// Node and goja parity runs through the built binary: DRAFTER_BIN names it, or the test builds one.
// Run with: node tests/docs-page-chain-blueprint.test.mjs
// Prints "PASS <name>" per check and exits 0 with "ALL PASS"; prints failures and exits 1 otherwise.
import { readFileSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { execFileSync, spawnSync } from "node:child_process";
import assert from "node:assert/strict";

const HERE = dirname(fileURLToPath(import.meta.url));
const ROOT = join(HERE, "..");
const MODULE = join(ROOT, "claude", ".claude", "workflows", "docs-page-chain.js");
const FIXTURES = join(HERE, "fixtures", "drafter");
const HANDOFF_MATRIX = join(FIXTURES, "matrix.json");
const BLUEPRINT_MATRIX = join(FIXTURES, "blueprint-matrix.json");
const BASELINE_MATRIX = join(FIXTURES, "blueprint-seat-baseline.matrix.json");

const readJSON = (path) => JSON.parse(readFileSync(path, "utf8"));
const clone = (v) => JSON.parse(JSON.stringify(v));

const failures = [];
const pending = [];
function check(name, fn) {
  pending.push({ name, fn });
}

const scratch = [];

// The chain loads as drafter's loader loads it: `export` dropped from the meta line, the script
// run as an async function body, and these the only globals. Each context crosses as JSON.
const AsyncFunction = (async () => {}).constructor;
const BODY = readFileSync(MODULE, "utf8").replace(/^export const meta/m, "const meta");
const noop = () => {};

/** The seat map the chain returns when loaded with a matrix's host. */
async function seatsOf(matrix) {
  const load = new AsyncFunction("args", "DRAFTER_HOST", "phase", "log", "parallel", "pipeline", "agent", BODY);
  return load(clone(matrix.host.args), clone({ args: matrix.host.args, resolved: matrix.host.resolved }), noop, noop, noop, noop, () => {
    throw new Error("agent called at load");
  });
}

/** Every case of a matrix rendered, by case name. */
async function renderAll(matrix) {
  const seats = await seatsOf(matrix);
  const out = {};
  for (const c of matrix.cases) out[c.name] = seats[c.seat](clone(c.context));
  return out;
}

const HANDOFF = readJSON(HANDOFF_MATRIX);
const BLUEPRINT = readJSON(BLUEPRINT_MATRIX);
const BASELINE = readJSON(BASELINE_MATRIX);
const RENDERS = await renderAll(BLUEPRINT);
const caseOf = (name) => BLUEPRINT.cases.find((c) => c.name === name);
const casesFor = (seat) => BLUEPRINT.cases.filter((c) => c.seat === seat);
// Prompt prose wraps at column 100, so a phrase is matched with its whitespace collapsed.
const flat = (s) => s.replace(/\s+/g, " ");

let drafterBin = process.env.DRAFTER_BIN || "";
function bin() {
  if (!drafterBin) {
    const d = mkdtempSync(join(tmpdir(), "dpc-blueprint-"));
    scratch.push(d);
    drafterBin = join(d, "drafter");
    execFileSync("go", ["build", "-o", drafterBin, "./cmd/drafter"], { cwd: ROOT, stdio: "inherit" });
  }
  return drafterBin;
}

/** Runs drafter check-prompts on a matrix, with Node's renders as the expect file when given. */
function checkPrompts(matrixPath, expect) {
  const argv = ["check-prompts", "--module", MODULE, "--matrix", matrixPath];
  if (expect) argv.push("--expect", "-");
  const r = spawnSync(bin(), argv, { cwd: ROOT, input: expect ? JSON.stringify(expect) : "", encoding: "utf8" });
  return { status: r.status, output: `${r.stdout}${r.stderr}` };
}

// -------------------------------------------------------------------------------------------
// Parity and the handoff-mode regression.
// -------------------------------------------------------------------------------------------

check("parity: Node and goja render the handoff matrix byte for byte", async () => {
  const r = checkPrompts(HANDOFF_MATRIX, await renderAll(HANDOFF));
  assert.equal(r.status, 0, r.output);
});

check("parity: Node and goja render the blueprint matrix byte for byte", async () => {
  const r = checkPrompts(BLUEPRINT_MATRIX, RENDERS);
  assert.equal(r.status, 0, r.output);
});

check("check-prompts: the blueprint matrix exits 0, containment included", () => {
  const r = checkPrompts(BLUEPRINT_MATRIX, null);
  assert.equal(r.status, 0, r.output);
  assert.match(r.output, /24 cases, 0 failed/);
});

check("handoff mode: every case equals the B3c.0 golden", async () => {
  const golden = readJSON(join(FIXTURES, "handoff-renders.golden.json"));
  const got = await renderAll(HANDOFF);
  assert.deepEqual(Object.keys(got).sort(), Object.keys(golden).sort());
  for (const name of Object.keys(golden)) assert.equal(got[name], golden[name], `case ${name}`);
});

// -------------------------------------------------------------------------------------------
// Blueprint mode: what no render may name.
// -------------------------------------------------------------------------------------------

for (const banned of [".plan.yaml", ".plan.md", ".framing.md", "undefined", "worktree", "cairn-run-gate", "check:docs-gate"]) {
  check(`blueprint mode: no render contains "${banned}"`, () => {
    const hits = Object.entries(RENDERS).filter(([, text]) => text.includes(banned)).map(([name]) => name);
    assert.deepEqual(hits, []);
  });
}

// -------------------------------------------------------------------------------------------
// Blueprint mode: the draft.
// -------------------------------------------------------------------------------------------

const TRACK_HEADINGS = {
  extend: "### The extend track (`docs/extend/`)",
  admin: "### The admin track (`docs/admin/`)",
  editors: "### The editor track (`docs/editors/`)",
  reference: "## The reference (`docs/reference/`), a shared instrument"
};
const SHARED_SECTIONS = ["## Drafting brief: developer docs", "## Names", "## Visuals (every page that carries one)", "## The page anatomies"];

check("draft: every render names the brief at page.brief and its page, sentences, cuts, and local fields", () => {
  for (const c of casesFor("draft")) {
    const text = RENDERS[c.name];
    assert.ok(text.includes(c.context.page.brief), `${c.name}: names ${c.context.page.brief}`);
    assert.ok(text.includes(`- "page": the page's path, exactly "${c.context.page.path}".`), `${c.name}: the page field`);
    assert.ok(text.includes(`- "sentences": one { "text", "id" } entry per sentence`), `${c.name}: the sentences field`);
    assert.ok(text.includes(`- "cuts", optional:`), `${c.name}: the cuts field`);
    assert.ok(text.includes(`- "local", optional:`), `${c.name}: the local field`);
    assert.ok(flat(text).includes(`A sentence's "id" is one fact id, an array of the fact ids it synthesizes when it states two or more together, or "no-claim".`), `${c.name}: the id rules`);
  }
});

check("draft: every render names the register sections of its page's track", () => {
  const trackOf = { "docs/extend/": "extend", "docs/admin/": "admin", "docs/claude-code/": "extend" };
  for (const c of casesFor("draft")) {
    const dir = Object.keys(trackOf).find((d) => c.context.page.path.startsWith(d));
    const want = [...SHARED_SECTIONS, TRACK_HEADINGS[trackOf[dir]]];
    for (const h of want) assert.ok(RENDERS[c.name].includes(`\n- ${h}\n`), `${c.name}: ${h}`);
    for (const [track, h] of Object.entries(TRACK_HEADINGS)) {
      if (track !== trackOf[dir]) assert.ok(!RENDERS[c.name].includes(h), `${c.name}: not ${h}`);
    }
  }
});

check("draft: the claude-code page reads the extend track's sections", () => {
  const text = RENDERS["draft/claude-code"];
  assert.ok(caseOf("draft/claude-code").context.page.path.startsWith("docs/claude-code/"));
  assert.ok(text.includes(`\n- ${TRACK_HEADINGS.extend}\n`));
});

check("draft: a page under an unknown docs directory throws, as registerSectionsFor does", async () => {
  const seats = await seatsOf(BLUEPRINT);
  const c = clone(caseOf("draft/whole").context);
  c.page.path = "docs/guide/widget-options.md";
  assert.throws(() => seats.draft(c), /unknown track "guide": no register track section/);
  assert.throws(() => seats.editor(c), /unknown track "guide"/);
});

check("draft: every render names each exemplar file by the helper's rule", () => {
  const fileOf = (s) => (s.endsWith(".md") ? s : `~/.local/share/cairn/exemplars/${s.replace(/\/+$/, "")}/page.md`);
  let named = 0;
  for (const c of casesFor("draft")) {
    for (const s of c.context.inputs.exemplarSources) {
      assert.ok(RENDERS[c.name].includes(`\n- ${fileOf(s)}\n`), `${c.name}: ${fileOf(s)}`);
      named++;
    }
  }
  assert.ok(named >= 2, "the matrix carries a capture directory and a .md source");
  assert.ok(RENDERS["draft/whole"].includes("\n- ~/.local/share/cairn/exemplars/operators/widget-create-token/page.md\n"));
  assert.ok(RENDERS["draft/whole"].includes("\n- docs/extend/how-widgets-work.md\n"));
});

check("draft: every render carries the agent-override sentences", () => {
  for (const c of casesFor("draft")) {
    const dir = c.context.page.path.split("/")[1];
    const text = flat(RENDERS[c.name]);
    assert.ok(text.includes("This prompt overrides your agent definition where the two differ"), `${c.name}: the override`);
    assert.ok(text.includes("drafter sends this prompt to the API, writes the page and the brief you return to their files, and runs the docs gate itself, so run no command and commit nothing."), `${c.name}: drafter writes and gates`);
    assert.ok(text.includes("Where it says to mirror the plan's cut dispositions in `cuts`, the blueprint's `dispositions` entries stand in for them."), `${c.name}: cuts`);
    assert.ok(text.includes(`the track is the directory under docs/ in the page's path: \`${dir}\`.`), `${c.name}: the track is ${dir}`);
  }
});

check("editor: every render says no command runs and Vale's output arrives as an input", () => {
  for (const c of casesFor("editor")) {
    const text = flat(RENDERS[c.name]);
    assert.ok(text.includes("No command runs in this read."), `${c.name}: no command runs`);
    assert.ok(text.includes("run nothing and judge the tells it names by reading the page"), `${c.name}: tellgrader`);
    assert.ok(text.includes("Vale's alerts on the page arrive as an input instead"), `${c.name}: Vale's output`);
  }
});

check("draft: the section draft changes only the sections in scope", () => {
  const text = flat(RENDERS["draft/section"]);
  assert.ok(text.includes("This is a section redraft. Change only the sections in scope: `verify`, `rotate`."));
  assert.ok(!flat(RENDERS["draft/whole"]).includes("Change only the sections in scope"));
  assert.ok(!flat(RENDERS["draft/round-2"]).includes("Change only the sections in scope"));
});

check("draft: the whole draft carries no findings and each redraft carries its findings whole", () => {
  assert.ok(!RENDERS["draft/whole"].includes("Findings:"));
  for (const name of ["draft/section", "draft/round-2", "draft/final"]) {
    assert.ok(RENDERS[name].includes(`Findings:\n${caseOf(name).context.findings}`), name);
  }
});

// -------------------------------------------------------------------------------------------
// Blueprint mode: the coherence read.
// -------------------------------------------------------------------------------------------

check("coherence: the read names the changed sections and says findings may lie anywhere", () => {
  const text = flat(RENDERS["coherence/read"]);
  assert.ok(text.includes("A section redraft changed the sections `verify`, `rotate`"));
  assert.ok(text.includes("A finding may lie anywhere on the page, in a changed section or not"));
  assert.ok(text.includes('Read the section "## Drafting brief: developer docs" of docs/internal/docs-register.md'));
  assert.ok(!text.includes("This is the retest"));
});

check("coherence: the retest says it is the retest", () => {
  const text = flat(RENDERS["coherence/retest"]);
  assert.ok(text.includes("This is the retest, after a fix that answered your earlier findings."));
  assert.ok(text.includes("A section redraft changed the sections `verify`, `rotate`"));
});

// -------------------------------------------------------------------------------------------
// The blueprint seat and its bar.
// -------------------------------------------------------------------------------------------

const BASELINE_GOLDEN = readJSON(join(FIXTURES, "blueprint-seat-renders.golden.json"));
for (const name of ["blueprint/whole", "blueprint/change"]) {
  check(`blueprint seat: empty findings equals the B3c.0 golden (${name})`, async () => {
    const got = await renderAll(BASELINE);
    assert.equal(got[name], BASELINE_GOLDEN[name]);
  });
}

check("blueprint seat: a bar read holds the bar paragraph, every finding, and the class sentence", () => {
  const text = RENDERS["blueprint/change-bar"];
  const findings = caseOf("blueprint/change-bar").context.findings;
  assert.ok(text.startsWith(RENDERS["blueprint/change"]), "the bar follows the change-read text");
  assert.ok(text.includes("Earlier findings. An earlier read of the whole blueprint blocked"), "the bar paragraph");
  assert.ok(text.includes(findings), "the findings whole");
  for (const line of findings.split("\n")) assert.ok(text.includes(line), line);
  const prose = flat(text);
  assert.ok(prose.includes("A listed finding the change didn't fix blocks whatever its class."), "the class sentence");
  assert.ok(prose.includes('its text starts "Still open:"'), "Still open");
  assert.ok(prose.includes("This overrides the change read's Nit rule"), "the Nit override");
  assert.ok(!RENDERS["blueprint/change"].includes("Earlier findings."), "no bar without findings");
  assert.ok(!RENDERS["blueprint/whole"].includes("Earlier findings."), "no bar on a whole read");
});

// -------------------------------------------------------------------------------------------
// The seat map.
// -------------------------------------------------------------------------------------------

const SEVEN = ["blueprint", "coherence", "draft", "editor", "fact", "reader", "structure"];

check("DRAFTER_SEATS: defines exactly the seven seats in each mode", async () => {
  for (const matrix of [BLUEPRINT, HANDOFF]) {
    const seats = await seatsOf(matrix);
    assert.deepEqual(Object.keys(seats).sort(), SEVEN);
    for (const f of Object.values(seats)) assert.equal(typeof f, "function");
  }
});

for (const { name, fn } of pending) {
  try {
    await fn();
    console.log(`PASS ${name}`);
  } catch (e) {
    failures.push(name);
    console.log(`FAIL ${name}`);
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

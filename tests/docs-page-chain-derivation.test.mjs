// Extracts and tests docs-page-chain.js's pure cross-regression derivation function, plus a few
// static guards against the profile/grader/v2 machinery the docs reset pass 1 spec retired. No
// dependencies beyond the Node runtime: this repo has no package.json or test runner to plug
// into. Run with: node tests/docs-page-chain-derivation.test.mjs
// Exits 0 with "ALL PASS" on success; prints failures and exits 1 otherwise.
import { readFileSync, mkdtempSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import { execFileSync, spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import assert from "node:assert/strict";

const HERE = dirname(fileURLToPath(import.meta.url));
const RUNNER_PATH = join(HERE, "..", "claude", ".claude", "workflows", "docs-page-chain.js");
const DRAFTER_PATH = join(HERE, "..", "claude", ".claude", "agents", "cairn-docs-drafter.md");
const RUNNER_SRC = readFileSync(RUNNER_PATH, "utf8");
const DRAFTER_SRC = readFileSync(DRAFTER_PATH, "utf8");

const START = "// === CROSS-REGRESSION DERIVATION (docs-page-chain-derivation.test.mjs extracts this block) ===";
const END = "// === END CROSS-REGRESSION DERIVATION ===";

const failures = [];
function check(name, fn) {
  try {
    fn();
    console.log(`ok - ${name}`);
  } catch (e) {
    failures.push(name);
    console.log(`FAIL - ${name}`);
    console.log(`  ${(e && e.stack) || e}`);
  }
}

// ---------------------------------------------------------------------------------------------
// Extract the pure derivation function from between its named markers. A missing or reordered
// marker fails this check directly, rather than letting a later check fail on an undefined
// function with a confusing message.
// ---------------------------------------------------------------------------------------------

const startAt = RUNNER_SRC.indexOf(START);
const endAt = RUNNER_SRC.indexOf(END);

check("the derivation markers are both present, in order", () => {
  assert.notEqual(startAt, -1, "start marker missing");
  assert.notEqual(endAt, -1, "end marker missing");
  assert.ok(startAt < endAt, "markers out of order");
});

let deriveCrossRegression = () => {
  throw new Error("derivation markers missing; function was not extracted");
};
if (startAt !== -1 && endAt !== -1) {
  const block = RUNNER_SRC.slice(startAt, endAt);
  // eslint-disable-next-line no-new-func -- extracting a pure function from a script the
  // workflow runtime cannot import (it has a top-level `return`) is the documented pattern.
  deriveCrossRegression = new Function(`${block}\nreturn deriveCrossRegression;`)();
}

/**
 * Builds a two-reviewer `reads` array for a synthetic round record.
 * @param {"accept"|"fix"} editorVerdict
 * @param {"accept"|"fix"} factVerdict
 * @returns {Array<{ read: string, verdict: string }>}
 */
function tworeads(editorVerdict, factVerdict) {
  return [
    { read: "register editor", verdict: editorVerdict },
    { read: "fact read", verdict: factVerdict }
  ];
}

// ---------------------------------------------------------------------------------------------
// The four synthetic round records the acceptance criteria name, each with its expected output.
// ---------------------------------------------------------------------------------------------

check("both accept in round 1: no round 2, flag absent", () => {
  const record = { rounds: [{ round: 1, reads: tworeads("accept", "accept") }] };
  assert.equal(deriveCrossRegression(record, false), undefined);
  assert.equal(deriveCrossRegression(record, true), undefined);
});

check("one fix, both re-read, the other flips to fix: flag set true, page qualifies", () => {
  const record = {
    rounds: [
      { round: 1, reads: tworeads("fix", "accept") },
      { round: 2, reads: tworeads("accept", "fix") }
    ]
  };
  assert.equal(deriveCrossRegression(record, true), true);
});

check("the same shape in lean mode: not measured, never false", () => {
  const record = {
    rounds: [
      { round: 1, reads: tworeads("fix", "accept") },
      // Lean mode: only the reviewer that returned fix (register editor) re-reads.
      { round: 2, reads: [{ read: "register editor", verdict: "accept" }] }
    ]
  };
  assert.equal(deriveCrossRegression(record, false), "not-measured");
});

check("a second fix from a re-reader: escalation, still not measured in lean mode", () => {
  const record = {
    rounds: [
      { round: 1, reads: tworeads("fix", "accept") },
      { round: 2, reads: [{ read: "register editor", verdict: "fix" }] }
    ]
  };
  assert.equal(deriveCrossRegression(record, false), "not-measured");
});

check("both reviewers re-read and neither regresses: flag set false", () => {
  const record = {
    rounds: [
      { round: 1, reads: tworeads("fix", "accept") },
      { round: 2, reads: tworeads("accept", "accept") }
    ]
  };
  assert.equal(deriveCrossRegression(record, true), false);
});

// ---------------------------------------------------------------------------------------------
// The register extraction, prompt renderers, and finding coercion: pure functions extracted from
// their own marker pair. The fixtures are synthetic registers, never the live register.
// ---------------------------------------------------------------------------------------------

const PURE_START = "// === REGISTER EXTRACTION AND COERCION (docs-page-chain-derivation.test.mjs extracts this block) ===";
const PURE_END = "// === END REGISTER EXTRACTION AND COERCION ===";
const pureStart = RUNNER_SRC.indexOf(PURE_START);
const pureEnd = RUNNER_SRC.indexOf(PURE_END);

check("the extraction markers are both present, in order", () => {
  assert.notEqual(pureStart, -1, "start marker missing");
  assert.notEqual(pureEnd, -1, "end marker missing");
  assert.ok(pureStart < pureEnd, "markers out of order");
});

let P = {};
if (pureStart !== -1 && pureEnd !== -1) {
  const block = RUNNER_SRC.slice(pureStart, pureEnd);
  const names = [
    "baseGuideFor", "sectionSpecsFor", "sectionCommand", "validateSection", "collectSections",
    "stripMarkers", "briefRuleIds", "coerceRead", "renderExemplars", "renderDrafterPrompt",
    "renderEditorPrompt"
  ];
  // eslint-disable-next-line no-new-func -- same pure-function extraction as above.
  P = new Function(`${block}\nreturn { ${names.join(", ")} };`)();
}

/**
 * A synthetic register carrying every section the extractor asks for, in the shape the ruled
 * inputs fix: two briefs, the layering sections, and the drafter-level sections.
 * @param {{ noBrief?: boolean, noMarker?: boolean }} [opts]
 * @returns {string}
 */
function fixtureRegister(opts = {}) {
  const marker = opts.noMarker ? "" : " <!-- q:g-active -->";
  const lines = [
    "# Register fixture",
    "",
    "Preamble that no stage receives: PREAMBLE-SENTINEL.",
    "",
    "## Universal contract (every page)",
    "",
    "UNIVERSAL-SENTINEL body that is not a drafter section.",
    "",
  ];
  if (!opts.noBrief) {
    lines.push(
      "## Drafting brief: developer docs",
      "",
      "### Structure",
      "",
      "Put the reader's task first, then the steps." + marker,
      "",
      "```",
      "## a fenced heading that must not end the section",
      "```",
      "",
      "### Voice",
      "",
      "DEV-VOICE-SENTINEL measured and precise, qualification inside the sentence. Padding to length.",
      "",
      "### Tells",
      "",
      "DEV-TELLS-SENTINEL no setup-colon payoff, no reflexive lists. Padding to length for the floor.",
      "",
      "## Drafting brief: editor docs",
      "",
      "### Structure",
      "",
      "EDITOR-BRIEF-SENTINEL steps in plain words. <!-- q:m-plain -->",
      "",
      "### Voice",
      "",
      "Plain second person. Padding to length for the minimum body check.",
      "",
      "### Tells",
      "",
      "No pitch. Padding to length for the minimum body check on the editor brief.",
      ""
    );
  }
  lines.push(
    "## Names",
    "",
    "NAMES-SENTINEL the engine is cairn, lowercase. Padding to length for the minimum body check.",
    "",
    "## Visuals (every page that carries one)",
    "",
    "VISUALS-SENTINEL one figure, one text alternative. Padding to length for the minimum body check.",
    "",
    "## The page anatomies",
    "",
    "ANATOMY-SENTINEL how-to: goal, prerequisites, steps. Padding to length for the minimum check.",
    "",
    "## The four tracks",
    "",
    "### The editor track (`docs/editors/`)",
    "",
    "EDITOR-TRACK-SENTINEL editors write in the admin. Padding to length for the minimum check.",
    "",
    "### The admin track (`docs/admin/`)",
    "",
    "ADMIN-TRACK-SENTINEL operators run the site. Padding to length for the minimum body check.",
    "",
    "### The extend track (`docs/extend/`)",
    "",
    "EXTEND-TRACK-SENTINEL developers build on the seams. Padding to length for the minimum check.",
    "",
    "## Provenance",
    "",
    "PROVENANCE-SENTINEL q:g-active | quote text | https://example.test/guide. Padding to length.",
    "",
    "## The tightening test",
    "",
    "TIGHTENING-SENTINEL a rule that forbids a form the guide prescribes needs a row. Padding.",
    "",
    "## Recorded exceptions: Google",
    "",
    "EXC-GOOGLE-SENTINEL | x:tone | tone delta | ruling 3. Padding to length for the minimum check.",
    "",
    "## Recorded exceptions: Microsoft",
    "",
    "EXC-MICROSOFT-SENTINEL | (none). Padding to length for the minimum check on the empty table.",
    ""
  );
  return lines.join("\n");
}

/**
 * Runs the rendered section command against a register fixture written to a temp checkout, the
 * way the page-inputs agent would, and returns its stdout verbatim.
 * @param {ReturnType<typeof P.sectionSpecsFor>[number]} spec
 * @param {string} registerText
 * @returns {string}
 */
function runSection(spec, registerText) {
  const dir = mkdtempSync(join(tmpdir(), "docs-chain-"));
  try {
    mkdirSync(join(dir, "docs", "internal"), { recursive: true });
    writeFileSync(join(dir, "docs", "internal", "docs-register.md"), registerText);
    return execFileSync("bash", ["-c", P.sectionCommand(spec, dir, "docs/internal/docs-register.md")], { encoding: "utf8" });
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

const specFor = (track, key) => P.sectionSpecsFor(track).find((s) => s.key === key);

check("track to base guide: editors give Microsoft, every other value gives Google", () => {
  assert.equal(P.baseGuideFor("editors"), "Microsoft");
  for (const t of ["admin", "extend", "reference", "front-door", "readme"]) {
    assert.equal(P.baseGuideFor(t), "Google", t);
  }
});

check("the brief section follows the track; an unknown track throws", () => {
  assert.equal(specFor("editors", "brief").heading, "## Drafting brief: editor docs");
  assert.equal(specFor("extend", "brief").heading, "## Drafting brief: developer docs");
  assert.equal(specFor("front-door", "brief").heading, "## Drafting brief: developer docs");
  assert.throws(() => P.sectionSpecsFor("nonsense"), /unknown track/);
});

check("brief present: the command returns the brief, fenced heading kept, next ## heading excluded", () => {
  const out = runSection(specFor("extend", "brief"), fixtureRegister());
  const v = P.validateSection(specFor("extend", "brief"), out);
  assert.equal(v.ok, true, v.reason);
  assert.match(v.body, /DEV-VOICE-SENTINEL/);
  assert.match(v.body, /a fenced heading that must not end the section/);
  assert.match(v.body, /DEV-TELLS-SENTINEL/);
  assert.doesNotMatch(v.body, /EDITOR-BRIEF-SENTINEL|NAMES-SENTINEL/);
});

check("a subsection range stops at the next heading of the same or higher level", () => {
  const out = runSection(specFor("admin", "track"), fixtureRegister());
  const v = P.validateSection(specFor("admin", "track"), out);
  assert.equal(v.ok, true, v.reason);
  assert.match(v.body, /ADMIN-TRACK-SENTINEL/);
  assert.doesNotMatch(v.body, /EDITOR-TRACK-SENTINEL|EXTEND-TRACK-SENTINEL/);
});

check("heading absent: the validator names it", () => {
  const out = runSection(specFor("extend", "brief"), fixtureRegister({ noBrief: true }));
  const v = P.validateSection(specFor("extend", "brief"), out);
  assert.equal(v.ok, false);
  assert.match(v.reason, /heading absent/);
});

check("empty return: rejected as empty, never as an accepted empty brief", () => {
  for (const out of ["", "   \n", undefined, null]) {
    const v = P.validateSection(specFor("extend", "brief"), out);
    assert.equal(v.ok, false);
    assert.match(v.reason, /empty return/);
  }
});

check("line-count mismatch: the sentinel count must equal the body lines", () => {
  const out = runSection(specFor("extend", "brief"), fixtureRegister());
  const dropped = out.replace(/^.*Put the reader's task first.*\n/m, "");
  const v = P.validateSection(specFor("extend", "brief"), dropped);
  assert.equal(v.ok, false);
  assert.match(v.reason, /line count/);
});

check("sentinel absent and wrong first line are named", () => {
  const out = runSection(specFor("extend", "brief"), fixtureRegister());
  const noSentinel = out.split("\n").filter((l) => !l.startsWith("=== cairn-section")).join("\n");
  assert.match(P.validateSection(specFor("extend", "brief"), noSentinel).reason, /sentinel/);
  const lines = out.split("\n");
  lines[0] = "## Something else";
  assert.match(P.validateSection(specFor("extend", "brief"), lines.join("\n")).reason, /first line/);
});

check("a body under the minimum length is rejected", () => {
  const short = "## Names\nx\n=== cairn-section names lines=2 ===\n";
  const v = P.validateSection(specFor("extend", "names"), short);
  assert.equal(v.ok, false);
  assert.match(v.reason, /under/);
});

check("a brief with no q: marker is rejected", () => {
  const out = runSection(specFor("extend", "brief"), fixtureRegister({ noMarker: true }));
  const v = P.validateSection(specFor("extend", "brief"), out);
  assert.equal(v.ok, false);
  assert.match(v.reason, /q: marker/);
});

/**
 * The page-inputs return for one track, built by running every rendered command on a fixture.
 * @param {string} track
 * @param {string} [registerText]
 * @returns {Array<{ key: string, output: string }>}
 */
function extractAll(track, registerText = fixtureRegister()) {
  return P.sectionSpecsFor(track).map((s) => ({ key: s.key, output: runSection(s, registerText) }));
}

check("collectSections accepts a full return and names the first rejected section otherwise", () => {
  const good = P.collectSections("extend", extractAll("extend"));
  assert.equal(good.ok, true, good.reason);
  assert.match(good.sections.brief, /DEV-VOICE-SENTINEL/);
  const partial = extractAll("extend").filter((s) => s.key !== "visuals");
  const bad = P.collectSections("extend", partial);
  assert.equal(bad.ok, false);
  assert.match(bad.reason, /visuals/);
});

const PAGE = { id: "x", path: "docs/extend/x.md", track: "extend", job: "Explain x.", pinned: [] };
const INPUTS = {
  job: "Explain x to a developer.",
  pageType: "how-to",
  exemplarExcerpts: [
    { source: "docs/extend/a.md", excerpt: "STEP-EXCERPT", note: "prose steps depart from the guide" },
    { source: "docs/editors/b.md", excerpt: "VOICE-EXCERPT" }
  ],
  factIds: ["f-1"],
  claimInventory: []
};
const SOURCES = [
  { path: "docs/extend/a.md", role: "anatomy" },
  { path: "docs/editors/b.md", role: "voice" }
];

check("the drafter prompt holds brief, Names, Visuals, track, and anatomy and no layering", () => {
  const { sections } = P.collectSections("extend", extractAll("extend"));
  const prompt = P.renderDrafterPrompt({
    wt: "/wt", valeRules: "Cairn.HeadingIng", page: PAGE, pageInputs: INPUTS, sections,
    exemplarSources: SOURCES, briefPath: "docs/internal/briefs/extend/x.json",
    round: 1, findings: "", gateText: "GATE-TEXT"
  });
  for (const s of ["DEV-VOICE-SENTINEL", "NAMES-SENTINEL", "VISUALS-SENTINEL", "EXTEND-TRACK-SENTINEL", "ANATOMY-SENTINEL", "GATE-TEXT"]) {
    assert.ok(prompt.includes(s), `drafter prompt lacks ${s}`);
  }
  for (const s of ["PROVENANCE-SENTINEL", "TIGHTENING-SENTINEL", "EXC-GOOGLE-SENTINEL", "EXC-MICROSOFT-SENTINEL", "EDITOR-BRIEF-SENTINEL", "UNIVERSAL-SENTINEL", "PREAMBLE-SENTINEL"]) {
    assert.ok(!prompt.includes(s), `drafter prompt carries ${s}`);
  }
  assert.doesNotMatch(prompt, /<!--\s*[qx]:/, "a marker survived in the drafter prompt");
  assert.doesNotMatch(prompt, /\bq:/);
  assert.doesNotMatch(prompt, /docs-register\.md/);
  assert.doesNotMatch(prompt, /provenance/i);
  assert.doesNotMatch(prompt, /exceptions/i);
});

check("the drafter prompt names no register path even when the caller passes none", () => {
  const { sections } = P.collectSections("editors", extractAll("editors"));
  const prompt = P.renderDrafterPrompt({
    wt: "/wt", valeRules: "", page: { ...PAGE, track: "editors" }, pageInputs: INPUTS, sections,
    exemplarSources: SOURCES, briefPath: "b.json", round: 2, findings: "F", gateText: "G"
  });
  assert.match(prompt, /EDITOR-BRIEF-SENTINEL/);
  assert.match(prompt, /EDITOR-TRACK-SENTINEL/);
  assert.doesNotMatch(prompt, /register/i);
});

check("exemplar roles: an anatomy excerpt borrows structure only, a voice excerpt borrows the voice", () => {
  const out = P.renderExemplars(INPUTS.exemplarExcerpts, SOURCES);
  const [anatomy, voice] = out.split(/\n\n(?=<example)/);
  assert.match(anatomy, /imitate the section order and step form only; its sentences are not the voice/);
  assert.match(anatomy, /prose steps depart from the guide/);
  assert.doesNotMatch(voice, /section order and step form only/);
  assert.match(voice, /sentence rhythm/);
});

check("the editor prompt adds the layering with markers kept and names the guide before the register", () => {
  const { sections } = P.collectSections("editors", extractAll("editors"));
  const prompt = P.renderEditorPrompt({ wt: "/wt", valeRules: "", page: { ...PAGE, track: "editors" }, sections });
  for (const s of ["EDITOR-BRIEF-SENTINEL", "PROVENANCE-SENTINEL", "TIGHTENING-SENTINEL", "EXC-GOOGLE-SENTINEL", "EXC-MICROSOFT-SENTINEL", "NAMES-SENTINEL"]) {
    assert.ok(prompt.includes(s), `editor prompt lacks ${s}`);
  }
  assert.match(prompt, /<!-- q:m-plain -->/);
  assert.match(prompt, /Microsoft/);
  assert.ok(prompt.search(/Guide conformance/i) < prompt.indexOf("EDITOR-BRIEF-SENTINEL"), "guide lens must precede the register brief");
  assert.match(prompt, /source \("guide" or "register"\)/);
});

check("markers are stripped for the drafter and read back from the brief", () => {
  const text = "A quote. <!-- q:one -->\nB row <!-- x:two -->\n";
  assert.equal(P.stripMarkers(text), "A quote.\nB row\n");
  assert.deepEqual([...P.briefRuleIds("a <!-- q:one --> b <!-- q:two-b -->")].sort(), ["q:one", "q:two-b"]);
});

const BRIEF = "Quote one. <!-- q:g-active -->\nQuote two. <!-- q:g-second -->";
const read = (verdict, findings) => ({ verdict, findings, summary: "s" });

check("a finding with no source is treated as guide; a brief rule id forces blocking and verdict fix", () => {
  const r = P.coerceRead(read("accept", [{ location: "l", finding: "f", blocking: false, rule: "q:g-active" }]), BRIEF);
  assert.equal(r.findings[0].source, "guide");
  assert.equal(r.findings[0].blocking, true);
  assert.equal(r.verdict, "fix");
});

check("an unknown source is treated as guide", () => {
  const r = P.coerceRead(read("accept", [{ location: "l", finding: "f", blocking: false, source: "vibes", rule: "q:g-second" }]), BRIEF);
  assert.equal(r.findings[0].source, "guide");
  assert.equal(r.findings[0].blocking, true);
  assert.equal(r.verdict, "fix");
});

check("a guide finding with blocking false on a brief rule id is forced, and the verdict is fix", () => {
  const r = P.coerceRead(read("accept", [{ location: "l", finding: "f", blocking: false, source: "guide", rule: "q:g-active" }]), BRIEF);
  assert.equal(r.findings[0].blocking, true);
  assert.equal(r.verdict, "fix");
});

check("a guide finding on a rule not in the brief keeps the reviewer's value", () => {
  const r = P.coerceRead(read("accept", [
    { location: "l", finding: "f", blocking: false, source: "guide", rule: "q:not-in-brief" },
    { location: "l", finding: "f", blocking: false, source: "guide" }
  ]), BRIEF);
  assert.deepEqual(r.findings.map((f) => f.blocking), [false, false]);
  assert.equal(r.verdict, "accept");
});

check("a register finding keeps its value, and a blocking one still sets the verdict to fix", () => {
  const kept = P.coerceRead(read("accept", [{ location: "l", finding: "f", blocking: false, source: "register", rule: "q:g-active" }]), BRIEF);
  assert.equal(kept.findings[0].blocking, false);
  assert.equal(kept.verdict, "accept");
  const blocked = P.coerceRead(read("accept", [{ location: "l", finding: "f", blocking: true, source: "register" }]), BRIEF);
  assert.equal(blocked.verdict, "fix");
});

check("coercion does not mutate its input", () => {
  const input = read("accept", [{ location: "l", finding: "f", blocking: false, source: "guide", rule: "q:g-active" }]);
  P.coerceRead(input, BRIEF);
  assert.equal(input.verdict, "accept");
  assert.equal(input.findings[0].blocking, false);
});

const RENDER_CLI = join(HERE, "..", "scripts", "docs-chain-render.mjs");

/**
 * Runs the render script against a temp checkout holding the fixture register.
 * @param {string} subcommand
 * @param {string[] | ((dir: string) => string[])} cliArgs - flags after the subcommand;
 *   `--checkout` is supplied here, and a function receives the checkout directory
 * @param {(dir: string) => void} [setup] - writes extra files into the checkout
 * @param {string} [registerText]
 * @returns {{ status: number, stdout: string, stderr: string }}
 */
function runCli(subcommand, cliArgs, setup, registerText = fixtureRegister()) {
  const dir = mkdtempSync(join(tmpdir(), "docs-chain-cli-"));
  try {
    mkdirSync(join(dir, "docs", "internal"), { recursive: true });
    writeFileSync(join(dir, "docs", "internal", "docs-register.md"), registerText);
    if (setup) setup(dir);
    const r = spawnSync("node", [RENDER_CLI, subcommand, "--checkout", dir, ...(typeof cliArgs === "function" ? cliArgs(dir) : cliArgs)], { encoding: "utf8" });
    return { status: r.status, stdout: r.stdout, stderr: r.stderr };
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

check("the render script renders the editor prompt and the drafter prompt from a checkout", () => {
  const editor = runCli("render", ["--kind", "editor", "--page", "docs/extend/x.md", "--track", "extend"]);
  assert.equal(editor.status, 0, editor.stderr);
  assert.match(editor.stdout, /PROVENANCE-SENTINEL/);
  assert.match(editor.stdout, /<!-- q:g-active -->/);
  const drafter = runCli("render", ["--kind", "drafter", "--page", "docs/extend/x.md", "--track", "extend"]);
  assert.equal(drafter.status, 0, drafter.stderr);
  assert.match(drafter.stdout, /DEV-VOICE-SENTINEL/);
  assert.doesNotMatch(drafter.stdout, /PROVENANCE-SENTINEL|<!--\s*q:|docs-register\.md/);
});

check("the render script exits 1 with the named reason when a section is rejected", () => {
  const r = runCli("render", ["--kind", "drafter", "--page", "docs/extend/x.md", "--track", "extend"], undefined, fixtureRegister({ noBrief: true }));
  assert.equal(r.status, 1);
  assert.match(r.stderr, /brief: heading absent/);
});

check("the render script's coerce mode forces a guide finding on a brief rule and flips the verdict", () => {
  const findings = [{ location: "l", finding: "f", blocking: false, source: "guide", rule: "q:g-active" }];
  const r = runCli(
    "coerce",
    (dir) => ["--track", "extend", "--read", join(dir, "read.json")],
    (dir) => writeFileSync(join(dir, "read.json"), JSON.stringify(findings))
  );
  assert.equal(r.status, 0, r.stderr);
  const coerced = JSON.parse(r.stdout);
  assert.equal(coerced.verdict, "fix");
  assert.equal(coerced.findings[0].blocking, true);
});

check("the whole runner parses as an async function body", () => {
  const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
  // eslint-disable-next-line no-new-func -- a parse check only; the function is never called.
  new AsyncFunction("args", "agent", "parallel", "log", "phase", RUNNER_SRC.replace("export const meta", "const meta"));
});

check("the runner coerces before anyFix and no longer builds the register path list", () => {
  assert.match(RUNNER_SRC, /coerceRead\(/);
  assert.doesNotMatch(RUNNER_SRC, /registerPaths/);
});

// ---------------------------------------------------------------------------------------------
// Static guards for the removed profile/grader/v2 machinery: the acceptance's grep bullet, made
// a durable regression instead of a one-time manual check.
// ---------------------------------------------------------------------------------------------

check("the runner takes no profile arg", () => {
  assert.doesNotMatch(RUNNER_SRC, /a\.profile\b/);
  assert.doesNotMatch(RUNNER_SRC, /args\.profile\b/);
});

check("the runner has no grader stage or Profile prompt section", () => {
  assert.doesNotMatch(RUNNER_SRC, /graderPrompt/);
  assert.doesNotMatch(RUNNER_SRC, /GRADER_SCHEMA/);
  assert.doesNotMatch(RUNNER_SRC, /"Profile"/);
});

check("the runner names no v2 chain", () => {
  assert.doesNotMatch(RUNNER_SRC, /docs-page-chain-v2/);
});

check("the drafter definition names no grader or v2 chain, and runs its own gate", () => {
  assert.doesNotMatch(DRAFTER_SRC, /grader/i);
  assert.doesNotMatch(DRAFTER_SRC, /v2/);
  assert.doesNotMatch(DRAFTER_SRC, /Do not run the page gate/);
  assert.doesNotMatch(DRAFTER_SRC, /\[candidate\]/);
});

check("the per-page record carries its brief path from the start, on every return path", () => {
  assert.match(RUNNER_SRC, /const record = \{ id: p\.id, path: p\.path, brief: briefPathFor\(p\), rounds: \[\] \};/);
});

check("a missing fact is a couldNotDo, not a friction-log entry", () => {
  assert.match(RUNNER_SRC, /couldNotDo naming the missing fact/);
});

check("the tool gate defaults to make -C <worktree>/tool check", () => {
  assert.match(RUNNER_SRC, /a\.toolGate \|\| `make -C \$\{WT\}\/tool check`/);
});

console.log("");
if (failures.length) {
  console.log(`${failures.length} FAILING: ${failures.join(", ")}`);
  process.exit(1);
}
console.log("ALL PASS");

// Extracts and tests docs-page-chain.js's pure cross-regression derivation function, plus a few
// static guards against the profile/grader/v2 machinery the docs reset pass 1 spec retired. No
// dependencies beyond the Node runtime: this repo has no package.json or test runner to plug
// into. Run with: node tests/docs-page-chain-derivation.test.mjs
// Exits 0 with "ALL PASS" on success; prints failures and exits 1 otherwise.
import { readFileSync } from "node:fs";
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

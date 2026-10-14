// Renders every case of the drafter parity matrix through the docs page chain under Node and
// prints the result as the expect object `drafter check-prompts --expect -` reads: a JSON object
// mapping each case name to its prompt. tests/drafter-prompts-parity.sh pipes this into drafter,
// which renders the same cases in goja, so the gate proves the two engines agree byte for byte.
//
// The chain loads the way drafter's loader loads it: the line matching `^export const meta` loses
// its `export`, the script runs as an async function body that adds nothing before line 1, and
// `args`, `DRAFTER_HOST`, no-op `phase`, `log`, `parallel`, and `pipeline`, and an `agent` that
// throws are its only globals. The host and each context cross as JSON text, parsed here as
// drafter's runtime parses them.
// Run with: node tests/drafter-render-prompts.mjs [matrix.json [module.js]]
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const HERE = dirname(fileURLToPath(import.meta.url));
const matrixPath = process.argv[2] || join(HERE, "fixtures", "drafter", "matrix.json");
const modulePath = process.argv[3] || join(HERE, "..", "claude", ".claude", "workflows", "docs-page-chain.js");

const matrix = JSON.parse(readFileSync(matrixPath, "utf8"));
const body = readFileSync(modulePath, "utf8").replace(/^export const meta/m, "const meta");

const AsyncFunction = (async () => {}).constructor;
const load = new AsyncFunction("args", "DRAFTER_HOST", "phase", "log", "parallel", "pipeline", "agent", body);
const hostText = JSON.stringify({ args: matrix.host.args, resolved: matrix.host.resolved });
const seats = await load(
  JSON.parse(JSON.stringify(matrix.host.args)),
  JSON.parse(hostText),
  () => {},
  () => {},
  () => {},
  () => {},
  () => {
    throw new Error("agent called at load: a prompts module cannot start agents");
  }
);

const rendered = {};
for (const c of matrix.cases) {
  if (typeof seats[c.seat] !== "function") throw new Error(`case ${c.name}: the module has no seat "${c.seat}"`);
  rendered[c.name] = seats[c.seat](JSON.parse(JSON.stringify(c.context)));
}
process.stdout.write(JSON.stringify(rendered));

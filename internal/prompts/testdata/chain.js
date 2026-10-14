// A synthetic chain-shaped prompts module.
// The phrase export const meta appears in this comment and must survive the rewrite.
export const meta = { name: "synthetic-chain" };

async function resolvePages() {
  if (typeof DRAFTER_HOST !== "undefined") return DRAFTER_HOST.resolved;
  return await agent("outline");
}

const resolved = await resolvePages();
const INDEX = resolved.index;

function draftPrompt(c) {
  if (c.round === 1) return `Draft ${c.page.path} in ${INDEX}.`;
  if (c.afterFinalRead) return `Redraft after the final read: ${c.findings}`;
  return `Redraft ${c.page.path}: ${c.findings}`;
}

function readerPrompt(c) {
  return c.turn === 2 ? `Step 3: ${c.inputs.job}` : `Steps 1 and 2 for ${c.page.path}.`;
}

const seats = {
  draft: draftPrompt,
  reader: readerPrompt,
  echo: (c) => JSON.stringify(c),
  throws: (c) => { throw new Error("builder broke"); },
  number: (c) => 42,
};

if (typeof DRAFTER_HOST !== "undefined") return seats;

return {};

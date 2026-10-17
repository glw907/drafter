// A synthetic prompts module shaped like the chain's back half: one builder per seat, each a
// function of the seat context object.
const resolved = DRAFTER_HOST.resolved;

function narrowText(c) {
  if (!c.narrow) return "";
  let out = `\nNarrowed to the changes since ${c.narrow.baseline}.`;
  if (c.narrow.own) out += `\nYour round 1 verdict: ${c.narrow.own.verdict}`;
  return out;
}

const seats = {
  draft: (c) => {
    if (c.round === 1) return `Draft ${c.page.path} for ${resolved.index}.`;
    const after = c.afterFinalRead ? " after the final read" : "";
    return `Redraft ${c.page.path}${after}.\nFindings:\n${c.findings}`;
  },
  blueprint: () => "Read the blueprint below.",
  structure: (c) => `Structure read of ${c.page.path} round ${c.round}.${narrowText(c)}`,
  editor: (c) => `Editor read of ${c.page.path} round ${c.round}.${c.scope ? "\nScope: " + c.scope : ""}${narrowText(c)}`,
  fact: (c) => `Fact read of ${c.page.path} round ${c.round}.${c.scope ? "\nScope: " + c.scope : ""}${narrowText(c)}`,
  reader: (c) => {
    if (c.turn === 1) return `Read ${c.page.path} and paraphrase it.`;
    return `The page's job: ${c.page.job}. Judge ${c.page.path}.`;
  },
};
return seats;

// A synthetic prompts module for the check-prompts end-to-end tests.
export const meta = { name: "synthetic" };

const resolved = DRAFTER_HOST.resolved;

function draftPrompt(c) {
  if (c.round === 1) return `Draft ${c.page.path} in ${resolved.index}.`;
  return `Redraft ${c.page.path}:\n${c.findings}`;
}

function editorPrompt(c) {
  let out = `Edit ${c.page.path}.`;
  if (c.narrow) {
    out += `\nBaseline ${c.narrow.baseline}.`;
    if (c.narrow.own) out += "\n" + c.narrow.own.findings.map((f) => f.finding).join("\n");
  }
  return out;
}

if (typeof DRAFTER_HOST !== "undefined") {
  return {
    draft: draftPrompt,
    editor: editorPrompt,
    // A seat that drops its findings, for the failing case.
    lossy: (c) => `Redraft ${c.page.path}.`,
  };
}
return {};

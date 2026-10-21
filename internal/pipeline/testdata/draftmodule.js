// A synthetic prompts module for the drafting tests. Every seat echoes the context fields the
// pipeline sets, one per line, so a test reads them back from the request.
function echo(seat) {
  return (c) =>
    [
      `seat ${seat} reads ${c.page.path}`,
      `round ${c.round}`,
      `turn ${c.turn || 0}`,
      `scope [${c.scope || ""}]`,
      `findings [${c.findings || ""}]`,
      `markdown [${c.markdown}]`,
      `preferences [${c.preferences || ""}]`,
    ].join("\n");
}

const seats = {};
for (const s of ["blueprint", "draft", "edit", "structure", "editor", "fact", "reader", "coherence"]) {
  seats[s] = echo(s);
}
return seats;

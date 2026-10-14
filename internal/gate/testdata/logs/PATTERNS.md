# Gate log patterns

The four logs are scrubbed runs of a docs gate: a green run, a provenance failure, a Vale error,
and a broken link on a second page. Page A is `docs/guides/widget-setup.md`, its brief is
`docs/briefs/guides/widget-setup.json`, and page B is `docs/guides/other-page.md`.

Section pattern, whose first capture group names the section. The npm package step has a
different header shape, so it stays in the preamble:

    ^== (check:\S+) ==$

Failure pattern, derived from the recorded logs. The tellgrader alternative comes from the
format its check script prints (`<file>:<line>: <excerpt>`), not from a recorded log:

    ✖|defect\(s\)|broken link|target not found|\.json: [^0-9]|^ +\S+\.md$|^ +[0-9]+:[0-9]+ +error |^\S+\.md:[0-9]+: ([^w]|w[^a])

Each alternative has a reason. A Vale run names the file on a header line (`^ +\S+\.md$`) and the
error on lines below it. A provenance defect names the brief and starts its message with text, so
`\.json: [^0-9]` skips the green `: 20 sentences` line. The final alternative is a tellgrader
finding; its `[^w]|w[^a]` tail keeps the leak scan's `warning` lines, which also start `file:line:`,
out of it, because RE2 has no lookahead.

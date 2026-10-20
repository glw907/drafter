# The prompts module

A prompts module is a JavaScript file that writes the prompt for each seat. drafter runs it
in-process with [goja](https://github.com/dop251/goja), an ECMAScript engine written in Go, so
you need no Node installation to run drafter. Name the file in the `prompts` setting.

The prompts are code, not templates. A builder can branch on the page type, the round, and the
findings of an earlier read, and it can compute text from any of them.

## The stock module

When `prompts` is unset, drafter runs the stock module that ships in its binary. `drafter config`
shows it as `drafter:prompts.js` with a short hash. A run copies the module into its run
directory, so a resume after an upgrade uses the module the run started with.

The stock module drafts and reviews a page from its blueprint. It defines seven seats:

| Seat | What it does | What it returns |
|---|---|---|
| `blueprint` | Reads the blueprint before any prose exists, as a developmental editor. `drafter read` renders it, and so does `drafter page` before it drafts. | A read. |
| `draft` | Writes the page and its brief from the blueprint and the facts it places. Every other turn, including a section redraft's first turn, edits the page as it stands. | For a whole draft, the page, the brief, and any friction. For every other turn, edits. |
| `structure` | Grades the page against its blueprint, its anatomy, and two published structure checklists. | A read. |
| `editor` | Grades the prose against the Google rubric and the patterns the standard rules out. | A read. |
| `fact` | Checks each id the brief cites against the facts and their source excerpts. | A read. |
| `reader` | Reads the page cold, as its reader, and then against its job. | On turn 1, a paraphrase and a summary. On turn 2, a read. |
| `coherence` | After a section redraft, reads the whole page as one argument: its transitions, its repetition, and its flow. | A read. |

A run that needs a seat the module lacks refuses before it spends anything, and the error names
the missing seats.

The drafting seat writes to the Google developer documentation style guide. The stock module
carries that standard as four files, each a reference page you can read in the repository:

| File | Holds | Read by |
|---|---|---|
| [`google-description.md`](../stock/google-description.md) | The standard, its reader, its source, and its license. | `draft` |
| [`google-tells.md`](../stock/google-tells.md) | The patterns the standard rules out, each with a before-and-after pair. | `draft` and `editor` |
| [`google-rubric.md`](../stock/google-rubric.md) | Short rules paraphrased from the guide, each linked to the guide's page. | `editor` |
| [`google-corpus.md`](../stock/google-corpus.md) | Short excerpts from the guide's pages, each with its URL, license, and retrieval date. | `draft`, for voice only |

The drafting, structure, and editor seats also read the page's anatomy from
[`anatomies.md`](../stock/anatomies.md), and the drafting seat reads
[`introductions.md`](../stock/introductions.md). When the page's track names a register, the seats
read the anatomy from that register's anatomies section instead.

The whole draft's prompt states the brief's schema and the provenance rules that
`drafter gate --page` checks. Every sentence of the page's prose is in the brief. Each sentence
cites the facts it rests on, or `no-claim`. A cited sentence states nothing its facts don't.

### Name a file or a section in a prompt

A prompt can name a file by path, and drafter inlines the file into the seat's request. Two more
tokens work in any prompt:

- `drafter:<name>` names a file embedded in drafter, such as `drafter:anatomies.md`. drafter
  inlines it as a file block labeled with the token. An unknown name inlines as `not inlined: no
  drafter file <name>`.
- `<path>#<slug>` names one section of a file. The path is a tracked file, a file that
  `inline_files` allows, or a `drafter:` name. drafter inlines the section that starts at the
  heading whose GitHub-style slug is `<slug>` and runs to the next heading of the same or a higher
  level. A slug that no heading has inlines as `no excerpt: no heading <slug>`.

The slug is the same one `drafter check` uses for `#anchor` links. drafter inlines
a file or section of at most 256 KiB. The limit applies to the excerpt, not the whole file, so a
section of a large file can fit when the file would not.

A project can refuse the stock module with `require_prompts = true` in `drafter.toml`. See
[Configuration](configuration.md).

## What the module returns

The module runs as the body of an async function. It returns an object that maps seat names to
builder functions. Each builder takes one [context object](#the-context-object) and returns the
prompt as a string.

```js
const resolved = DRAFTER_HOST.resolved;

function draftPrompt(c) {
  if (c.round === 1) return `Draft ${c.page.path} for the index at ${resolved.index}.`;
  return `Redraft ${c.page.path}. Address these findings:\n${c.findings}`;
}

return {
  draft: draftPrompt,
  reader: (c) => `Read ${c.page.path}, turn ${c.turn}.`,
};
```

The seat names are `draft`, `structure`, `editor`, `fact`, `reader`, `blueprint`, and
`coherence`. The module needs a builder for each of these seats that runs. drafter builds the
`coverage` and `consistency` prompts itself, so a module never defines them. Asking for a seat the
module doesn't define fails with an error that lists the seats it does define.

A builder that throws, and a builder that returns anything other than a string, are errors that
name the module, the seat, and the line.

## What the loader does

drafter reads the module and makes one textual change before it runs: a line that starts with
`export const meta` becomes `const meta`, because goja rejects ES module syntax. The loader adds
nothing before your first line, so the line numbers in an error match your file.

The module sees these globals:

| Global | Holds |
|---|---|
| `args` | The `args` of the handoff's `context`, or `{}` when there are none. |
| `DRAFTER_HOST` | An object with the context's `args` and `resolved`. `resolved` holds the resolved pages and the index. |
| `phase`, `log`, `parallel`, `pipeline` | Functions that do nothing, so a script written for a planner can run unchanged. |
| `agent` | A function that throws. A module can't start an agent while it loads. |

The module can test `typeof DRAFTER_HOST !== "undefined"` to tell whether drafter is running it,
and return the seat map early when it is. drafter refuses to load a module when the handoff's
`context` has no `resolved` value, so a handoff that feeds a prompts module needs one. See [The handoff](handoff.md).

## The context object

drafter calls a builder with one object. It renders the object to JSON and parses it inside the
module, so every value is plain data.

| Field | Holds |
|---|---|
| `page` | The page's resolved entry: `path`, `slug`, `title`, `pageType`, `job`, `covers`, `outOfScope`, `figure`, and `anatomy`, plus `register` when the page's track names one. A blueprint page's entry also holds `brief`, the brief's path, `standard`, the track's standard, and `reader`, the track's reader profile. A handoff with no resolved entry for the page gets `{path, brief, type, job}`. |
| `inputs` | The page inputs: `job`, `pageType`, `registerSections`, `exemplarSources`, `claimInventory`, and `factIds`. A blueprint page sends `registerSections` and `claimInventory` empty. |
| `round` | The round the seat renders: `1`, `2`, or `"final"`. The final round is the redraft and the scoped reads that follow the last reader's read. |
| `findings` | The findings text that the seat addresses, as a string. For the `blueprint` seat, it's the read bar when one applies: the blocking findings of the last whole read that blocked, one per line as `<n>. [<location>] <text>`, numbered from 1. The seat judges whether the change fixed each one. A finding it didn't fix blocks, whatever its class. With no bar, it's an empty string. |
| `afterFinalRead` | `true` when the seat renders after the final reader's read. |
| `scope` | What the read or redraft is limited to, or an empty string. The value depends on the seat; see [The scope](#the-scope). |
| `where` | A string that names where in the page the read applies. drafter sends an empty string, and a module may ignore it. |
| `narrow` | At round 2, an object `{baseline, own}`: the id of the snapshot of the page that round 1 read, and the seat's own round-1 answer. Otherwise `null`. |
| `turn` | The turn number, for a seat that runs more than one turn, such as `reader`. |
| `facts` | `true` when the project has a facts store, so a module can word the fact read for either case. |

A round-2 prompt must contain each finding of `narrow.own`, and `drafter check-prompts` checks
that it does. The stock module has no seat input command, so its round-2 read verifies its own
round-1 findings on the page as it stands.

### The scope

`scope` holds a different value for each seat:

- `blueprint`: the sections the blueprint changed since the lock's revision, or `page` for a change
  that touches no section's own content. An empty string is a read of the whole blueprint.
- `draft`, `structure`, `editor`, `fact`, and `coherence` at round 1 or 2: the comma-joined ids of
  the sections a section redraft changes, or an empty string for a whole draft. The blueprint
  block maps each id to its heading.
- `editor` and `fact` at round `"final"`: the findings the final redraft answered, as it answered
  them.

### The blocks after the prompt

drafter appends blocks of its own after a blueprint page's prompt. A prompt doesn't need to name
them:

1. The facts the blueprint places or cites, each with its bullet's text.
2. The blueprint and the page's outline entry.
3. For a section redraft's draft request, a sentence naming the sections in scope and the change
   block: the blueprint's diff and the old and new text of each fact bullet that changed.

The `draft`, `structure`, `editor`, `fact`, and `coherence` requests carry these blocks. The
`reader` requests carry none of them. The reader sees only the page. A prompt names the page and
its brief by path, so drafter inlines them as files. It never names the blueprint's path.

### What each answer holds

The answer a seat returns depends on the turn:

- `draft` returns the page, the brief, and any friction for a whole draft, at round 1 with no
  findings. Every other `draft` turn returns edits.
- `structure`, `editor`, `fact`, `coherence`, and the reader's turn 2 return a read: a verdict,
  the findings, and a summary.
- The reader's turn 1 returns a paraphrase and a summary.

## What the runtime lacks

The goja runtime defines no `require`, `fetch`, `process`, `console`, or `setTimeout`. A module can
compute text and do nothing else. It can't read a file, call the network, or print.

The module can use `await`, because it runs under an async wrapper. It can't use ES module
syntax: `import` and `export` fail to compile.

## Check a module

`drafter check-prompts` renders a matrix of cases and checks each prompt. Pass the cases in a JSON
file:

```json
{
  "host": {"args": {}, "resolved": {"index": "docs/README.md"}},
  "cases": [
    {"name": "draft round 1", "seat": "draft", "context": {"page": {"path": "docs/a.md"}, "round": 1}}
  ]
}
```

Run:

```sh
drafter check-prompts --matrix matrix.json --module prompts.js
```

Each prompt except the `blueprint` seat's must contain its context's `page.path` and, when the
context has findings, the findings text. The `blueprint` prompt must not name `page.path`, because
its request inlines every path it names and the page isn't written yet. With `--expect <file>`, drafter compares each prompt byte for byte with the
rendering that the file holds for the same case name. A name on only one side fails. The command
exits 0 when every case passes, 1 when any case fails, and 2 when it can't read or load an input.

When the project sets no `prompts`, the command checks drafter's stock module, and two more checks
run on each prompt. A project module, set by `prompts` or `--module`, gets neither.

- `path`: every token that looks like a path must resolve. A token is a path when it holds a `/`
  and no `://`, or begins `drafter:`. drafter trims a trailing `.`, `,`, `;`, or `:`, then cuts
  any `#slug`. A repository path must name a file or directory that git tracks, except the
  context's `page.path` and `page.brief`, which the run itself produces. A `drafter:` name must be
  one of the stock files. A token that doesn't resolve fails, and the failure names it.
- `names`: the prompt must not hold `cairn`, `docs/internal`, `docs-register`, `sveltekit`,
  `cloudflare`, `wrangler`, `glw907`, `npm run check:`, or `option-map`, in any letter case. Each
  string found fails once.

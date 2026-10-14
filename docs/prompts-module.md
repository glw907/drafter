# The prompts module

A prompts module is a JavaScript file that writes the prompt for each seat. drafter runs it
in-process with [goja](https://github.com/dop251/goja), an ECMAScript engine written in Go, so
you need no Node installation to run drafter. Name the file in the `prompts` setting.

The prompts are code, not templates. A builder can branch on the page type, the round, and the
findings of an earlier read, and it can compute text from any of them.

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

The seat names are `draft`, `structure`, `editor`, `fact`, `reader`, `coverage`, and
`consistency`. The module needs a builder for each seat that runs. Asking for a seat the module
doesn't define fails with an error that lists the seats it does define.

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
| `page` | The page's path, brief, and type from the handoff. |
| `inputs` | The handoff's page inputs. |
| `round` | The round the seat renders: `1`, `2`, or `"final"`. The final round is the redraft and the scoped reads that follow the last reader's read. |
| `findings` | The findings text that the seat addresses, as a string. |
| `afterFinalRead` | `true` when the seat renders after the final reader's read. |
| `scope` | The findings a scoped read after the final reader's read checks, as the redraft answered them, or an empty string. |
| `where` | A string that names where in the page the read applies. drafter sends an empty string, and a module may ignore it. |
| `narrow` | An object that describes a narrowed read, or `null`. |
| `turn` | The turn number, for a seat that runs more than one turn, such as `reader`. |
| `facts` | `true` when the project has a facts store, so a module can word the fact read for either case. |

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

Each prompt must contain its context's `page.path` and, when the context has findings, the
findings text. With `--expect <file>`, drafter compares each prompt byte for byte with the
rendering that the file holds for the same case name. A name on only one side fails. The command
exits 0 when every case passes, 1 when any case fails, and 2 when it can't read or load an input.

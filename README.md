# drafter

drafter drafts and reviews documentation pages through the Claude API. A planner, such as a
Claude Code session, plans each page and hands it to drafter. drafter then runs the page through a
pipeline of seats: a draft, then reads by a structure editor, a register editor, a fact checker, a
reader, and coverage and consistency checks. It runs your gate after each write, enforces
spending caps, and stops with a record of what it did.

drafter also validates page plans offline, before any drafting begins.

Nothing in drafter belongs to one project. A project opts in with a `drafter.toml` at its root,
and every setting that differs between users, machines, or projects lives in a configuration
layer.

## Install

You need Go. From a checkout of this repository, run:

```sh
make install
```

The command builds the binary and copies it to `~/.local/bin/drafter`. To install somewhere
else, set `PREFIX`, as in `make install PREFIX=/usr/local`. The binary then goes to
`$PREFIX/bin`.

The install also links the docs page chain's two pieces from the checkout. `docs-page-chain.js`
goes to `~/.claude/workflows/`, and `cairn-docs-outline` goes to `$PREFIX/bin`. Set `CLAUDE_DIR`
to link the workflow somewhere other than `~/.claude`. The install stops with a message when
either destination directory is a symlink, since a link written through it would land in another
tree. Replace the symlink with a real directory and run the install again.

Before your first run of `drafter page`, follow [Set up drafter](docs/setup.md). The pipeline
spends money, and it refuses to start until you set a per-page cap and a per-cycle ceiling.

## Commands

Every command prints for a person by default and takes `--json` for scripts. Run
`drafter <command> --help` for every flag.

### page

```sh
drafter page [--workflow-run <run-file>] [--resume <run-id>] [--whole] [--judge] [--dry-run] [--sync] [--json] [<handoff-file>|<blueprint>|<page>...]
```

The command runs each handoff's page through the pipeline. A handoff tells drafter everything
about one page; [The handoff](docs/handoff.md) describes the file. With `--workflow-run`, drafter
reads the pages from a planner's run file instead.

| Flag | Does |
|---|---|
| `--workflow-run <run-file>` | Reads the handoff from a planner's Workflow run file. |
| `--resume <run-id>` | Continues a stopped run. |
| `--whole` | Drafts each blueprint page whole, whatever its `drafted.lock` entry says. |
| `--judge` | Also runs the configured review seats, the blueprint and consistency reads, and the coherence read. |
| `--dry-run` | Assembles the first stage's requests, prints their sizes and estimated cost, and submits nothing. |
| `--sync` | Sends streamed Messages calls for this run instead of Message Batches. |
| `--json` | Prints the result as JSON. |

A blueprint is a `.blueprint.yaml` or a `.blueprint.md` file, and a page argument maps to its
blueprint: the `.blueprint.md` when that file exists, else the `.blueprint.yaml`. Without
`--judge`, a blueprint page runs the draft seat and the edit seat only: the draft, the gate, one
edit request, and an accept when the gate holds. A section redraft runs its section exchange, the
gate, and the edit pass. With `--judge`, the page also takes the configured `seats`, the blueprint
and consistency reads, and, after a section redraft, the coherence read. [Configuration](docs/configuration.md)
lists which seats run when, and `[editing] preferences` names a file of editing preferences that
the draft and edit prompts carry.

The exit status is 0 when every page is accepted, 1 when any page is escalated, 2 when the command
couldn't start, and 3 when the run stopped and you can resume it. Status 3 outranks 1: a run that
stopped with an escalated page exits 3.

### check

```sh
drafter check [--root <dir>] [--json] [<blueprint>...]
```

The command validates every blueprint under the project's blueprint root, or the ones you name,
offline. It decodes each blueprint, runs the plan checks on it, and adds the blueprint checks:
each managed page has a blueprint, every `[[id]]` reference resolves, every section says why, and
`drafted.lock` agrees with the disk. Findings print as `file:line: severity: check: message
(fix)`, and the exit status follows `check-plan`'s. `drafter check --fact <id>`, repeatable,
verifies only those facts' source pointers and runs no blueprint check. [Blueprint
format](docs/blueprint-format.md) describes the format and each check.

### refs

```sh
drafter refs [--root <dir>] [--json] <fact-id>...
drafter refs [--root <dir>] [--json] --page <path | slug>
```

The command lists every blueprint item, on every page, that places or cites each fact, with the
section it sits in, and the brief entries and drafted pages that cite it. A brief counts for every
page the outline lists, whether or not the page has a blueprint yet. Each fact's line names its
source kinds: `code` for a repository path, `url` for a URL, and `prose` for a `.md` file or a
file under `facts.prose_sources`; `--json` adds them as `source_kinds`. It exits 1 when a named id
is not in the facts store, or, for a single id, when nothing cites it.

With `--page` and no fact ids, the command lists each blueprint `See` or link item and each
outline `crossLinks` entry whose target is that page, as `file:line: kind`, or as an array of
`{file, line, kind}` with `--json`. It exits 0 when nothing links to the page, 1 when the outline
lists no such page, and 2 when `--page` comes with fact ids.

### outline

```sh
drafter outline [--root <dir>] [--json] <slug | page path>
```

The command prints the page's outline entry as YAML, or as a JSON object with `--json`. It exits 1
when the outline lists no such page.

### excerpt

```sh
drafter excerpt [--root <dir>] [--json] [--full] <fact-id | pointer>
```

The command prints the source lines a read gets for a fact: the fact's bullet, then each excerpt
block as a read's request carries it. Given a source pointer instead of a fact id, it prints that
pointer's block. It exits 0 when at least one block shows source lines and 1 when none does. A
fact whose sources are URLs prints each URL with its read date as a checklist and exits 0. Output
stops at 200 lines unless `--full` is set. `drafter --version` prints the build's version.
[Blueprint format](docs/blueprint-format.md#see-what-a-read-sees) lists what each pointer form
yields.

### convert

```sh
drafter convert outline [--out <file>] <outline.json>...
drafter convert plan [--framing <file>] --outline <outline.json> [--root <dir>] <plan.yaml>
```

`convert outline` writes `outline.yaml` from the chain's JSON outlines. `convert plan` writes a
page's blueprint from its v1 plan and framing record, never over an existing file, and seeds the
page's `drafted.lock` entry. It leaves every section's `why` for a person to write.

### read

```sh
drafter read [--resume <run-id>] [--dry-run] [--sync] [--json] <blueprint>...
```

The command runs stage 1 of the pipeline on each blueprint: the local check, then one batch
holding the consistency read and the blueprint read. It needs a clean tree and `[seat.blueprint]`
in the configuration, and it reads every input from the commit the run started at. When the run
ends with every blueprint decided, it records each accepted blueprint's blob id in
`drafted.lock`. For each whole read that blocked, it also records a
[read bar](docs/blueprint-format.md#the-read-bar): the read's blocking findings, which the next
read checks the change against. The exit status is the same as `page`'s.

### check-plan

```sh
drafter check-plan --facts docs/facts --outline docs/outline.json docs/guides/rotate.plan.yaml
```

The command decodes each plan and checks its ids, the order of its steps, the recovery path of
every step that can fail, its links and anchors, and, with `--facts`, whether each fact it cites
exists and is citable. Each finding prints as `file:line: severity: check: message (fix)`. The
exit status is 0 when no finding is an error, 1 when one is, and 2 when the command couldn't run.
[Plan format](docs/plan-format.md) describes every field and check.

### anchors

```sh
drafter anchors docs/reference/keys.md
```

The command prints each anchor the Markdown file offers, one per line: the slug of every
heading, and the `id` of every `<a id="...">` element. Add `--json` for a JSON array.

### check-prompts

```sh
drafter check-prompts --matrix matrix.json
```

The command loads the project's prompts module, renders each seat over a matrix of cases, and
checks every prompt. With `--expect <file>`, it also compares each prompt byte for byte with an
expected rendering. [The prompts module](docs/prompts-module.md) describes the module and the
matrix.

### spend

```sh
drafter spend --since 2026-10-01 --until 2026-10-31
```

The command prints the cost recorded in the ledger, by repo and seat, with a total. Without
`--since` and `--until`, it covers the current billing cycle. Both dates are UTC, and `--until` is
inclusive. A line settled at an estimate is marked, and open reservations print on a line of their
own at their worst case.

### status

```sh
drafter status <run-id>
```

The command prints the status, age, and request counts of each batch the run has submitted and
not yet read. It reads the run's state without taking its lock, so it works while the run is
live. The API moves requests out of processing only when the whole batch ends, so a batch in
progress shows its status and age. A run with no open batch prints `no open batch`. With `--json`,
each batch prints as one object. The exit status is 2 when the run's state cannot be read or the
API cannot answer. A polling `drafter page` or `drafter read` prints the same facts as it waits,
and `--sync` runs attended without a batch wait.

### config

```sh
drafter config
```

The command prints every resolved setting for the project you are in, and the layer, variable, or
flag that set each one. A cap lists every source that set it. The command never prints an API key.
[Configuration reference](docs/configuration.md) describes the layers and every setting.

## Documentation

- [Set up drafter](docs/setup.md): the key source, the Console workspace, and the spending limits.
- [Configuration reference](docs/configuration.md): the three layers, their precedence, and every
  setting.
- [The handoff](docs/handoff.md): the file that tells drafter about a page.
- [The prompts module](docs/prompts-module.md): the contract for the JavaScript that writes each
  seat's prompt.
- [Plan format](docs/plan-format.md): every field and item kind of a page plan, each check, and the
  facts bullet format.
- [Plan schema](schema/plan.schema.json): the JSON Schema for a plan file.
- [Blueprint format](docs/blueprint-format.md): the durable page blueprint, `outline.yaml`,
  `drafted.lock`, the path rule, and the edit catalog.
- [Blueprint schema](schema/blueprint.schema.json) and [outline schema](schema/outline.schema.json):
  the JSON Schemas for those files.

## Develop

`make check` runs `go vet`, `golangci-lint`, and the tests. It needs `node` and
[Vale](https://vale.sh/docs/install) on `PATH`: the gate's tests run Vale and fail when it is
missing. It also runs the docs page chain's Node tests, the prompt parity check between Node and
the built binary, and the install test. The chain's tests read two agent definitions from a workstation checkout. They look in
`~/Projects/workstation` and fail when it is missing, so set `WORKSTATION_DIR` where your checkout
lives. Run `make check` before each commit.

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

Before your first run of `drafter page`, follow [Set up drafter](docs/setup.md). The pipeline
spends money, and it refuses to start until you set a per-page cap and a per-cycle ceiling.

## Commands

Every command prints for a person by default and takes `--json` for scripts. Run
`drafter <command> --help` for every flag.

### page

```sh
drafter page [--workflow-run <run-file>] [--resume <run-id>] [--dry-run] [--sync] [--json] [<handoff-file>...]
```

The command runs each handoff's page through the pipeline. A handoff tells drafter everything
about one page; [The handoff](docs/handoff.md) describes the file. With `--workflow-run`, drafter
reads the pages from a planner's run file instead.

| Flag | Does |
|---|---|
| `--workflow-run <run-file>` | Reads the handoff from a planner's Workflow run file. |
| `--resume <run-id>` | Continues a stopped run. |
| `--dry-run` | Assembles the first stage's requests, prints their sizes and estimated cost, and submits nothing. |
| `--sync` | Sends streamed Messages calls for this run instead of Message Batches. |
| `--json` | Prints the result as JSON. |

The exit status is 0 when every page is accepted, 1 when any page is escalated, 2 when the command
couldn't start, and 3 when the run stopped and you can resume it. Status 3 outranks 1: a run that
stopped with an escalated page exits 3.

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

## Develop

`make check` runs `go vet`, `golangci-lint`, and the tests. Run it before each commit.

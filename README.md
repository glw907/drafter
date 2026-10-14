# drafter

drafter validates documentation page plans and, in later releases, runs the drafting and review
of each planned page as batched calls to the Claude API. A planner, such as a Claude Code
session, writes a page plan in drafter's YAML format. drafter checks the plan before any
drafting happens. The tool is in early development, and only `check-plan` and `anchors` exist so far.

## Install

You need Go. From a checkout of this repository, run:

```sh
make install
```

The command builds the binary and copies it to `~/.local/bin/drafter`. To install somewhere
else, set `PREFIX`, as in `make install PREFIX=/usr/local`. The binary then goes to
`$PREFIX/bin`.

## Check a plan

```sh
drafter check-plan --facts docs/facts --outline docs/outline.json docs/guides/rotate.plan.yaml
```

The command decodes each plan and checks its ids, the order of its steps, the recovery path of
every step that can fail, its links and anchors, and, with `--facts`, whether each fact it cites
exists and is citable. Each finding prints as `file:line: severity: check: message (fix)`. The
exit status is 0 when no finding is an error, 1 when one is, and 2 when the command couldn't run.

Run `drafter check-plan --help` for every flag.

## List a file's anchors

```sh
drafter anchors docs/reference/keys.md
```

The command prints each anchor the Markdown file offers, one per line: the slug of every
heading, and the `id` of every `<a id="...">` element. Add `--json` for a JSON array.

## Reference

- [Plan format](docs/plan-format.md): every field and item kind, each check, the facts bullet
  format, and the exit codes.
- [Plan schema](schema/plan.schema.json): the JSON Schema for a plan file.

## Develop

`make check` runs `go vet`, `golangci-lint`, and the tests. Run it before each commit.

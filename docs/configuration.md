# Configuration reference

drafter reads its settings from three TOML files, the environment, and two command-line flags.
This page describes how those sources combine, what drafter refuses to load, and every setting.

To see what drafter resolved for the project you are in, run `drafter config`. It prints each
setting with the layer, variable, or flag that set it. It never prints an API key.

## The three layers

| Layer | File | Holds |
|---|---|---|
| User | `~/.config/drafter/config.toml` | One user's settings for every project on the machine. |
| Project | `drafter.toml` at the project root, committed | Settings everyone who works on the project shares. |
| Local | `drafter.local.toml` at the project root, ignored by git | One user's settings for one project. |

The project root is the top level of the git worktree. A directory is a drafter project when its
root holds a `drafter.toml`. Outside a project, only the user layer applies.

The user config lives under `$XDG_CONFIG_HOME` when that variable is set. The path above is the
usual default.

## Precedence

A setting resolves from the first source that sets it, in this order:

1. A command-line flag.
2. An environment variable.
3. `drafter.local.toml`.
4. `drafter.toml`.
5. The user config.
6. The built-in default.

A layer overrides key by key. A list, such as `seats` or `seat.<name>.inputs`, replaces the lower
layer's list whole.

Each scalar setting reads an environment variable named `DRAFTER_` plus the key in upper case,
with each dot replaced by `_`. The key `mode` is `DRAFTER_MODE`, and `gate.protocol` is
`DRAFTER_GATE_PROTOCOL`. An empty variable counts as unset. A `DRAFTER_` variable that isn't a
setting's, such as `DRAFTER_API_KEY`, changes nothing here. Tables and lists have no variable.

Only two settings have flags. `--sync` sets `mode` to `sync`, and `--json` sets `output` to `json`.

### Paths

A relative path in a file resolves from the directory that holds the file. A relative path in an
environment variable resolves from the working directory. A leading `~` expands to the home
directory in the user config, `drafter.local.toml`, and the environment, and doesn't expand in
`drafter.toml`.

### The caps

The two caps, `page_cap` and `cycle_ceiling`, don't follow the order above. Each resolves to the
lowest value any source sets, so a layer can tighten a cap and never loosen one. If the user
config sets `page_cap = 10` and `drafter.local.toml` sets `page_cap = 4`, the cap is 4. If the
local file sets 20, the cap stays 10.

`drafter config` lists every source that set a cap, lowest layer first, beside the resolved value.

## What drafter refuses

drafter checks each layer as it loads it and stops at the first problem, naming the file and line.

- An unknown key. Every layer decodes strictly, so a misspelled key is an error and never a
  silent default.
- A key in a layer that may not hold it. Money and credentials never live in the committed
  `drafter.toml`. The error names the key and the files it belongs in. The "Layers" column below
  says where each setting may appear.
- A `drafter.local.toml` that `git check-ignore` doesn't report as ignored. The file can hold a
  key source and caps, so add it to `.gitignore`. A file that git tracks isn't ignored, even when
  `.gitignore` lists it.
- A `drafter.local.toml` with no `drafter.toml` beside it.
- A path in `drafter.toml` that resolves outside the project. This covers `prompts`, `facts.dir`,
  and `seat.<name>.agent`. A machine-specific path belongs in the local or user layer.
- A command template that uses a placeholder other than `{page}`, `{brief}`, `{slug}`, and
  `{baseline}`. A shell `${name}` and an awk `{print $1}` pass, because the check looks only for a
  bare brace pair around an identifier.
- A `gate.lane` when the resolved `gate.protocol` isn't `cairn-run-gate`.
- A value outside its range, such as a `mode` other than `batch` or `sync`, or a `billing_cycle_day`
  outside 1 to 31.

Some problems show up only when a command needs the missing setting. `drafter config` and
`drafter page` require `workspace_id`, `page_cap`, and `cycle_ceiling`. Inside a project, they also
require `gate.command`, `prompts`, and, for each seat that runs, the seat's `model`, `effort`,
`max_tokens`, and a `prices.models` entry for its model. The `consistency` seat is required only
when `facts.dir` is set. drafter lists every missing setting in one error.

## Settings

The tables use these terms. A dotted key is a TOML key inside a table, so `gate.command` is the
`command` key under `[gate]`. The Layers column names the files that may hold the setting: user,
project, local, or all three.

### Money and credentials

| Setting | Layers | Default | Environment variable |
|---|---|---|---|
| `workspace_id` | user, local | None. Required. | `DRAFTER_WORKSPACE_ID` |
| `key_env` | user, local | `DRAFTER_API_KEY` | `DRAFTER_KEY_ENV` |
| `key_command` | user, local | Unset. | `DRAFTER_KEY_COMMAND` |
| `api_base_url` | user, local | `https://api.anthropic.com` | `DRAFTER_API_BASE_URL` |
| `page_cap` | user, local | None. Required. | `DRAFTER_PAGE_CAP` |
| `cycle_ceiling` | user, local | None. Required. | `DRAFTER_CYCLE_CEILING` |
| `billing_cycle_day` | user, local | `1` | `DRAFTER_BILLING_CYCLE_DAY` |
| `state_dir` | user | `~/.local/state/drafter` | `DRAFTER_STATE_DIR` |
| `retention_days` | user, local | `30` | `DRAFTER_RETENTION_DAYS` |
| `inline_files` | user, local | None. | None. |

`workspace_id` is the Console workspace you expect the key to belong to. On its first call of a
run, drafter reads the `anthropic-workspace-id` response header and stops before any further
request when the header differs. See [Set up drafter](setup.md).

`key_env` names the environment variable that holds the API key. `key_command` is a command that
prints the key. When `key_command` is set, drafter runs it through `/bin/sh -c` at the start of
each run and each resume, takes its trimmed standard output, and refuses the run on a nonzero exit
or empty output. Otherwise drafter reads the variable `key_env` names. drafter never prints the
key and never uses OAuth.

The default variable is drafter's own so that an `ANTHROPIC_API_KEY` already in your shell never
bills a run by accident. To use that key, set `key_env = "ANTHROPIC_API_KEY"`.

`api_base_url` is where drafter sends requests, and the key goes there too. It exists for tests
and for proxies that forward to the Claude API.

`page_cap` is the most one page may cost, in US dollars. `cycle_ceiling` is the most all drafter
runs may cost in one billing cycle. Both must be above 0, and both follow the lowest-wins rule.
drafter refuses to start until both are set, so it never spends against an amount you didn't
choose.

`billing_cycle_day` is the day of the month, from 1 to 31, on which the ceiling resets at 00:00
UTC. A day past the end of a month resets on that month's last day.

`state_dir` holds the ledger, the run records, and the run locks. It is a user-only setting
because the ledger enforces the cycle ceiling across every project. A project that moved it would
get a ledger and a ceiling of its own.

`retention_days` is how long drafter keeps the record of a finished run, at least 1. At the start
of each run, drafter deletes run directories older than this whose pages all ended accepted or
escalated. It never deletes a stopped run that you can still resume, and it never prunes the
ledger.

`inline_files` lists files outside the repository that a seat may inline into its request. It is a
list of paths, and each may be a file or a directory. It can send any file on the machine to the
API, so it lives in the user or local layer.

### Prices

The price table is dated, and drafter computes every cost from it. Rates are in US dollars per
million tokens. A seat whose model has no entry in `prices.models` fails validation, so a missing
price can't compute as zero.

| Setting | Layers | Default | Environment variable |
|---|---|---|---|
| `prices.date` | user, local | None. | None. |
| `prices.source` | user, local | None. | None. |
| `prices.batch_discount` | user, local | `0` | None. |
| `prices.models.<model>.input` | user, local | None. | None. |
| `prices.models.<model>.output` | user, local | None. | None. |
| `prices.models.<model>.cache_write_5m` | user, local | None. | None. |
| `prices.models.<model>.cache_write_1h` | user, local | None. | None. |
| `prices.models.<model>.cache_read` | user, local | None. | None. |
| `prices.models.<model>.long_context.threshold` | user, local | None. | None. |
| `prices.models.<model>.long_context.input` | user, local | None. | None. |
| `prices.models.<model>.long_context.output` | user, local | None. | None. |
| `prices.models.<model>.long_context.cache_write_5m` | user, local | None. | None. |
| `prices.models.<model>.long_context.cache_write_1h` | user, local | None. | None. |
| `prices.models.<model>.long_context.cache_read` | user, local | None. | None. |

`prices.date` and `prices.source` record when you copied the rates and from which published
pricing page. drafter doesn't read them for cost, and `drafter config` shows them.

`prices.batch_discount` is the fraction taken off every rate in `batch` mode, from 0 up to but not
including 1. Leaving it unset means no discount, so a missing value overstates cost and never
understates it.

Replace `<model>` with a model ID, such as the one a seat's `model` names. A model's
`long_context` table describes a second rate tier. It applies to a request whose total input
(input, cache-read, and cache-write tokens) is over `threshold` tokens, and its five rates replace
all five of the model's rates.

### Run behavior

| Setting | Layers | Default | Environment variable |
|---|---|---|---|
| `mode` | all | `batch` | `DRAFTER_MODE` |
| `output` | all | `human` | `DRAFTER_OUTPUT` |
| `pages_in_flight` | all | `3` | `DRAFTER_PAGES_IN_FLIGHT` |
| `retries.gate_fix` | all | `2` | `DRAFTER_RETRIES_GATE_FIX` |
| `retries.edit_contract` | all | `2` | `DRAFTER_RETRIES_EDIT_CONTRACT` |

`mode` is `batch` or `sync`. In `batch` mode drafter sends each stage as a Message Batch, which
costs less and takes longer. In `sync` mode it sends streamed Messages calls one at a time. The
flag is `--sync`.

`output` is `human` or `json`, and the flag is `--json`.

`pages_in_flight` is how many pages drafter keeps in the pipeline at once, at least 1.

`retries.gate_fix` and `retries.edit_contract` are the retry budgets, each at least 0. A project
sets how many retries a budget allows. It can't change what counts as an accepted answer, which
is part of drafter's contract.

### The gate

drafter runs the gate command after each write to the project and reads the result.

| Setting | Layers | Default | Environment variable |
|---|---|---|---|
| `gate.command` | project, local | None. Required to run. | `DRAFTER_GATE_COMMAND` |
| `gate.index_link_command` | project, local | Unset. | `DRAFTER_GATE_INDEX_LINK_COMMAND` |
| `gate.protocol` | project, local | `plain` | `DRAFTER_GATE_PROTOCOL` |
| `gate.lane` | project, local | Unset. | `DRAFTER_GATE_LANE` |
| `gate.section_pattern` | project, local | Unset. The whole log is one section. | `DRAFTER_GATE_SECTION_PATTERN` |
| `gate.failure_pattern` | project, local | Unset. | `DRAFTER_GATE_FAILURE_PATTERN` |

`gate.command` and `gate.index_link_command` are command templates. drafter runs each through
`/bin/sh -c` from the project root. The placeholders are `{page}`, `{brief}`, `{slug}`, and
`{baseline}`.

`gate.protocol` is `plain` or `cairn-run-gate`. With `plain`, exit status 0 is a green gate and
anything else is red, and drafter writes the combined output to a log in the run directory. With
`cairn-run-gate`, drafter runs the command through that wrapper and follows its protocol.
`gate.lane` sets the `CAIRN_GATE_LANE` variable for the wrapper, so it requires the
`cairn-run-gate` protocol.

`gate.section_pattern` is a regular expression that opens a new section of the log. It needs one
capture group, which names the section. `gate.failure_pattern` is a regular expression that picks
the failing lines of a red log. drafter uses the two to attribute each failing section to a page.

### Seats and prompts

| Setting | Layers | Default | Environment variable |
|---|---|---|---|
| `prompts` | project, local | None. Required to run. | `DRAFTER_PROMPTS` |
| `seats` | project, local | Every seat. | None. |
| `seat.<name>.agent` | project, local | Unset. | None. |
| `seat.<name>.model` | project, local | None. Required for each seat that runs. | None. |
| `seat.<name>.effort` | project, local | None. Required for each seat that runs. | None. |
| `seat.<name>.max_tokens` | project, local | None. Required for each seat that runs. | None. |
| `seat.<name>.inputs` | project, local | None. | None. |

`prompts` is the path to the prompts module. [The prompts module](prompts-module.md) describes its
contract.

The seven seats are `draft`, `structure`, `editor`, `fact`, `reader`, `coverage`, and
`consistency`. `seats` lists the ones that run. The list must include `draft`, which always runs.
Leaving a review seat out removes its stages, and the run record notes each seat that didn't run.
The `consistency` seat runs only when `facts.dir` is set.

Replace `<name>` with a seat name. A `seat.<name>` table for any other name is an error.

`seat.<name>.agent` is the path to an agent definition file that becomes the seat's system prompt.
`seat.<name>.model` is a model ID, and it needs an entry under `prices.models`.
`seat.<name>.effort` is the effort level drafter sends with each of the seat's requests.
`seat.<name>.max_tokens` is the most output tokens one request may use, at least 1. drafter
reserves the worst case of that many output tokens against the caps before it sends a request.

`seat.<name>.inputs` is a list of command templates. drafter runs each from the project root when
it builds a request for the `draft` seat or a read seat, and includes the command's output in the
request. A template that names `{baseline}` runs only when the page has a baseline to compare with.

### Facts and registers

| Setting | Layers | Default | Environment variable |
|---|---|---|---|
| `facts.dir` | project, local | Unset. No facts store. | `DRAFTER_FACTS_DIR` |
| `facts.bullet_format` | project, local | Unset. | `DRAFTER_FACTS_BULLET_FORMAT` |
| `register.<track>` | project, local | Unset. | None. |

`facts.dir` is the directory of fact files. Setting it turns on the facts store, including the
`consistency` seat. `facts.bullet_format` names the format of the bullets in a fact file. The only
value is `v1`. [Plan format](plan-format.md) describes how a plan cites facts.

`register.<track>` is a list of the register sections that a track reads. Replace `<track>` with
the track's name. drafter records the list and `drafter config` prints it.

## Example

A user config that sets the money and credentials:

```toml
workspace_id = "wrkspc_example"
page_cap = 10
cycle_ceiling = 160
key_env = "DRAFTER_API_KEY"

[prices]
date = "2026-10-01"
source = "https://example.com/pricing"
batch_discount = 0.5

[prices.models.example-model]
input = 5.0
output = 25.0
cache_write_5m = 6.25
cache_write_1h = 10.0
cache_read = 0.5
```

The rates are placeholders. Copy the real ones from the provider's published pricing page.

A committed `drafter.toml` that holds only what the whole project shares:

```toml
prompts = "docs/drafter/prompts.js"
seats = ["draft", "structure", "editor"]

[gate]
command = "make check-docs PAGE={page}"

[seat.draft]
model = "example-model"
effort = "high"
max_tokens = 64000

[seat.structure]
model = "example-model"
effort = "medium"
max_tokens = 32000

[seat.editor]
model = "example-model"
effort = "medium"
max_tokens = 32000
inputs = ["vale {page}"]
```

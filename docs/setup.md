# Set up drafter

drafter spends money on the Claude API. Before your first run, decide where the API key comes
from, give drafter a workspace and a key of its own, and choose the limits that stop a run from
spending more than you intend.

drafter enforces two caps itself, and the Console can enforce a third limit on the server. This
page covers each.

## Choose the key source

drafter takes its API key from one of two sources. Both are settings in your user config or in
`drafter.local.toml`. Neither may appear in a committed `drafter.toml`.

- `key_command` is a command that prints the key. drafter runs it through `/bin/sh -c` at the
  start of each run and each resume, and it takes the trimmed standard output. A nonzero exit or
  empty output refuses the run, and drafter never logs the output. Use this source to read the key
  from a password manager or a secrets store.
- Otherwise, drafter reads the environment variable that `key_env` names. The default name is
  `DRAFTER_API_KEY`.

The default is a variable of drafter's own so that an `ANTHROPIC_API_KEY` already in your shell
can't bill drafter's runs by accident. To use that key anyway, set `key_env` to
`"ANTHROPIC_API_KEY"`.

drafter never uses OAuth, so it can't bill a Claude subscription. It builds its client without the
SDK's usual credential chain, so `ANTHROPIC_AUTH_TOKEN`, profiles, and `ANTHROPIC_BASE_URL` change
nothing. Set `api_base_url` to point drafter at a proxy.

drafter doesn't care where you keep the key. It refuses to start when the source it resolves
yields no key.

## Create a Console workspace and a key

Use a workspace of its own in the Claude Console, and a key that belongs to that workspace.

1. In the Console, create a workspace for drafter.
2. In that workspace, create an API key.
3. Store the key where your key source reads it.
4. Copy the workspace's ID into the `workspace_id` setting.

On the first call of each run, drafter reads the `anthropic-workspace-id` response header and
stops before any further request when it differs from `workspace_id`. A key that belongs to the
wrong workspace therefore can't spend anything.

A workspace of its own also keeps drafter's batches in a short list. When a run stops between
creating a batch and recording its ID, `--resume` lists the workspace's batches to find the one it
created.

## Set the caps

drafter refuses to start until you set both caps. Put them in your user config or in
`drafter.local.toml`:

```toml
page_cap = 10
cycle_ceiling = 160
```

`page_cap` is the most one page may cost, in US dollars, and `cycle_ceiling` is the most all runs
may cost in one billing cycle. Before each submission, drafter reserves the stage's worst case:
the counted input plus every request's `max_tokens` at the output price. A stage that would cross
the page cap escalates its page. A stage that would cross the ceiling stops the run, and you can
resume it.

Set `billing_cycle_day` to the day your credit cycle starts. The ceiling resets at 00:00 UTC on
that day. See [Configuration reference](configuration.md) for how the caps combine across layers.

## Set a spend limit in the Console

A workspace spend limit is a server-side backstop. It works even if drafter has a bug, and you
can set it only in the Console, under Settings, Workspaces, Spend limits.

The limit covers a calendar month, while drafter's ceiling follows your billing cycle. That
mismatch is a trade, and the amount is your decision:

- A limit equal to `cycle_ceiling` is a coarse guard. A billing cycle that spans two calendar
  months can spend more than the limit allows in total, because each month gets its own limit.
- A lower limit is tighter, but it caps what drafter can spend in any one month, which may be less
  than one cycle's ceiling.

The Batch API's documentation warns that batches may go slightly over a workspace's configured
spend limit, so treat the backstop as coarse. When the API reports that a workspace limit has been
reached, drafter stops the run, resumable, and prints the limit's message. It escalates no page.

## Check the organization's billing

If you want your prepaid credit to be the hard stop, confirm that the API organization has no
payment method on file and no auto-reload turned on. With either one, the organization can keep
spending after the credit runs out, and neither drafter's caps nor the workspace limit guards the
organization's balance.

## Upgrade drafter

Finish or resume every run before you upgrade drafter. A run's state carries a schema version and
the drafter version that wrote it. A drafter whose state schema differs refuses to resume an older
run, with a message that says so.

`drafter spend` shows open reservations, which a stopped run leaves behind. To continue a stopped
run, pass its run ID:

```sh
drafter page --resume <run-id>
```

A resume reads the caps, the key, and `workspace_id` fresh, so a cap you tightened since the stop
applies at once. It uses the prompts module, the agent definitions, and the configuration that the
run copied at its start.

## Check your setup

Run `drafter config` from the project. It prints every resolved setting and where it came from,
and it exits nonzero when a layer is refused or a setting the command needs is missing. A correct
setup prints the caps and the workspace ID and exits 0.

Run `drafter page --dry-run` on a handoff to see the first stage's request sizes and estimated
cost without submitting anything.

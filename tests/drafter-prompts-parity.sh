#!/usr/bin/env bash
# tests/drafter-prompts-parity.sh: proves drafter renders the docs page chain's back-half prompts
# as Node does. Node renders every case of tests/fixtures/drafter/matrix.json
# (tests/drafter-render-prompts.mjs), and `drafter check-prompts` renders the same cases in goja
# and requires each prompt to equal Node's byte for byte.
# DRAFTER_BIN names the drafter binary; it defaults to `drafter` on PATH.
set -uo pipefail

cd "$(dirname "$0")/.."
bin="${DRAFTER_BIN:-drafter}"
if ! command -v "$bin" >/dev/null 2>&1; then
    echo "drafter-prompts-parity: no drafter binary at '$bin'; set DRAFTER_BIN to a build with check-prompts" >&2
    exit 1
fi

node tests/drafter-render-prompts.mjs | "$bin" check-prompts \
    --module claude/.claude/workflows/docs-page-chain.js \
    --matrix tests/fixtures/drafter/matrix.json \
    --expect -

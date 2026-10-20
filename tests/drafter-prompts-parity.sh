#!/usr/bin/env bash
# tests/drafter-prompts-parity.sh: proves drafter renders the docs page chain's back-half prompts
# as Node does, in both of the chain's modes. Node renders every case of a matrix
# (tests/drafter-render-prompts.mjs), and `drafter check-prompts` renders the same cases in goja
# and requires each prompt to equal Node's byte for byte. tests/fixtures/drafter/matrix.json holds
# the handoff-mode cases and tests/fixtures/drafter/blueprint-matrix.json the blueprint-mode ones.
# DRAFTER_BIN names the drafter binary; it defaults to `drafter` on PATH.
set -uo pipefail

cd "$(dirname "$0")/.."
bin="${DRAFTER_BIN:-drafter}"
if ! command -v "$bin" >/dev/null 2>&1; then
    echo "drafter-prompts-parity: no drafter binary at '$bin'; set DRAFTER_BIN to a build with check-prompts" >&2
    exit 1
fi

status=0
for matrix in tests/fixtures/drafter/matrix.json tests/fixtures/drafter/blueprint-matrix.json; do
    echo "drafter-prompts-parity: $matrix"
    node tests/drafter-render-prompts.mjs "$matrix" | "$bin" check-prompts \
        --module claude/.claude/workflows/docs-page-chain.js \
        --matrix "$matrix" \
        --expect - || status=1
done
exit "$status"

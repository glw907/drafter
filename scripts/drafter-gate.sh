#!/usr/bin/env bash
# scripts/drafter-gate.sh: run drafter's `make check` through cairn-run-gate on the light lane.
# Shaped like scripts/gate.sh: a worktree-isolated session's Bash guard refuses
# `cairn-run-gate '<string>'` and accepts a no-argument wrapper. The checkout is DRAFTER_DIR
# (default ~/Projects/drafter). The gate's tree fingerprint covers only this repo, so a change
# in drafter alone would replay the last finished run; --fresh reruns every time. Exit codes
# are cairn-run-gate's, so exit 75 still means re-issue. It repeats the `gate exit:` line last.
set -uo pipefail
cd "$(dirname "$0")/.."
export CAIRN_GATE_LANE=light
dir=${DRAFTER_DIR:-$HOME/Projects/drafter}
out=$(cairn-run-gate --fresh "make -C $dir check" 2>&1)
rc=$?
printf '%s\n' "$out"
printf '%s\n' "$out" | grep -m1 '^gate exit:' || true
exit "$rc"

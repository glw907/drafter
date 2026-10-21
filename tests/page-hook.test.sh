#!/usr/bin/env bash
# tests/page-hook.test.sh: proves drafter-page-hook adds context for a page that has a blueprint and
# stays silent, with exit 0, in every other case. It builds a synthetic repo and never reads a real one.
set -uo pipefail

cd "$(dirname "$0")/.."
hook=$PWD/bin/.local/bin/drafter-page-hook

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fail=0

# A fixture repo with one managed page and one unmanaged page, and a symlink to its parent.
repo=$tmp/real/repo
mkdir -p "$repo/docs/extend" "$repo/blueprints/docs/extend"
git -C "$repo" init -q
echo page >"$repo/docs/extend/rotate-the-key.md"
echo page >"$repo/docs/extend/notes.md"
echo "page: rotate" >"$repo/blueprints/docs/extend/rotate-the-key.blueprint.yaml"
echo page >"$repo/docs/extend/debug.md"
echo page >"$repo/blueprints/docs/extend/debug.blueprint.yaml"
printf -- '---\npage: Debug\n---\n' >"$repo/blueprints/docs/extend/debug.blueprint.md"
ln -s "$tmp/real" "$tmp/link"

# A repo with no blueprints root.
plain=$tmp/plain
mkdir -p "$plain/docs"
git -C "$plain" init -q
echo page >"$plain/docs/a.md"

payload() { printf '{"tool_name":"Edit","tool_input":{"file_path":"%s"}}' "$1"; }

# expect_context NAME PATH: the hook exits 0 and prints context naming the blueprint.
expect_context() {
    local name=$1 path=$2 out
    out=$(payload "$path" | "$hook") || { echo "FAIL: $name: exit $?" >&2; fail=1; return; }
    if ! jq -e '.hookSpecificOutput.hookEventName == "PreToolUse" and (.hookSpecificOutput.additionalContext | contains("blueprints/docs/extend/rotate-the-key.blueprint.yaml"))' <<<"$out" >/dev/null; then
        echo "FAIL: $name: output was: $out" >&2
        fail=1
    fi
}

# expect_silent NAME PAYLOAD [PATH-ENV]: the hook exits 0 with no output.
expect_silent() {
    local name=$1 body=$2 out status
    shift 2
    out=$(printf '%s' "$body" | env "$@" "$hook")
    status=$?
    if [ "$status" -ne 0 ] || [ -n "$out" ]; then
        echo "FAIL: $name: exit $status, output: $out" >&2
        fail=1
    fi
}

expect_context "managed page" "$repo/docs/extend/rotate-the-key.md"
expect_context "managed page through a symlinked parent" "$tmp/link/repo/docs/extend/rotate-the-key.md"

out=$(payload "$repo/docs/extend/debug.md" | "$hook")
if ! jq -e '.hookSpecificOutput.additionalContext | contains("blueprints/docs/extend/debug.blueprint.md")' <<<"$out" >/dev/null; then
    echo "FAIL: a markdown blueprint wins over a YAML one: output was: $out" >&2
    fail=1
fi

expect_silent "unmanaged page" "$(payload "$repo/docs/extend/notes.md")" HOME="$HOME"
expect_silent "non-md file" "$(payload "$repo/docs/extend/rotate-the-key.txt")" HOME="$HOME"
expect_silent "repo without a blueprints root" "$(payload "$plain/docs/a.md")" HOME="$HOME"
expect_silent "path outside any repo" "$(payload "$tmp/nowhere.md")" HOME="$HOME"
expect_silent "new file in a missing directory" "$(payload "$repo/docs/new/dir/page.md")" HOME="$HOME"
expect_silent "malformed payload" 'not json {' HOME="$HOME"
expect_silent "payload without a path" '{"tool_input":{}}' HOME="$HOME"

# A PATH with no jq: the hook must still exit 0 and print nothing. bash and git come from their own dirs.
nojq=$tmp/nojq
mkdir -p "$nojq"
ln -s "$(command -v git)" "$nojq/git"
ln -s "$(command -v dirname)" "$nojq/dirname"
ln -s "$(command -v bash)" "$nojq/bash"
expect_silent "no jq on PATH" "$(payload "$repo/docs/extend/rotate-the-key.md")" PATH="$nojq"

exit "$fail"

#!/usr/bin/env bash
# tests/install.test.sh: proves `make install` links the docs page chain workflow and its outline
# helper into a temp HOME, and refuses, leaving the tree it would have written through untouched,
# when ~/.claude/workflows is a symlink (stow's folded form). It never touches the real HOME.
set -uo pipefail

cd "$(dirname "$0")/.."
repo=$PWD
# Resolved before HOME changes, so the temp HOME does not rebuild the module and build caches.
export GOCACHE=$(go env GOCACHE)
export GOMODCACHE=$(go env GOMODCACHE)

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fail=0

check() {
    if ! "$@"; then
        echo "FAIL: $*" >&2
        fail=1
    fi
}

under_repo() {
    case "$(realpath "$1")" in
    "$repo"/*) return 0 ;;
    *) return 1 ;;
    esac
}

# A plain HOME: both links land under the checkout, and a second run changes nothing.
home=$tmp/plain
mkdir -p "$home"
check env HOME="$home" make -s install
check under_repo "$home/.claude/workflows/docs-page-chain.js"
check under_repo "$home/.local/bin/cairn-docs-outline"
check test -x "$home/.local/bin/drafter"
check test -d "$home/.claude/workflows" -a ! -L "$home/.claude/workflows"
check env HOME="$home" make -s install

# A folded workflows directory: install must fail and leave the fixture tree byte for byte.
home=$tmp/folded
dotfiles=$tmp/dotfiles
mkdir -p "$home/.claude" "$dotfiles/claude/.claude/workflows"
echo keep >"$dotfiles/claude/.claude/workflows/other.js"
ln -s "$dotfiles/claude/.claude/workflows" "$home/.claude/workflows"
before=$(find "$dotfiles" -type f -print0 | sort -z | xargs -0 sha256sum)
if env HOME="$home" make -s install 2>"$tmp/folded.err"; then
    echo "FAIL: install succeeded through a folded workflows link" >&2
    fail=1
fi
after=$(find "$dotfiles" -type f -print0 | sort -z | xargs -0 sha256sum)
check test "$before" = "$after"
check grep -q 'workflows' "$tmp/folded.err"
check test ! -e "$dotfiles/claude/.claude/workflows/docs-page-chain.js"

if [ "$fail" -ne 0 ]; then
    exit 1
fi
echo "install: ok"

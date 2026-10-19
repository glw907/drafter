# Tally facts

- `t-state` Tally keeps its count in a file named .tally in the current directory, held in `stateFile`. Source: `src/state.go:12-13`. [verified]
- `t-count` The command `tally count` adds one for each item you name. The flag --by sets the amount, and the default is 1. Source: `src/count.go:33-39`. [verified]
- `t-limit` Tally stores at most 1,284 items and needs version 2.4.1 or later. Source: `src/limit.go:5`. [verified]
- `t-date` Support for the old format ended on 2026-03-14. Source: `docs/notes.md`. [external: Example]
- `t-url` The release notes are at https://example.com/tally/notes. Source: `docs/notes.md`. [vendor]
- `t-npm` Install the package with npm install tally-cli. Source: `docs/install.md`. [verified]
- `t-cand` Tally can sync to a server. Source: `src/sync.go`. [candidate]
- `t-rej` Tally encrypts the file. Source: `src/enc.go`. [rejected]
- `t-drift` Tally keeps ten backups. Source: `src/backup.go`. [docs-drift]
- `t-notag` Tally prints a banner. Source: `src/banner.go`.
- `t-twice` Tally has a first definition. Source: `src/a.go`. [verified]
- `t-twice` Tally has a second definition. Source: `src/b.go`. [verified]

## Provenance

- `t-skipped` This bullet sits under a heading the reader skips. [verified]

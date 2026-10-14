# drafter

A Go tool that validates documentation page plans and, in later releases, drafts and reviews pages
through the Claude API.

- `make check` is the gate before every commit: vet, lint, and the full test suite.
- Write Go to the `go-conventions` skill: stdlib assertions only, table-driven tests, errors
  returned and printed only in `main`.
- Fixtures are synthetic. The repo is public, so no private path, account detail, or key belongs in it.
- `make smoke` runs the live smoke against the real API with the user key and spends money; it is kept out of `make check`.

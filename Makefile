BINARY := drafter
PREFIX ?= $(HOME)/.local
CLAUDE_DIR ?= $(HOME)/.claude

.PHONY: build test vet lint chain-test prompts-parity install-test hook-test install check smoke live clean

build:
	go build -o $(BINARY) ./cmd/drafter

test:
	go test ./...

vet:
	go vet ./...
	go vet -tags smoke ./...
	go vet -tags live ./...

lint:
	golangci-lint run ./...

# The Node tests read the agent definitions from a workstation checkout (WORKSTATION_DIR); the last
# one runs the built binary.
chain-test: build
	node tests/docs-page-chain-derivation.test.mjs
	node tests/docs-page-chain-outline.test.mjs
	node tests/docs-page-chain-handoff.test.mjs
	DRAFTER_BIN=./$(BINARY) node tests/docs-page-chain-blueprint.test.mjs

# Node renders the chain's prompts and the built binary renders them in goja; they must match.
prompts-parity: build
	DRAFTER_BIN=./$(BINARY) bash tests/drafter-prompts-parity.sh

install-test:
	bash tests/install.test.sh

hook-test:
	bash tests/page-hook.test.sh

# A symlinked destination directory (stow folds ~/.claude/workflows into its package) would send
# the links into that package's tree, so install refuses before it writes anything.
install: build
	@for d in $(CLAUDE_DIR)/workflows $(CLAUDE_DIR)/skills $(PREFIX)/bin; do \
		if [ -L "$$d" ]; then \
			echo "install: $$d is a symlink; replace it with a real directory first" >&2; \
			exit 1; \
		fi; \
	done
	install -d $(PREFIX)/bin
	install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)
	ln -sfn $(CURDIR)/bin/.local/bin/cairn-docs-outline $(PREFIX)/bin/cairn-docs-outline
	ln -sfn $(CURDIR)/bin/.local/bin/drafter-page-hook $(PREFIX)/bin/drafter-page-hook
	install -d $(CLAUDE_DIR)/workflows
	ln -sfn $(CURDIR)/claude/.claude/workflows/docs-page-chain.js $(CLAUDE_DIR)/workflows/docs-page-chain.js
	install -d $(CLAUDE_DIR)/skills
	ln -sfn $(CURDIR)/claude/.claude/skills/blueprint $(CLAUDE_DIR)/skills/blueprint

check: vet lint test chain-test prompts-parity install-test hook-test

# smoke spends real money; it is never part of check.
smoke:
	go test -tags smoke -count=1 -timeout 2h -run Live ./internal/smoke/

# live drafts the fixture page twice against the real API and spends money; it is never part of
# check. DRAFTER_LIVE_BASELINE is the meter's total at launch (drafter spend --since 2026-10-19
# --json). A skipped test prints "--- SKIP" under -v while go test still prints ok, so the target
# fails on one.
live:
	@out=$$(DRAFTER_LIVE=1 go test -tags live -count=1 -v -timeout 3h -run Live ./e2e/ 2>&1); rc=$$?; \
	printf '%s\n' "$$out"; \
	if [ $$rc -ne 0 ]; then exit $$rc; fi; \
	if printf '%s\n' "$$out" | grep -q -e '--- SKIP'; then echo "live: a test skipped" >&2; exit 1; fi

clean:
	rm -f $(BINARY)

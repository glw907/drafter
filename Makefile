BINARY := drafter
PREFIX ?= $(HOME)/.local
CLAUDE_DIR ?= $(HOME)/.claude

.PHONY: build test vet lint chain-test prompts-parity install-test install check smoke clean

build:
	go build -o $(BINARY) ./cmd/drafter

test:
	go test ./...

vet:
	go vet ./...
	go vet -tags smoke ./...

lint:
	golangci-lint run ./...

# The three Node tests read the agent definitions from a workstation checkout (WORKSTATION_DIR).
chain-test:
	node tests/docs-page-chain-derivation.test.mjs
	node tests/docs-page-chain-outline.test.mjs
	node tests/docs-page-chain-handoff.test.mjs

# Node renders the chain's prompts and the built binary renders them in goja; they must match.
prompts-parity: build
	DRAFTER_BIN=./$(BINARY) bash tests/drafter-prompts-parity.sh

install-test:
	bash tests/install.test.sh

# A symlinked destination directory (stow folds ~/.claude/workflows into its package) would send
# the links into that package's tree, so install refuses before it writes anything.
install: build
	@for d in $(CLAUDE_DIR)/workflows $(PREFIX)/bin; do \
		if [ -L "$$d" ]; then \
			echo "install: $$d is a symlink; replace it with a real directory first" >&2; \
			exit 1; \
		fi; \
	done
	install -d $(PREFIX)/bin
	install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)
	ln -sfn $(CURDIR)/bin/.local/bin/cairn-docs-outline $(PREFIX)/bin/cairn-docs-outline
	install -d $(CLAUDE_DIR)/workflows
	ln -sfn $(CURDIR)/claude/.claude/workflows/docs-page-chain.js $(CLAUDE_DIR)/workflows/docs-page-chain.js

check: vet lint test chain-test prompts-parity install-test

# smoke spends real money; it is never part of check.
smoke:
	go test -tags smoke -count=1 -timeout 2h -run Live ./internal/smoke/

clean:
	rm -f $(BINARY)

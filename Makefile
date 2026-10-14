BINARY := drafter
PREFIX ?= $(HOME)/.local

.PHONY: build test vet lint install check smoke clean

build:
	go build -o $(BINARY) ./cmd/drafter

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

install: build
	install -d $(PREFIX)/bin
	install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)

check: vet lint test

# smoke spends real money; it is never part of check.
smoke:
	go test -tags smoke -count=1 -timeout 2h -run Live ./internal/smoke/

clean:
	rm -f $(BINARY)

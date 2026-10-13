BINARY := drafter
PREFIX ?= $(HOME)/.local

.PHONY: build test vet lint install check clean

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

clean:
	rm -f $(BINARY)

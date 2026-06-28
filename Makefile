# One binary, one command: sm2. The agent is an internal subcommand the CLI
# auto-spawns; there is no separate daemon binary.
BINARY  := sm2
PKG     := ./cmd/sm2
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0-dev")
LDFLAGS := -X 'github.com/abdorizak/sm2/internal/cli.version=$(VERSION)'

.PHONY: build install run test test-cli test-all vet fmt tidy clean help

## build: compile sm2 into ./bin
build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(PKG)

## install: install sm2 into $GOBIN (or $GOPATH/bin) so it is on your PATH
install:
	go install -ldflags "$(LDFLAGS)" $(PKG)

## run: build then run sm2
run: build
	./bin/$(BINARY)

## test: run Go unit tests
test:
	go test ./...

## test-cli: build, then run the end-to-end CLI smoke test
test-cli: build
	./scripts/e2e.sh

## test-all: unit tests + end-to-end CLI test
test-all: test test-cli

## vet: run go vet
vet:
	go vet ./...

## fmt: format all Go code
fmt:
	go fmt ./...

## tidy: tidy go.mod / go.sum
tidy:
	go mod tidy

## clean: remove build artifacts
clean:
	rm -rf bin

## help: list available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'

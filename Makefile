# Three command names, one program: runix (full) plus rx and sp (short).
CMDS    := runix rx sp
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0-dev")
LDFLAGS := -X 'github.com/abdorizak/runix/internal/cli.version=$(VERSION)'

.PHONY: build install run test test-cli test-all vet fmt tidy clean help

## build: compile runix, rx and sp into ./bin
build:
	@for c in $(CMDS); do go build -ldflags "$(LDFLAGS)" -o bin/$$c ./cmd/$$c; done

## install: install runix, rx and sp into $GOBIN (or $GOPATH/bin)
install:
	@for c in $(CMDS); do go install -ldflags "$(LDFLAGS)" ./cmd/$$c; done

## run: build then run runix
run: build
	./bin/runix

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

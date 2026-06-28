# One binary, one command: sm2. The agent is an internal subcommand the CLI
# auto-spawns; there is no separate daemon binary.
BINARY  := sm2
PKG     := ./cmd/sm2
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0-dev")
LDFLAGS := -X 'github.com/abdorizak/sm2/internal/cli.version=$(VERSION)'

# Release targets (Unix only — sm2 uses process groups, signals & unix sockets).
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: build install run test test-cli test-all vet fmt tidy clean dist help

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

## dist: cross-compile static release archives + checksums into ./dist
dist:
	@rm -rf dist && mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		name="sm2_$(VERSION)_$${os}_$${arch}"; \
		echo "  $$name"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "-s -w $(LDFLAGS)" -o dist/sm2 $(PKG) || exit 1; \
		cp LICENSE README.md dist/; \
		tar -C dist -czf "dist/$$name.tar.gz" sm2 LICENSE README.md; \
		rm -f dist/sm2; \
	done
	@rm -f dist/LICENSE dist/README.md
	@cd dist && shasum -a 256 *.tar.gz > SHA256SUMS
	@echo "Done. Upload with: gh release upload $(VERSION) dist/*.tar.gz dist/SHA256SUMS"

## clean: remove build artifacts
clean:
	rm -rf bin dist

## help: list available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'

# ── Castle CLI Makefile ────────────────────────────────────────────────────────
BINARY     := bin/castle
MODULE     := github.com/manuel-garcia-gomez/castle-cli
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
  -X '$(MODULE)/cmd.Version=$(VERSION)' \
  -X '$(MODULE)/cmd.Commit=$(COMMIT)'   \
  -X '$(MODULE)/cmd.BuildDate=$(BUILD_DATE)'

.PHONY: all build clean test lint vet

all: build

## build: Compile the castle binary to ./bin/castle
build:
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

## clean: Remove the compiled binary
clean:
	@rm -rf bin/

## test: Run the full test suite
test:
	go test -race -coverprofile=coverage.out ./...

## vet: Run go vet
vet:
	go vet ./...

## lint: Run staticcheck (install: go install honnef.co/go/tools/cmd/staticcheck@latest)
lint:
	staticcheck ./...

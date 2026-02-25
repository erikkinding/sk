BINARY     := sk
MODULE     := $(shell go list -m)
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS    := -ldflags "-X main.version=$(VERSION)"

.PHONY: all build install test test-integration lint vet clean help

all: build

## build: compile the binary into ./sk
build:
	go build $(LDFLAGS) -o $(BINARY) .

## install: install the binary to $GOPATH/bin
install:
	go install $(LDFLAGS) .

## test: run unit tests (no Docker required)
test:
	go test -v -count=1 -race ./...

## test-integration: run all tests including integration tests (requires Docker)
test-integration:
	go test -tags integration -v -count=1 -race -timeout 10m ./...

## lint: run golangci-lint (install from https://golangci-lint.run)
lint:
	golangci-lint run ./...

## vet: run go vet
vet:
	go vet ./...

## clean: remove build artefacts
clean:
	rm -f $(BINARY)

## help: print this help message
help:
	@echo "Usage: make <target>"
	@echo ""
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/  /'

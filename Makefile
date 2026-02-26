BINARY     := sk
MODULE     := $(shell go list -m)
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS    := -ldflags "-X main.version=$(VERSION)"

.PHONY: all build install test test-integration lint vet clean release help

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

## release: trigger the GitHub release workflow (bumps patch, or minor for feat commits)
release:
	@git pull
	@LATEST=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	VERSION=$${LATEST#v}; \
	MAJOR=$$(echo "$$VERSION" | cut -d. -f1); \
	MINOR=$$(echo "$$VERSION" | cut -d. -f2); \
	PATCH=$$(echo "$$VERSION" | cut -d. -f3); \
	COMMIT_MSG=$$(git log -1 --format="%s"); \
	if echo "$$COMMIT_MSG" | grep -q "^feat"; then \
		MINOR=$$((MINOR + 1)); PATCH=0; \
	else \
		PATCH=$$((PATCH + 1)); \
	fi; \
	SUGGESTED="$$MAJOR.$$MINOR.$$PATCH"; \
	echo "Latest tag : $$LATEST"; \
	echo "Last commit: $$COMMIT_MSG"; \
	echo "Suggested  : v$$SUGGESTED"; \
	printf "Version to release [$$SUGGESTED]: "; \
	read INPUT; \
	RELEASE=$${INPUT:-$$SUGGESTED}; \
	RELEASE=$${RELEASE#v}; \
	echo "Triggering release workflow for v$$RELEASE …"; \
	gh workflow run release.yml --field version=$$RELEASE

## clean: remove build artefacts
clean:
	rm -f $(BINARY)

## help: print this help message
help:
	@echo "Usage: make <target>"
	@echo ""
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/  /'

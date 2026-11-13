# Origin: nodelogicau/intentions-cli Makefile @ b72a395454aa (binary and module renamed; skill and bundle targets dropped)
BINARY  := episodes
MODULE  := github.com/nodelogicau/episodes-cli
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(MODULE)/internal/cli.version=$(VERSION)
DIST    := dist

export CGO_ENABLED := 0

.PHONY: all build test lint fmt vet cross clean tidy

all: build

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY) ./cmd/episodes

test:
	go test ./... -count=1

fmt:
	gofmt -l -w .

vet:
	go vet ./...

lint: vet
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed; ran go vet only"

tidy:
	go mod tidy

# Cross-compile matrix matching the release targets.
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64
cross:
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; \
		[ "$$os" = "windows" ] && ext=".exe"; \
		out=$(DIST)/$(BINARY)_$${os}_$${arch}$$ext; \
		echo "  $$out"; \
		GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o $$out ./cmd/episodes || exit 1; \
	done

clean:
	rm -rf $(DIST)

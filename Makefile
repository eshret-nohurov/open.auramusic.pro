VERSION := $(shell git describe --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
ARCH ?= amd64

.PHONY: dev test lint build build-linux

# Site on :3310 against the backend stub on :3399.
dev:
	@go build -o bin/bm-open ./cmd/bm-open
	@trap 'kill 0' INT TERM EXIT; \
	./bin/bm-open mock & \
	MODE=mock ./bin/bm-open serve

test:
	go test -race -count=1 ./...

lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...
	@if command -v staticcheck >/dev/null; then staticcheck ./...; fi

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/bm-open ./cmd/bm-open

# For a server without Go: make build-linux ARCH=arm64 if uname -m says aarch64.
build-linux:
	GOOS=linux GOARCH=$(ARCH) CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/bm-open ./cmd/bm-open

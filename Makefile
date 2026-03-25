VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GO_BUILD_FLAGS := -trimpath -buildvcs=false
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
RELEASE_DIST ?= dist/release

.PHONY: all cliphub clipd tailclip test test-race lint clean release release-verify

all: cliphub clipd tailclip

cliphub:
	go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o bin/cliphub ./cmd/cliphub

clipd:
	go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o bin/clipd ./cmd/clipd

tailclip:
	go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o bin/tailclip ./cmd/tailclip

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	go vet ./...

clean:
	rm -rf bin/ dist/

# Deterministic release archives, checksums, notes, and manifest.
release:
	go run ./cmd/releasectl build --version $(VERSION) --dist $(RELEASE_DIST)

release-verify:
	go run ./cmd/releasectl verify --version $(VERSION) --dist $(RELEASE_DIST)

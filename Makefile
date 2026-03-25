VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GO_BUILD_FLAGS := -trimpath -buildvcs=false
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
RELEASE_DIST ?= dist/release
RELEASE_MANIFEST ?=
RELEASE_CHECKSUMS ?=
RELEASE_ASSET_BASE_URL ?=

.PHONY: all cliphub clipd tailclip test test-race lint clean release release-verify release-package-managers release-package-managers-verify

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

release-package-managers:
	go run ./cmd/releasectl package-managers build --version $(VERSION) --dist $(RELEASE_DIST) $(if $(RELEASE_MANIFEST),--manifest $(RELEASE_MANIFEST),) $(if $(RELEASE_CHECKSUMS),--checksums $(RELEASE_CHECKSUMS),) $(if $(RELEASE_ASSET_BASE_URL),--release-asset-base $(RELEASE_ASSET_BASE_URL),)

release-package-managers-verify:
	go run ./cmd/releasectl package-managers verify --version $(VERSION) --dist $(RELEASE_DIST) $(if $(RELEASE_MANIFEST),--manifest $(RELEASE_MANIFEST),) $(if $(RELEASE_CHECKSUMS),--checksums $(RELEASE_CHECKSUMS),) $(if $(RELEASE_ASSET_BASE_URL),--release-asset-base $(RELEASE_ASSET_BASE_URL),)

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: all cliphub clipd tailclip test test-race lint clean

all: cliphub clipd tailclip

cliphub:
	go build $(LDFLAGS) -o bin/cliphub ./cmd/cliphub

clipd:
	go build $(LDFLAGS) -o bin/clipd ./cmd/clipd

tailclip:
	go build $(LDFLAGS) -o bin/tailclip ./cmd/tailclip

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	go vet ./...

clean:
	rm -rf bin/

# Cross-compilation for desktop agents + CLI
release:
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o bin/clipd-darwin-amd64       ./cmd/clipd
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o bin/clipd-darwin-arm64       ./cmd/clipd
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o bin/clipd-linux-amd64        ./cmd/clipd
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/clipd-windows-amd64.exe  ./cmd/clipd
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o bin/tailclip-darwin-amd64    ./cmd/tailclip
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o bin/tailclip-darwin-arm64    ./cmd/tailclip
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o bin/tailclip-linux-amd64     ./cmd/tailclip
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/tailclip-windows-amd64.exe ./cmd/tailclip
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o bin/cliphub-linux-amd64      ./cmd/cliphub

BINARY  := aic
PKG     := github.com/moluuser/aic
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(PKG)/internal/app.Version=$(VERSION)

.PHONY: all build install test vet clean dist

# Build for the host architecture.
build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

# Install into $GOBIN (or $GOPATH/bin).
install:
	go install -ldflags "$(LDFLAGS)" .

test:
	go test ./...

vet:
	go vet ./...

# Cross-compile macOS binaries for both architectures and build a universal
# (fat) binary via lipo. Requires Xcode command line tools for lipo.
dist:
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-darwin-amd64 .
	lipo -create -output bin/$(BINARY)-darwin-universal \
		bin/$(BINARY)-darwin-arm64 bin/$(BINARY)-darwin-amd64
	@echo "built bin/$(BINARY)-darwin-{arm64,amd64,universal}"

clean:
	rm -rf bin

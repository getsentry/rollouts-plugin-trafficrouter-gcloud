BINARY_NAME ?= rollouts-plugin-trafficrouter-gcloud
VERSION_PKG := github.com/argoproj-labs/rollouts-plugin-trafficrouter-gcloud/pkg/version
GIT_COMMIT := $(shell git rev-parse HEAD 2>/dev/null)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)
LDFLAGS := -X $(VERSION_PKG).GitCommit=$(GIT_COMMIT)
ifneq ($(VERSION),)
LDFLAGS += -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).VersionPrerelease=
endif

# Platforms to cross-compile for release. Format: os/arch
PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY_NAME) .

.PHONY: build-linux-amd64
build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY_NAME)-linux-amd64 .

.PHONY: build-linux-arm64
build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY_NAME)-linux-arm64 .

.PHONY: release-binaries
release-binaries:
	rm -rf dist
	mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		out=dist/$(BINARY_NAME)-$${os}-$${arch}; \
		echo "building $${out}"; \
		CGO_ENABLED=0 GOOS=$${os} GOARCH=$${arch} go build -ldflags '$(LDFLAGS)' -o $${out} . || exit 1; \
	done
	cd dist && shasum -a 256 $(BINARY_NAME)-* > checksums.txt

.PHONY: test
test:
	go test -race -count=1 ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: lint
lint: vet
	gofmt -l .

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: clean
clean:
	rm -rf dist

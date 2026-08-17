BINARY_NAME ?= rollouts-plugin-trafficrouter-gcloud
VERSION_PKG := github.com/argoproj-labs/rollouts-plugin-trafficrouter-gcloud/pkg/version
GIT_COMMIT := $(shell git rev-parse HEAD 2>/dev/null)
LDFLAGS := -X $(VERSION_PKG).GitCommit=$(GIT_COMMIT)

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY_NAME) .

.PHONY: build-linux-amd64
build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY_NAME)-linux-amd64 .

.PHONY: build-linux-arm64
build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY_NAME)-linux-arm64 .

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

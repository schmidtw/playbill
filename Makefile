# playbill — build, test, and packaging targets.
#
# The deliverable is a single self-contained static binary that requires no
# external programs at runtime (see docs/adr/0002): every build here is
# CGO_ENABLED=0 so the result is statically linked and trivially containerized.

# Binary name and output locations.
BINARY      := playbill
DIST        := dist
CMD         := ./cmd/playbill

# Version stamp, derived from git. Overridable: make build VERSION=1.2.3
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# Container image coordinates. Overridable: make docker IMAGE=ghcr.io/me/playbill
IMAGE       ?= playbill
IMAGE_TAG   ?= $(VERSION)

# Static-build flags. -s -w strips the symbol table and DWARF for a smaller
# binary; CGO_ENABLED=0 forces a pure-Go, statically linked result.
GO_LDFLAGS  := -s -w -X main.version=$(VERSION)
BUILD_ENV   := CGO_ENABLED=0

# Pick a container engine: docker if present, else podman.
DOCKER      ?= $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null || echo docker)

.PHONY: all build test cover lint tidy clean docker docker-run help

all: build ## Build the binary (default target)

build: ## Build the static CGO_ENABLED=0 binary into ./dist
	@mkdir -p $(DIST)
	$(BUILD_ENV) go build -trimpath -ldflags '$(GO_LDFLAGS)' -o $(DIST)/$(BINARY) $(CMD)

test: ## Run the test suite
	go test ./...

cover: ## Run tests with per-package coverage
	go test ./... -cover

lint: ## Run golangci-lint
	golangci-lint run ./...

tidy: ## Tidy and verify go.mod / go.sum
	go mod tidy
	go mod verify

clean: ## Remove build artifacts
	rm -rf $(DIST)

docker: ## Build the container image
	$(DOCKER) build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(IMAGE_TAG) .

docker-run: ## Run the image against $(LIBRARY) (e.g. make docker-run LIBRARY=/movies)
	$(DOCKER) run --rm \
		-e TMDB_API_KEY \
		-e FANARTTV_API_KEY \
		-v "$(LIBRARY):/library" \
		$(IMAGE):$(IMAGE_TAG) --dir /library

help: ## List available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

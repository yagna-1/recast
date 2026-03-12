## recast — Makefile
.PHONY: build test golden-test golden-update clean lint vet fmt

BINARY  = bin/recast
CMD     = ./cmd/recast
VERSION = $(shell git describe --tags --always 2>/dev/null || echo "dev")
COMMIT  = $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

# ---- Build ----

build:
	@mkdir -p bin
	go build $(LDFLAGS) -o $(BINARY) $(CMD)
	@echo "Built: $(BINARY)"

build-all:
	@mkdir -p dist
	GOOS=linux   GOARCH=amd64  go build $(LDFLAGS) -o dist/recast_linux_amd64   $(CMD)
	GOOS=linux   GOARCH=arm64  go build $(LDFLAGS) -o dist/recast_linux_arm64   $(CMD)
	GOOS=darwin  GOARCH=amd64  go build $(LDFLAGS) -o dist/recast_darwin_amd64  $(CMD)
	GOOS=darwin  GOARCH=arm64  go build $(LDFLAGS) -o dist/recast_darwin_arm64  $(CMD)
	GOOS=windows GOARCH=amd64  go build $(LDFLAGS) -o dist/recast_windows_amd64.exe $(CMD)
	@echo "Built all targets in dist/"

# ---- Test ----

test:
	go test ./... -v -timeout 60s

test-short:
	go test ./... -short -timeout 30s

golden-test:
	go test ./test/... -v -run TestGolden

golden-update:
	go test ./test/... -v -run TestGolden -args -update

# ---- Code quality ----

fmt:
	gofmt -w .

vet:
	go vet ./...

lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not found — install from https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run ./...

# ---- Dev usage ----

run-compile: build
	$(BINARY) compile testdata/fixtures/workflow_use_login.json -v

run-validate: build
	$(BINARY) validate testdata/fixtures/workflow_use_login.json

run-formats: build
	$(BINARY) formats

clean:
	rm -rf bin/ dist/ recast-out/

# ---- Dependencies ----

deps:
	go mod download

tidy:
	go mod tidy

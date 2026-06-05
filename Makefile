# Virta — build pipeline. Everything is `make`; "CI" = `make ci` run locally (ADR-022).
# No step requires a network or a remote.

MODULE      := github.com/elythi0n/virta
VERSION     ?= dev
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE        := $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)
LDFLAGS     := -s -w \
	-X $(MODULE)/internal/buildinfo.Version=$(VERSION) \
	-X $(MODULE)/internal/buildinfo.Commit=$(COMMIT) \
	-X $(MODULE)/internal/buildinfo.Date=$(DATE)

# Cross-compile matrix (the 6 shipped OS/arch targets).
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: all ci build run lint fmt fmt-check vet test test-race cover cross app fixtures \
        test-live-twitch test-live-kick test-live-x test-live-llm clean tidy

all: ci

## ci: the full local gate — must be green before any step is "done".
ci: fmt-check vet lint test-race cover cross
	@echo "✓ make ci green"

## build: compile everything.
build:
	go build ./...

## run: start the engine daemon.
run:
	go run -ldflags '$(LDFLAGS)' ./cmd/virtad

## fmt: format code.
fmt:
	gofmt -w .
	golangci-lint fmt ./... 2>/dev/null || true

## fmt-check: fail if code is not formatted.
fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

## vet: go vet.
vet:
	go vet ./...

## lint: golangci-lint (incl. depguard, forbidigo — ADR-020/024).
lint:
	golangci-lint run

## test: unit/contract/integration tests (offline only — ADR-024).
test:
	go test ./...

## test-race: tests with the race detector.
test-race:
	go test -race ./...

## cover: enforce coverage floors (core >=90% / overall >=80%).
cover:
	./scripts/coverage.sh

## cross: cross-compile virtad for all 6 OS/arch targets (proves it builds everywhere).
cross:
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		echo "  build $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' \
			-o dist/virtad-$$os-$$arch$$ext ./cmd/virtad || exit 1; \
	done
	@echo "✓ cross-compiled $(words $(PLATFORMS)) targets"

## app: the one-click deliverable (ADR-022). For now = the virtad binary; becomes the
## Wails desktop bundle at M4. Must always produce a runnable artifact.
app: build
	@mkdir -p dist
	go build -ldflags '$(LDFLAGS)' -o dist/virtad ./cmd/virtad
	@echo "✓ artifact: dist/virtad  (desktop bundle lands at M4)"

## fixtures: regenerate adapter fixtures (no adapters yet — wired in M1).
fixtures:
	@echo "no fixtures yet (adapters arrive in M1)"

## tidy: go mod tidy.
tidy:
	go mod tidy

## clean: remove build/test artifacts.
clean:
	rm -rf dist coverage.out coverage.txt

# --- Live tests (manual, build-tagged, NEVER part of `make ci` — ADR-024). ---
## test-live-*: hit real platform/provider endpoints. Run deliberately; see docs/live-debt.md.
test-live-twitch:
	go test -tags live -run TestLive ./internal/platform/twitch/... 2>/dev/null || echo "no live twitch tests yet"
test-live-kick:
	go test -tags live -run TestLive ./internal/platform/kick/... 2>/dev/null || echo "no live kick tests yet"
test-live-x:
	go test -tags live -run TestLive ./internal/platform/x/... 2>/dev/null || echo "no live x tests yet"
test-live-llm:
	go test -tags live -run TestLive ./internal/llm/... 2>/dev/null || echo "no live llm tests yet"

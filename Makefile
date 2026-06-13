# Virta — build pipeline. Everything is `make`; "CI" = `make ci` run locally.
# No step requires a network or a remote.

SHELL       := /bin/bash
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

# Dev mode: daemon addr and shared token. Override on the command line if needed.
DEV_ADDR  ?= 127.0.0.1:8799
DEV_TOKEN ?= dev

.PHONY: all ci build run dev db lint fmt fmt-check vet test test-race cover cross app app-run app-dmg app-win app-appimage \
        daemon tui web serve fixtures \
        tokens tokens-check apigen apigen-check test-live-twitch test-live-kick test-live-x test-live-llm soak docker clean tidy \
        build-plugins

all: ci

## ci: the full local gate — must be green before any step is "done".
ci: fmt-check vet lint test-race cover cross tokens-check apigen-check
	@echo "✓ make ci green"

## tokens: regenerate the design-system token artifacts (tokens.css, tokens.ts) from tokens.json.
tokens:
	go run ./cmd/tokengen

## tokens-check: fail if the generated token artifacts are stale (run `make tokens` and commit).
tokens-check:
	@go run ./cmd/tokengen
	@git diff --quiet -- frontends/ui-kit/tokens.css frontends/ui-kit/tokens.ts || \
		{ echo "token artifacts are stale; run 'make tokens' and commit"; exit 1; }

## apigen: regenerate the TypeScript wire types from the Go API contract structs.
apigen:
	go run ./cmd/apigen

## apigen-check: fail if the generated wire types are stale (run `make apigen` and commit).
apigen-check:
	@go run ./cmd/apigen
	@git diff --quiet -- frontends/web/src/daemon/wire.gen.ts || \
		{ echo "wire types are stale; run 'make apigen' and commit"; exit 1; }

## build: compile everything.
build:
	go build ./...

## run: start the engine daemon.
run:
	go run -ldflags '$(LDFLAGS)' ./cmd/virtad

## dev: daemon + Vite hot-reload in one terminal. Loads .env for VIRTA_TOKEN, VIRTA_LOGGING_ENABLED, etc.
##      Run `make db` first to start Postgres. Open http://localhost:5173.
##      Override: DEV_ADDR=host:port make dev
dev:
	@echo "→ daemon  http://$(DEV_ADDR)"
	@echo "→ web UI  http://localhost:5173"
	@set -a; [ -f .env ] && . ./.env; set +a; \
		VIRTA_DB_DSN="postgres://virta:$${POSTGRES_PASSWORD}@localhost:5433/virta?sslmode=disable"; \
		VIRTA_ADDR=$(DEV_ADDR) go run -ldflags '$(LDFLAGS)' ./cmd/virtad & \
		GO_PID=$$!; \
		trap "kill $$GO_PID 2>/dev/null" EXIT INT TERM; \
		TOKEN=$${VIRTA_TOKEN:-$(DEV_TOKEN)}; \
		cd frontends/web && npm install && \
			VIRTA_DAEMON=http://$(DEV_ADDR) VIRTA_TOKEN=$$TOKEN npm run dev; \
		wait $$GO_PID

## db: start the Postgres container for local dev (exposes :5432 on localhost).
db:
	docker compose -f docker-compose.yml -f docker-compose.dev.yml up db -d

## fmt: format code.
fmt:
	gofmt -w .
	golangci-lint fmt ./... 2>/dev/null || true

## fmt-check: fail if code is not formatted.
fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

## vet: go vet (cmd/virta-overlay excluded — requires CGO/pkg-config not available on all hosts).
vet:
	go vet $(shell go list ./... | grep -v 'cmd/virta-overlay')

## lint: golangci-lint (incl. depguard, forbidigo).
## cmd/virta-overlay is excluded — it requires CGO/pkg-config not available on all hosts.
## Tries PATH first (golangci-lint-action installs there); falls back to GOPATH/bin for local installs.
GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null || echo $(shell go env GOPATH)/bin/golangci-lint)
lint:
	$(GOLANGCI_LINT) run \
		./internal/... ./cmd/virtad/... ./cmd/virta-tui/... \
		./cmd/apigen/... ./cmd/tokengen/... ./cmd/healthcheck/... \
		./examples/...

## test: unit/contract/integration tests (offline only — no network).
test:
	go test $(shell go list ./... | grep -v 'cmd/virta-overlay')

## test-race: tests with the race detector.
test-race:
	go test -race $(shell go list ./... | grep -v 'cmd/virta-overlay')

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

## tui: build the virta-tui binary for the host OS.
tui:
	@mkdir -p dist
	@ext=""; [ "$$(go env GOOS)" = "windows" ] && ext=".exe"; \
		go build -ldflags '$(LDFLAGS)' -o dist/virta-tui$$ext ./cmd/virta-tui
	@echo "✓ artifact: dist/virta-tui"

## web: build the web UI and stage it where virtad embeds it, so the daemon serves the app itself.
web:
	cd frontends/web && npm install && npm run build
	@find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
	@cp -r frontends/web/dist/. internal/webui/dist/

## daemon: build the virtad binary. Run `make web` first to embed the UI (so `virtad` serves it).
daemon:
	@mkdir -p dist
	go build -ldflags '$(LDFLAGS)' -o dist/virtad ./cmd/virtad
	@echo "✓ artifact: dist/virtad"

## serve: build the UI into the daemon and run it; open the printed URL in any browser. One process.
serve: web daemon
	./dist/virtad

## app: build the web UI + virtad daemon and stage them into the Electron shell, then install its
## deps. The shell (frontends/desktop) is an Electron app: it serves the web build over a loopback
## HTTP server and spawns the bundled virtad. No CGO/webkit/pkg-config needed.
app:
	cd frontends/web && npm install && npm run build
	@find frontends/desktop/resources/web -mindepth 1 ! -name .gitkeep -delete
	@cp -r frontends/web/dist/. frontends/desktop/resources/web/
	@find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
	@cp -r frontends/web/dist/. internal/webui/dist/
	@mkdir -p frontends/desktop/resources/bin frontends/desktop/build
	@ext=""; [ "$$(go env GOOS)" = "windows" ] && ext=".exe"; \
		CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o frontends/desktop/resources/bin/virtad$$ext ./cmd/virtad
	@cp frontends/ui-kit/src/assets/virta-logo-512.png frontends/desktop/build/icon.png
	cd frontends/desktop && npm install
	@echo "✓ Electron shell staged: frontends/desktop (run 'make app-run' to launch)"

## app-run: stage everything and launch the Electron shell in development (spawns its own daemon).
app-run: app
	cd frontends/desktop && npm start

## app-dmg: macOS .dmg via electron-builder (output: dist/Virta.dmg). Run on macOS.
app-dmg: app
	cd frontends/desktop && npm run dist:mac

## app-win: Windows installer via electron-builder NSIS (output: dist/Virta-Setup.exe). Run on Windows.
app-win: app
	cd frontends/desktop && npm run dist:win

## app-appimage: Linux AppImage via electron-builder (output: dist/Virta-x86_64.AppImage). Run on Linux.
app-appimage: app
	cd frontends/desktop && npm run dist:linux

## fixtures: regenerate golden fixtures by re-running normalization with -update.
fixtures:
	go test ./... -run 'Golden|Replay' -update

## docker: build the server image (for hosting virtad via docker compose).
docker:
	docker build -t virta:dev .

## tidy: go mod tidy.
tidy:
	go mod tidy

## build-plugins: build all first-party plugin zips into plugins/*/dist/ and copy to dist/.
## Install locally with: POST /v1/plugins/install {"url": "/absolute/path/to/plugins/<name>"}
build-plugins:
	@mkdir -p dist
	@for p in alerts leaderboard polls vod-replay; do \
		echo "  plugin $$p"; \
		bash plugins/$$p/build.sh; \
		cp plugins/$$p/dist/$$p.zip dist/; \
	done
	@echo "✓ plugins built"

## clean: remove build/test artifacts.
clean:
	rm -rf dist coverage.out coverage.txt

# --- Live tests (manual, build-tagged, NEVER part of `make ci`). ---
## test-live-*: hit real platform/provider endpoints. Run deliberately.
test-live-twitch:
	go test -tags live -run TestLive ./internal/platform/twitch/... 2>/dev/null || echo "no live twitch tests yet"
test-live-kick:
	go test -tags live -run TestLive ./internal/platform/kick/... 2>/dev/null || echo "no live kick tests yet"
test-live-x:
	go test -tags live -run TestLive ./internal/platform/x/... 2>/dev/null || echo "no live x tests yet"
test-live-llm:
	go test -tags live -run TestLive ./internal/llm/... 2>/dev/null || echo "no live llm tests yet"

# --- Soak (manual, build-tagged). Offline memory/goroutine stability under sustained load. ---
## soak: drive the full ingest path for SOAK_SECONDS (default 60) at SOAK_RATE msg/s (default 200).
soak:
	go test -tags soak -run TestSoak_FullPath -v -timeout 0 ./internal/engine/...

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

.PHONY: all ci build run dev db lint fmt fmt-check vet test test-race cover cross app daemon tui web serve fixtures \
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
lint:
	$(shell go env GOPATH)/bin/golangci-lint run \
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

## app: one-click desktop bundle. No external CLI required — Wails v3 builds with plain go build.
## Requires WebKit dev libraries: webkit2gtk-4.1 (GTK3) or webkitgtk-6.0 (GTK4).
app:
	@command -v pkg-config >/dev/null 2>&1 || { echo "pkg-config not found — install pkg-config and webkit dev libraries"; exit 1; }
	cd frontends/web && npm install && npm run build
	@find frontends/desktop/assets -mindepth 1 ! -name .gitkeep -delete
	cp -r frontends/web/dist/. frontends/desktop/assets/
	@find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
	@cp -r frontends/web/dist/. internal/webui/dist/
	@ext=""; [ "$$(go env GOOS)" = "windows" ] && ext=".exe"; \
		CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o frontends/desktop/bin/virtad$$ext ./cmd/virtad
	@mkdir -p frontends/desktop/build/bin
	@cd frontends/desktop && go mod tidy && { \
		gtk=""; \
		if pkg-config --modversion webkit2gtk-4.1 >/dev/null 2>&1; then gtk="-tags gtk3"; \
		elif pkg-config --modversion webkit2gtk-4.0 >/dev/null 2>&1; then gtk="-tags gtk3"; fi; \
		echo "+ CGO_ENABLED=1 go build $$gtk -o build/bin/virta ."; \
		CGO_ENABLED=1 go build $$gtk -ldflags '-s -w' -o build/bin/virta .; \
	}
	@echo "✓ desktop bundle: frontends/desktop/build/bin/virta"

## app-debug: same as app but with DevToolsEnabled per window and no binary stripping.
## Launch the binary and go to Settings → About → Open WebKit Inspector.
app-debug:
	@command -v pkg-config >/dev/null 2>&1 || { echo "pkg-config not found — install pkg-config and webkit dev libraries"; exit 1; }
	cd frontends/web && npm install && npm run build
	@find frontends/desktop/assets -mindepth 1 ! -name .gitkeep -delete
	cp -r frontends/web/dist/. frontends/desktop/assets/
	@find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
	@cp -r frontends/web/dist/. internal/webui/dist/
	@ext=""; [ "$$(go env GOOS)" = "windows" ] && ext=".exe"; \
		CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o frontends/desktop/bin/virtad$$ext ./cmd/virtad
	@mkdir -p frontends/desktop/build/bin
	@cd frontends/desktop && go mod tidy && { \
		gtk=""; \
		if pkg-config --modversion webkit2gtk-4.1 >/dev/null 2>&1; then gtk="-tags gtk3,devtools"; \
		elif pkg-config --modversion webkit2gtk-4.0 >/dev/null 2>&1; then gtk="-tags gtk3,devtools"; fi; \
		echo "+ CGO_ENABLED=1 go build $$gtk -o build/bin/virta-debug ."; \
		CGO_ENABLED=1 go build $$gtk -o build/bin/virta-debug .; \
	}
	@echo "✓ debug bundle: frontends/desktop/build/bin/virta-debug  (Settings → About → Open WebKit Inspector)"

## app-appimage: Linux AppImage. Requires appimagetool + the app target's prerequisites.
app-appimage: app daemon tui
	@command -v appimagetool >/dev/null 2>&1 || { echo "appimagetool not found (https://appimage.github.io/appimagetool/)"; exit 1; }
	@mkdir -p dist/AppDir/usr/bin dist/AppDir/usr/share/applications dist/AppDir/usr/share/icons/hicolor/512x512/apps
	cp frontends/desktop/build/bin/virta dist/AppDir/usr/bin/virta
	cp dist/virtad dist/AppDir/usr/bin/virtad
	cp dist/virta-tui dist/AppDir/usr/bin/virta-tui
	cp packaging/virta.desktop dist/AppDir/usr/share/applications/virta.desktop
	cp frontends/ui-kit/src/assets/virta-logo-512.png dist/AppDir/usr/share/icons/hicolor/512x512/apps/virta.png
	# appimagetool requires AppRun, a .desktop file, and an icon at the AppDir root.
	cp packaging/virta.desktop dist/AppDir/virta.desktop
	cp frontends/ui-kit/src/assets/virta-logo-512.png dist/AppDir/virta.png
	ln -sf usr/bin/virta dist/AppDir/AppRun
	ARCH=x86_64 appimagetool dist/AppDir dist/Virta-$(VERSION).AppImage
	@echo "✓ AppImage: dist/Virta-$(VERSION).AppImage"

## app-dmg: macOS disk image. Requires xcode + create-dmg. Stages a Virta.app bundle first —
## the raw binary from `app` isn't double-clickable on macOS without one.
app-dmg: app
	@command -v create-dmg >/dev/null 2>&1 || { echo "create-dmg not found (brew install create-dmg)"; exit 1; }
	@rm -rf dist/dmg
	@mkdir -p dist/dmg/Virta.app/Contents/MacOS dist/dmg/Virta.app/Contents/Resources
	cp frontends/desktop/build/bin/virta dist/dmg/Virta.app/Contents/MacOS/virta
	sed 's/__VERSION__/$(VERSION)/' packaging/Info.plist > dist/dmg/Virta.app/Contents/Info.plist
	create-dmg \
		--volname "Virta" \
		--background frontends/ui-kit/src/assets/virta-logo-512.png \
		--window-pos 200 120 \
		--window-size 600 400 \
		--icon-size 100 \
		--icon "Virta.app" 175 120 \
		--hide-extension "Virta.app" \
		--app-drop-link 425 120 \
		"dist/Virta-$(VERSION).dmg" \
		"dist/dmg/"
	@echo "✓ DMG: dist/Virta-$(VERSION).dmg"

## app-inno: Windows Inno Setup installer. Requires ISCC + running on Windows.
## On Windows runners (no make) use scripts/package-windows.sh, which mirrors this.
app-inno: app daemon tui
	@command -v iscc >/dev/null 2>&1 || { echo "iscc not found (choco install innosetup)"; exit 1; }
	MSYS2_ARG_CONV_EXCL="/D" iscc "/DAppVersion=$(VERSION)" packaging/virta.iss
	@echo "✓ installer: dist/VirtaSetup-$(VERSION).exe"

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

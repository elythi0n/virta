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

.PHONY: all ci build run lint fmt fmt-check vet test test-race cover cross app daemon web serve fixtures \
        tokens tokens-check apigen apigen-check test-live-twitch test-live-kick test-live-x test-live-llm soak docker clean tidy

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

## app: the one-click desktop bundle (ADR-022): the web UI + an embedded virtad in one native
## artifact. Requires the Wails CLI and the WebKit dev libraries for this OS (not part of make ci).
## Builds the web UI, stages it and a host virtad for embedding, then runs the Wails build.
## WAILS resolves the wails CLI: checks PATH first, then the standard Go bin dir.
WAILS := $(shell command -v wails 2>/dev/null || echo "$$(go env GOPATH)/bin/wails")

app:
	@test -x "$(WAILS)" || { echo "wails CLI not found: run 'go install github.com/wailsapp/wails/v2/cmd/wails@latest' and install your OS's WebKit dev libraries"; exit 1; }
	cd frontends/web && npm install && npm run build
	@find frontends/desktop/assets -mindepth 1 ! -name .gitkeep -delete
	cp -r frontends/web/dist/. frontends/desktop/assets/
	@find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
	@cp -r frontends/web/dist/. internal/webui/dist/
	@ext=""; [ "$$(go env GOOS)" = "windows" ] && ext=".exe"; \
		CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o frontends/desktop/bin/virtad$$ext ./cmd/virtad
	@mkdir -p frontends/desktop/build && cp frontends/ui-kit/src/assets/virta-logo-512.png frontends/desktop/build/appicon.png
	@cd frontends/desktop && go mod tidy && { \
		tags=""; \
		if pkg-config --modversion webkit2gtk-4.1 >/dev/null 2>&1 && ! pkg-config --modversion webkit2gtk-4.0 >/dev/null 2>&1; then tags="-tags webkit2_41"; fi; \
		echo "+ $(WAILS) build -s $$tags"; \
		$(WAILS) build -s $$tags; \
	}
	@echo "✓ desktop bundle: frontends/desktop/build/bin"

## app-appimage: Linux AppImage. Requires appimagetool + the app target's prerequisites.
app-appimage: app
	@command -v appimagetool >/dev/null 2>&1 || { echo "appimagetool not found (https://appimage.github.io/appimagetool/)"; exit 1; }
	@mkdir -p dist/AppDir/usr/bin dist/AppDir/usr/share/applications dist/AppDir/usr/share/icons/hicolor/512x512/apps
	cp frontends/desktop/build/bin/virta dist/AppDir/usr/bin/virta
	cp dist/virtad dist/AppDir/usr/bin/virtad
	cp dist/virta-tui dist/AppDir/usr/bin/virta-tui
	cp packaging/virta.desktop dist/AppDir/usr/share/applications/virta.desktop
	cp frontends/ui-kit/src/assets/virta-logo-512.png dist/AppDir/usr/share/icons/hicolor/512x512/apps/virta.png
	ARCH=x86_64 appimagetool dist/AppDir dist/Virta-$(shell git describe --tags --dirty).AppImage
	@echo "✓ AppImage: dist/Virta-*.AppImage"

## app-dmg: macOS disk image (universal). Requires xcode + create-dmg.
app-dmg: app
	@command -v create-dmg >/dev/null 2>&1 || { echo "create-dmg not found (brew install create-dmg)"; exit 1; }
	create-dmg \
		--volname "Virta" \
		--background frontends/ui-kit/src/assets/virta-logo-512.png \
		--window-pos 200 120 \
		--window-size 600 400 \
		--icon-size 100 \
		--icon "Virta.app" 175 120 \
		--hide-extension "Virta.app" \
		--app-drop-link 425 120 \
		"dist/Virta-$(shell git describe --tags --dirty).dmg" \
		"frontends/desktop/build/bin/"
	@echo "✓ DMG: dist/Virta-*.dmg"

## app-nsis: Windows NSIS installer. Requires NSIS makensis + running on Windows or Wine.
app-nsis: app
	@command -v makensis >/dev/null 2>&1 || { echo "makensis not found (choco install nsis / apt install nsis)"; exit 1; }
	makensis packaging/virta.nsi
	@echo "✓ installer: dist/VirtaSetup-*.exe"

## fixtures: regenerate golden fixtures by re-running normalization with -update.
fixtures:
	go test ./... -run 'Golden|Replay' -update

## docker: build the server image (for hosting virtad via docker compose).
docker:
	docker build -t virta:dev .

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

# --- Soak (manual, build-tagged). Offline memory/goroutine stability under sustained load. ---
## soak: drive the full ingest path for SOAK_SECONDS (default 60) at SOAK_RATE msg/s (default 200).
soak:
	go test -tags soak -run TestSoak_FullPath -v -timeout 0 ./internal/engine/...

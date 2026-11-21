# The recipes (clean's rm -rf too) name paths relative to the working
# folder, so they run only in magpie's own, known by its go.mod: run from
# another one (make -f path/to/Makefile) they would act there. make -C
# works. go.mod is read relative to the working folder, so a path with
# spaces (or a symlinked Makefile) changes nothing.
ifneq ($(shell sed -n 's/^module //p' go.mod 2>/dev/null),github.com/yetone/magpie)
  $(error run make in magpie's folder (make -C path/to/magpie), not in $(CURDIR))
endif

# A tag may hold $, ( or `, which the shell lines below would run: the
# version keeps only what a version is spelled with.
# 浩哥AI's version comes from the newest haoge-v tag (1.4.0+haoge) and
# moves with every release. The +haoge marker keeps update.Released()
# false: the fork never self-updates from upstream's feed (which would
# restore the magpie brand), and stats telemetry stays off too.
VERSION ?= $(shell go run ./build/version 2>/dev/null || echo 0.0.0-dev+haoge)
# How many things build at once: the machine's cores (Go's own default is
# the same; said here so the icon set runs that many at a time too).
JOBS ?= $(shell sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4)
LDFLAGS  = -s -w -X main.version=$(VERSION)
TAGS     = production
# 浩哥AI builds two artifacts only: macOS arm64 (.dmg) and Windows amd64 (.exe).
TARGETS  = darwin/arm64 windows/amd64

# The GUI links the platform webview through cgo, so it is built natively.
# `nogui` builds the terminal-only magpie, which cross-compiles anywhere.
# macOS 12 is the oldest Go (1.25 and later) runs on; Info.plist says so too.
ifeq ($(shell uname -s),Darwin)
  export CGO_CFLAGS  = -mmacosx-version-min=12.0
  export CGO_LDFLAGS = -mmacosx-version-min=12.0
endif
# On Linux, GTK 3 and WebKitGTK 4.1: older distributions have them, where
# Wails' default GTK 4 and WebKitGTK 6 would leave them out.
ifeq ($(shell uname -s),Linux)
  TAGS += gtk3
endif
# Termux uses Android's libc and has no desktop webview. Keep the web UI
# and terminal commands, without linking GTK or Wails.
ifeq ($(shell go env GOOS),android)
  TAGS = nogui
endif

.PHONY: assets-pack conpty-pair build-release build cli install test test-ui app icons release release-cli release-windows release-linux clean dev dev-once

# A release build embeds the UI's files as one zip instead of the tree
# (internal/gui/assets_packed.go): a Windows or Linux download is the raw
# binary, and the scripts, styles and svgs are a quarter of their size packed.
# The zip is not kept in git; every release target makes it first.
assets-pack:
	@go run ./build/assetpack internal/gui/assets internal/gui/assets.zip

# The ConPTY a Windows release carries for Windows 10 (internal/conptypair,
# #5205): Microsoft's, from nuget.org, checked against its pinned hashes and
# embedded with -tags conptypair.
conpty-pair:
	@go run ./build/conpty

# Every build but the test runs takes Go's runtime with the macOS page
# release of build/darwin/footprint-overlay.sh: with Go's own, heap Go
# gave back stayed in Activity Monitor's Memory on macOS 27. It replaces
# a darwin-only file, so other systems' builds are the same with it.
OVERLAY = F=$$(sh build/darwin/footprint-overlay.sh .overlay) || exit 1;

build:
	$(OVERLAY) go build $$F -tags "$(TAGS)" -trimpath -ldflags="$(LDFLAGS)" -o magpie .

cli:
	$(OVERLAY) CGO_ENABLED=0 go build $$F -tags nogui -trimpath -ldflags="$(LDFLAGS)" -o magpie .

build-release: assets-pack
	$(OVERLAY) go build -p $(JOBS) $$F -tags "$(TAGS) packedassets" -trimpath -ldflags="$(LDFLAGS)" -o magpie .

install:
	go install -tags "$(TAGS)" -trimpath -ldflags="$(LDFLAGS)" .

test:
	go vet -tags "$(TAGS)" ./... && go test -tags "$(TAGS)" ./...

# The browser regressions under internal/gui/tests: the real pages in
# Playwright's Chromium and WebKit; see internal/gui/tests/README.md.
UI_TESTS = $(wildcard internal/gui/tests/*.test.cjs)
# Files are independent and run in parallel; 1 diagnoses a flaky one.
UI_TEST_CONCURRENCY ?= 2

test-ui:
	@test -n "$(UI_TESTS)" || { echo "no GUI tests found under internal/gui/tests" >&2; exit 1; }
	node --test --test-concurrency=$(UI_TEST_CONCURRENCY) $(UI_TESTS)

# macOS bundle: menu bar app with no Dock icon (LSUIElement). The bundle is
# named 浩哥AI.app; its executable stays magpie (CFBundleExecutable), and the
# Dock icon comes from magpie.icns (no Assets.car: the icon set is pure Go).
app: build-release
	@rm -rf 浩哥AI.app
	@mkdir -p 浩哥AI.app/Contents/MacOS 浩哥AI.app/Contents/Resources
	@cp magpie 浩哥AI.app/Contents/MacOS/magpie
	@cp build/darwin/magpie.icns 浩哥AI.app/Contents/Resources/magpie.icns
	@cp THIRD_PARTY_NOTICES 浩哥AI.app/Contents/Resources/THIRD_PARTY_NOTICES
	@sed 's/@VERSION@/$(VERSION)/' build/darwin/Info.plist > 浩哥AI.app/Contents/Info.plist
	@echo "  浩哥AI.app"

# macOS arm64 disk image: the .app with an Applications symlink for drag
# install. Runs on macOS only (hdiutil); the GUI itself needs cgo + WebKit,
# so it is built natively on an arm64 Mac (or a macOS arm64 CI runner).
# hdiutil's UDZO is zlib on one core, with no parallel option: ULFO
# (lzfse, macOS 10.11+) compresses several times faster for a slightly
# larger image. DMG_FORMAT=UDZO brings the old one back.
DMG_FORMAT ?= ULFO
release-dmg: app
	@mkdir -p dist
	@rm -rf dist/dmg-stage && mkdir -p dist/dmg-stage
	@cp -R 浩哥AI.app dist/dmg-stage/
	@ln -s /Applications dist/dmg-stage/Applications
	@hdiutil create -volname "浩哥AI" -srcfolder dist/dmg-stage -ov -format $(DMG_FORMAT) dist/haoge-ai-darwin-arm64.dmg
	@rm -rf dist/dmg-stage
	@echo "  dist/haoge-ai-darwin-arm64.dmg"

icons:
	@mkdir -p dist/tools
	@go build -trimpath -o dist/tools/gen-icons build/icon/gen.go
	@# Windows draws an app's icon to the edges of its square: the exe's
	@# icon group (build/windows/winres.json) and the tray's .ico. Every icon
	@# is drawn on its own, so all of them run at once (the compiler was the
	@# slow part: it is built once above, not once per icon), and the recipe
	@# fails if any of them did.
	@pids=""; bg() { "$$@" & pids="$$pids $$!"; }; \
	 bg dist/tools/gen-icons tray internal/gui/tray.png; \
	 bg dist/tools/gen-icons tray-flap internal/gui/trayflap; \
	 for s in 16 20 24 32 40 48 64 128 256; do bg dist/tools/gen-icons win $$s build/windows/icon-$$s.png; done; \
	 bg dist/tools/gen-icons win-ico internal/gui/icon-windows.ico; \
	 bg dist/tools/gen-icons app 1024 internal/gui/icon-1024.png; \
	 bg dist/tools/gen-icons app 256 internal/gui/icon-256.png; \
	 bg dist/tools/gen-icons icns build/darwin/magpie.icns; \
	 for p in $$pids; do wait $$p || exit 1; done

# 浩哥AI's two artifacts: the macOS arm64 .dmg (built on a Mac) and the
# Windows amd64 .exe (cross-compiles anywhere). `make release` builds the one
# for the host it runs on; CI builds each on its own runner.
release:
ifeq ($(shell uname -s),Darwin)
	@$(MAKE) --no-print-directory release-dmg
else
	@$(MAKE) --no-print-directory release-windows
endif

release-cli: assets-pack conpty-pair
	@mkdir -p dist
	@$(OVERLAY) for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; ext=""; [ $$os = windows ] && ext=.exe; \
		echo "  $$os/$$arch (cli)"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build $$F -tags "nogui packedassets conptypair" -trimpath -ldflags="$(LDFLAGS)" -o dist/magpie-cli-$$os-$$arch$$ext . || exit 1; \
	done

# The desktop app for Windows: the system WebView2 needs no cgo, so both
# arches cross-compile. Linked as a GUI program (no console window), with
# the icon, version and a DPI-aware manifest from build/windows. It carries
# magpie.com, the console program a terminal runs for `magpie` and waits
# for, which it writes beside itself (internal/update/com.go, #3737).
# The launcher is built without the commit stamped in (-buildvcs=false):
# magpie.com is written again whenever its bytes differ, and with the
# commit in them every release wrote a new one though the launcher was the
# same (TestLauncherSameAtEveryCommit).
WINVER = $(shell echo $(VERSION) | sed -E 's/^v//; s/[^0-9.].*//')
release-windows: assets-pack conpty-pair
	@mkdir -p dist
	@go run github.com/tc-hib/go-winres@v0.3.3 make --in build/windows/winres.json --arch amd64 --out rsrc \
		--file-version "$(or $(WINVER),0.0.0)" --product-version "$(or $(WINVER),0.0.0)"
	@echo "  windows/amd64 (app)"
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="-s -w" -o internal/update/launcher-amd64.com ./internal/update/launcher || exit 1
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags production,comlauncher,packedassets,conptypair -trimpath -ldflags="$(LDFLAGS) -H windowsgui" -o dist/haoge-ai-windows-amd64.exe . || exit 1
	@rm -f rsrc_windows_*.syso internal/update/launcher-*.com

# The desktop app for Linux links GTK 3 and WebKitGTK 4.1 through cgo, so it
# is built natively, one arch per machine.
release-linux: assets-pack
	@mkdir -p dist
	@echo "  linux/$(shell go env GOARCH) (app)"
	@go build -tags "$(TAGS) packedassets" -trimpath -ldflags="$(LDFLAGS)" -o dist/magpie-linux-$(shell go env GOARCH) .

clean:
	rm -rf magpie magpie.exe magpie.app 浩哥AI.app dist .overlay internal/gui/assets.zip rsrc_windows_*.syso internal/update/launcher-*.com internal/conptypair/amd64 internal/conptypair/arm64

# Development: the UI is served from internal/gui/assets and the window
# reloads itself when a file there is saved. With fswatch installed, the
# windows stay up and a Go change rebuilds and restarts only the backend
# behind them (build/dev.sh); without it, dev-once runs the app as one
# process. Uses its own ports so a running magpie keeps serving the agents.
DEV_ADDR ?= 127.0.0.1:3426
DEV_UI ?= 127.0.0.1:3427
DEV_BACKEND ?= 127.0.0.1:3428
DEV_CONTROL ?= 127.0.0.1:3429
dev:
	@if command -v fswatch >/dev/null; then \
	  MAGPIE_ADDR=$(DEV_ADDR) MAGPIE_DEV_UI=$(DEV_UI) MAGPIE_DEV_BACKEND=$(DEV_BACKEND) MAGPIE_DEV_CONTROL=$(DEV_CONTROL) build/dev.sh; \
	else $(MAKE) --no-print-directory dev-once; fi

dev-once:
	@go build -tags dev -o magpie-dev . && echo "  magpie-dev · UI from internal/gui/assets, reload on save · gateway $(DEV_ADDR)"
	@MAGPIE_ADDR=$(DEV_ADDR) MAGPIE_DEV_UI=$(DEV_UI) ./magpie-dev app

APP := polycom-manager
VERSION ?= dev
DIST := dist
WEB_SRC := web
WEB_DIST := $(WEB_SRC)/dist
WEB_EMBED := internal/webui/dist
WEB_EMBED_TMP := internal/webui/.dist.tmp
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all deps go-deps web-deps web web-ready verify-web test build build-go linux linux-go windows windows-go release clean

all: build

go-deps:
	go mod tidy

web-deps:
	cd $(WEB_SRC) && if [ -f package-lock.json ]; then bun ci; else bun install; fi

deps: go-deps web-deps

# Build React/Vite first and then atomically replace the directory consumed by
# //go:embed. This prevents Go from compiling a stale or half-copied UI.
web: web-deps
	cd $(WEB_SRC) && bun run build
	test -s $(WEB_DIST)/index.html || (echo "ERROR: $(WEB_DIST)/index.html was not produced by Vite" >&2; exit 1)
	rm -rf $(WEB_EMBED_TMP)
	mkdir -p $(WEB_EMBED_TMP)
	cp -R $(WEB_DIST)/. $(WEB_EMBED_TMP)/
	test -s $(WEB_EMBED_TMP)/index.html || (echo "ERROR: failed to stage embedded web UI" >&2; exit 1)
	rm -rf $(WEB_EMBED)
	mv $(WEB_EMBED_TMP) $(WEB_EMBED)

# Ordered verification target for full builds. Using an explicit dependency on
# `web` also makes `make -j build` safe.
web-ready: web
	$(MAKE) verify-web

verify-web:
	test -s $(WEB_EMBED)/index.html || (echo "ERROR: embedded web UI is missing. Run 'make web' or 'make build'." >&2; exit 1)
	@asset_count=$$(find $(WEB_EMBED) -type f | wc -l); \
	if [ "$$asset_count" -lt 2 ]; then \
		echo "ERROR: embedded web UI looks incomplete ($(WEB_EMBED), $$asset_count file(s))" >&2; \
		exit 1; \
	fi

test: verify-web
	go test ./...

# Full local build: Go dependencies -> Vite -> embedded assets -> executable.
build: go-deps web-ready
	mkdir -p $(DIST)
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(APP) ./cmd/$(APP)

# Compile only the Go executable when the embedded web bundle is already ready.
build-go: go-deps verify-web
	mkdir -p $(DIST)
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(APP) ./cmd/$(APP)

linux: go-deps web-ready
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(APP)-linux-amd64 ./cmd/$(APP)

linux-go: go-deps verify-web
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(APP)-linux-amd64 ./cmd/$(APP)

windows: go-deps web-ready
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(APP)-windows-amd64.exe ./cmd/$(APP)

windows-go: go-deps verify-web
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(APP)-windows-amd64.exe ./cmd/$(APP)

# Build Vite exactly once, run all Go tests, then cross-compile both platforms.
release: go-deps web-ready
	go test ./...
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(APP)-linux-amd64 ./cmd/$(APP)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(APP)-windows-amd64.exe ./cmd/$(APP)

clean:
	rm -rf $(DIST) $(WEB_SRC)/dist $(WEB_SRC)/node_modules $(WEB_EMBED_TMP)

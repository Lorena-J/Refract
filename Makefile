.PHONY: dev dev-api dev-web build web test login users docker e2e mac mac-release mac-release-check mac-test

# Go API on :8080 + Vite on :5173 (proxies /api). Open http://localhost:5173.
dev:
	$(MAKE) -j2 dev-api dev-web

# Set REFRACT_ALLOWED_SUBJECT (printed by `make login`) and optionally REFRACT_MODEL_AGENT in your shell.
# With a sign-in page: make dev AUTH=builtin (accounts: make users ARGS="add you@example.com").
AUTH ?= none
# Loopback only by default; REFRACT_ADDR in the environment overrides it.
REFRACT_ADDR ?= 127.0.0.1:8080
DEV_ENV = REFRACT_DATA_DIR=./data REFRACT_SECRET_KEY=$$(cat data/dev.key)
dev-api: data/dev.key
	REFRACT_AUTH=$(AUTH) REFRACT_ADDR=$(REFRACT_ADDR) $(DEV_ENV) go run ./cmd/refract

# Accounts in the dev data: make users ARGS="list" | ARGS="add you@example.com" | ARGS="rename local you@example.com"
users: data/dev.key
	$(DEV_ENV) go run ./cmd/refract users $(ARGS)

# Local-only encryption key for dev data; data/ is gitignored.
data/dev.key:
	mkdir -p data
	umask 077 && openssl rand -base64 32 > $@

# Browser smoke test against the built binary (needs tectonic and Google Chrome).
e2e: build
	cd web && npx playwright test

login:
	go run ./cmd/refract login

dev-web: web/node_modules
	cd web && npm run dev

web/node_modules: web/package-lock.json
	cd web && npm ci
	touch $@

# vite empties dist/, restore the placeholder go:embed needs on a fresh clone.
web: web/node_modules
	cd web && npm run build
	touch web/dist/.gitkeep

build: web
	CGO_ENABLED=0 go build -o refract ./cmd/refract

test:
	go vet ./...
	go test ./...
	node --test web/src/*.test.ts web/src/lib/*.test.ts
	python3 -m unittest discover -s scripts -p 'test_*.py'

# Builds and smoke-tests the image for each platform (the foreign one runs emulated).
# Extra buildx flags: make docker BUILDX_FLAGS="--builder mybuilder"
PLATFORMS ?= amd64 arm64
docker:
	for p in $(PLATFORMS); do \
		docker buildx build $(BUILDX_FLAGS) --platform linux/$$p --load -t refract:$$p . && \
		scripts/smoke-docker.sh refract:$$p linux/$$p || exit 1; \
	done

# Refract.app for one architecture (macOS only; needs Xcode and Go): build/mac/<arch>/Refract.app,
# signed ad hoc. The first run builds the bundled Git and synctex, which takes a few minutes.
# make mac MAC_ARCH=amd64 on an Intel Mac; TOOLS_ARCHIVE=<refract-tools-*.tar.gz> skips building the tools.
MAC_ARCH ?= $(if $(filter arm64,$(shell uname -m)),arm64,amd64)
MAC_VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo v0.0.0-dev)
mac: web
	macos/build-app.sh $(MAC_ARCH) $(MAC_VERSION) build/mac/$(MAC_ARCH)

# A signed, notarized, stapled Refract.app and its zip, for one architecture (see macos/README.md,
# "Signing and notarization"):
#   REFRACT_SIGN_IDENTITY="Developer ID Application: Name (TEAMID)" REFRACT_NOTARY_PROFILE=refract-notary make mac-release
mac-release: mac-release-check web
	macos/build-app.sh $(MAC_ARCH) $(MAC_VERSION) build/mac/$(MAC_ARCH)
	macos/notarize.sh build/mac/$(MAC_ARCH)/Refract.app
	ditto -c -k --keepParent build/mac/$(MAC_ARCH)/Refract.app build/mac/$(MAC_ARCH)/Refract-$(MAC_ARCH).zip
	@echo build/mac/$(MAC_ARCH)/Refract-$(MAC_ARCH).zip

mac-release-check:
	@test -n "$$REFRACT_SIGN_IDENTITY" && test "$$REFRACT_SIGN_IDENTITY" != "-" || { echo "set REFRACT_SIGN_IDENTITY to your Developer ID Application identity" >&2; exit 2; }
	@test -n "$$REFRACT_NOTARY_PROFILE" || { test -n "$$REFRACT_NOTARY_KEY_PATH" && test -n "$$REFRACT_NOTARY_KEY_ID" && test -n "$$REFRACT_NOTARY_ISSUER"; } || { echo "set REFRACT_NOTARY_PROFILE (or REFRACT_NOTARY_KEY_PATH, REFRACT_NOTARY_KEY_ID and REFRACT_NOTARY_ISSUER)" >&2; exit 2; }

# The app's lifecycle tests (macOS only).
mac-test:
	cd macos && swift test --scratch-path ../build/mac/swift

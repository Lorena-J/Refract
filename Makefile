.PHONY: dev dev-api dev-web build web test

# Go API on :8080 + Vite on :5173 (proxies /api). Open http://localhost:5173.
dev:
	$(MAKE) -j2 dev-api dev-web

dev-api:
	REFRACT_AUTH=none REFRACT_DATA_DIR=./data go run ./cmd/refract

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

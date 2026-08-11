VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: dev dev-backend dev-ui ui build release test vet clean

## Development ---------------------------------------------------------------

dev: ## run mock backend + vite dev server (two processes)
	@$(MAKE) -j2 dev-backend dev-ui

dev-backend:
	go run ./cmd/control-panel --mock --config deploy/config.dev.yaml

dev-ui:
	cd web && npm run dev

## Build ---------------------------------------------------------------------

ui: ## build frontend into the embed directory
	cd web && npm ci --silent && npm run build

build: ui ## production binary for the host platform
	go build -trimpath -ldflags '$(LDFLAGS)' -o bin/control-panel ./cmd/control-panel

release: ui ## static linux/amd64 binary for the Debian server
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -trimpath -ldflags '$(LDFLAGS)' \
		-o dist/control-panel-linux-amd64 ./cmd/control-panel

## Quality -------------------------------------------------------------------

test:
	go test ./...
	cd web && npx tsc --noEmit

vet:
	go vet ./...

clean:
	rm -rf bin dist web/dist internal/httpd/ui/dist/*
	@touch internal/httpd/ui/dist/.gitkeep

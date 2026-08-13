VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: dev dev-backend dev-ui ui build release deploy test vet clean

# Server the panel runs on; override with:  make deploy DEPLOY_HOST=user@host
DEPLOY_HOST ?= ziggy@192.168.0.235

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

deploy: release ## build, install onto DEPLOY_HOST, restart the panel
	scp dist/control-panel-linux-amd64 $(DEPLOY_HOST):/tmp/control-panel-new
	ssh -t $(DEPLOY_HOST) "sudo install -m 0755 /tmp/control-panel-new /usr/local/bin/control-panel \
		&& rm /tmp/control-panel-new && sudo systemctl restart control-panel \
		&& systemctl is-active control-panel"

## macOS menu bar app --------------------------------------------------------

mac-bar: ## build the menu bar companion (dist/Panel Bar.app)
	cd macbar && swift build -c release
	rm -rf "dist/Panel Bar.app"
	mkdir -p "dist/Panel Bar.app/Contents/MacOS"
	cp macbar/.build/release/PanelBar "dist/Panel Bar.app/Contents/MacOS/PanelBar"
	cp macbar/Info.plist "dist/Panel Bar.app/Contents/Info.plist"
	codesign --force --sign - "dist/Panel Bar.app"
	@echo 'Built dist/Panel Bar.app — copy to /Applications and open it.'

## Quality -------------------------------------------------------------------

test:
	go test ./...
	cd web && npx tsc --noEmit

vet:
	go vet ./...

clean:
	rm -rf bin dist web/dist internal/httpd/ui/dist/*
	@touch internal/httpd/ui/dist/.gitkeep

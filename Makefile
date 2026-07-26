.PHONY: install install-view install-all test test-web test-all web-install web-build view


# Installs only the LSP. It has no Node.js or React dependency.
install:
	go install ./cmd/neva-lsp

# Builds the shared browser UI, embeds it, then installs the standalone host.
install-view: web-build
	go install ./cmd/neva-view

install-all: install install-view

test:
	go test ./...

test-web: web-install
	cd web && npm test

test-all: test test-web

web-install:
	cd web && npm ci

web-build: web-install
	cd web && npm run build

view: install-view
	neva-view --port=7792 --workspace=.

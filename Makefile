.PHONY: install install-view test test-web test-all web-install web-build view

install: web-build
	go install ./cmd/neva-lsp

install-view: web-build
	go install ./cmd/neva-view

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

GO ?= go

.PHONY: css build test run

# Tailwind v4: `npx tailwindcss` no longer exists; the pinned binary is @tailwindcss/cli.
css:
	npx @tailwindcss/cli -i internal/ui/static/input.css -o internal/ui/static/app.css --minify

# CSS is generated before the Go build so the embedded assets are current.
build: css
	CGO_ENABLED=0 $(GO) build -o bin/tracker ./cmd/tracker

test:
	$(GO) test ./...

run: build
	./bin/tracker serve

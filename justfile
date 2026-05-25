set shell := ["bash", "-cu"]
set dotenv-load := false

default:
    @just --list

# --- Dev ---

dev:
    #!/usr/bin/env bash
    set -euo pipefail
    just server &
    SERVER_PID=$!
    trap "kill $SERVER_PID 2>/dev/null || true" EXIT
    just client

server:
    cd server && go run ./cmd/forza-telemetry serve

client:
    cd client && pnpm dev

# --- Build ---

build: gen-types build-client copy-client-dist build-server

build-client:
    cd client && pnpm install && pnpm build

# TanStack Start emits dist/client/ (browser assets) and dist/server/ (SSR runtime).
# In SPA-embed deploy mode we only ship the client assets — copy them into the
# Go embed target so `go:embed all:dist` picks them up.
copy-client-dist:
    rm -rf server/internal/web/dist
    mkdir -p server/internal/web/dist
    cp -R client/dist/client/. server/internal/web/dist/
    touch server/internal/web/dist/.gitkeep

build-server:
    cd server && CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o bin/forza-telemetry ./cmd/forza-telemetry

# --- Schema ---

gen-types:
    cd server && go run ./cmd/gen-types > ../client/src/types/tick.generated.ts

# --- Quality ---

test:
    cd server && CGO_ENABLED=1 go test ./...
    cd client && pnpm lint

lint:
    cd server && go vet ./...
    cd client && pnpm lint

fmt:
    cd server && gofmt -w .
    cd client && pnpm format

# --- Clean ---

clean:
    rm -rf server/bin server/internal/web/dist/* client/dist client/node_modules

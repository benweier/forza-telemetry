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
    # Wait for the Go server to accept requests before starting Vite, so the
    # client's telemetry WS doesn't race server startup (proxy ECONNRESET noise).
    # First `go run` compiles, so allow up to ~30s; start the client regardless
    # after that (LiveSocket retries anyway).
    echo "waiting for server on :8080…"
    for _ in $(seq 1 60); do
      curl -sf http://localhost:8080/healthz >/dev/null 2>&1 && { echo "server ready"; break; }
      sleep 0.5
    done
    just client

server:
    cd server && go run ./cmd/forza-telemetry serve

client:
    cd client && pnpm dev

# WebRTC media relay for the in-UI game preview (ADR 0010).
# OBS (game PC) pushes WHIP in; the browser plays WHEP out. We pass mediamtx.yml
# (a 2-line catch-all path config) — the built-in empty config rejects arbitrary
# paths ("path 'mystream' is not configured"). Everything else stays default.
# Override the binary path if it's not on PATH:  just media MEDIAMTX=/path/to/mediamtx
#
#   OBS    -> Settings/Stream/Service "WHIP", Server: http://<this-host-lan-ip>:8889/mystream/whip
#   Browser <- WHEP read URL:                          http://<this-host-lan-ip>:8889/mystream/whep
#
# Run on whichever box both OBS and the viewer can reach. NOT folded into `just dev`
# on purpose: in the common split topology (game PC + server on another machine) the
# relay lives on the game PC, not the dev box. For the all-on-one-PC case use `dev-local`.
# Resolution order: MEDIAMTX override → `mediamtx` on PATH → a bundled ./mediamtx_*/mediamtx.
MEDIAMTX := "mediamtx"
media:
    #!/usr/bin/env bash
    set -euo pipefail
    bin="{{MEDIAMTX}}"
    command -v "$bin" >/dev/null 2>&1 || [ -x "$bin" ] || bin="$(ls mediamtx_*/mediamtx 2>/dev/null | head -1 || true)"
    if [ -z "$bin" ] || { ! command -v "$bin" >/dev/null 2>&1 && [ ! -x "$bin" ]; }; then
      echo "mediamtx not found (PATH or ./mediamtx_*/mediamtx). Set MEDIAMTX=/path/to/mediamtx"; exit 1
    fi
    exec "$bin" "{{justfile_directory()}}/mediamtx.yml"

# Single-machine dev (topology C): game + OBS + server + client + relay all on one box.
dev-local:
    #!/usr/bin/env bash
    set -euo pipefail
    just MEDIAMTX="{{MEDIAMTX}}" media &
    MEDIA_PID=$!
    trap "kill $MEDIA_PID 2>/dev/null || true" EXIT
    just dev

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

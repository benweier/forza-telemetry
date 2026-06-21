import tailwindcss from "@tailwindcss/vite";
import { tanstackStart } from "@tanstack/react-start/plugin/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [tailwindcss(), tanstackStart({ spa: { enabled: true } }), react()],
  resolve: {
    tsconfigPaths: true,
  },
  build: {
    // Suppress rolldown's INVALID_ANNOTATION spam from HeroUI Pro's
    // pre-minified `/*#__PURE__*/` comments — they sit at positions
    // rolldown's strict parser ignores. Non-fatal (build still succeeds);
    // just noisy. Filter so real errors remain visible.
    rolldownOptions: {
      onwarn(warning, defaultHandler) {
        if (warning.code === "INVALID_ANNOTATION") return;
        defaultHandler(warning);
      },
    },
  },
  server: {
    port: 3000,
    proxy: {
      // The Go server hosts UDP ingest + REST + WS on :8080. CORS is not
      // wired (single-origin in prod via go:embed). In dev we proxy the
      // REST + WS + health paths from Vite :3000 → Go :8080.
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
        ws: true,
        // Swallow benign WebSocket-teardown noise. The live telemetry WS gets
        // torn down on every reload / HMR / route change; the proxy then logs
        // ECONNRESET ("ws proxy socket error") and EPIPE ("ws proxy error").
        // Vite attaches its own error loggers AFTER this `configure` runs, so we
        // can't unregister them — instead we wrap `emit` (here, first) to drop
        // those two codes before Vite's listeners see them. Real failures like
        // ECONNREFUSED (server down) still log loudly.
        configure: (proxy) => {
          const benign = (err: unknown) =>
            !!err && ["ECONNRESET", "EPIPE"].includes((err as NodeJS.ErrnoException).code ?? "");
          const muteErrors = (em: { emit: (event: string, ...args: unknown[]) => boolean }) => {
            const original = em.emit.bind(em);
            em.emit = (event, ...args) =>
              event === "error" && benign(args[0]) ? false : original(event, ...args);
          };
          muteErrors(proxy as never);
          // The per-socket error fires on the upgraded socket, not the proxy —
          // wrap it too, before Vite's proxyReqWs handler attaches its logger.
          proxy.on("proxyReqWs", (_proxyReq, _req, socket) => muteErrors(socket as never));
        },
      },
      "/healthz": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});

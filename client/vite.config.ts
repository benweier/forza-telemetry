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
    rollupOptions: {
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
      },
      "/healthz": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});


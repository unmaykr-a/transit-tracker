import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

/** Stamps a hash of the built index.html (which names every hashed asset) into dist/sw.js. */
const stampServiceWorker = () => ({
  name: "stamp-service-worker",
  apply: "build" as const,
  closeBundle() {
    const dist = path.resolve(import.meta.dirname, "dist");
    const hash = createHash("sha256").update(readFileSync(path.join(dist, "index.html"))).digest("hex").slice(0, 12);
    const sw = path.join(dist, "sw.js");
    writeFileSync(sw, readFileSync(sw, "utf8").replace("__BUILD_HASH__", hash));
  },
});

// The Go server embeds web/dist (copied to internal/api/dist). In dev, Vite
// proxies API and WebSocket traffic to a locally running `go run ./cmd/bus`.
export default defineConfig({
  plugins: [react(), tailwindcss(), stampServiceWorker()],
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "src") } },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8080",
      "/healthz": "http://127.0.0.1:8080",
      "/ws": { target: "ws://127.0.0.1:8080", ws: true },
    },
  },
  // MapLibre alone is ~1 MB and lazy-loaded with the map route.
  build: { outDir: "dist", emptyOutDir: true, chunkSizeWarningLimit: 1200 },
  worker: { format: "es" },
  test: {
    include: ["src/**/*.test.{ts,tsx}"],
    environment: "jsdom",
    setupFiles: ["./src/test-setup.ts"],
  },
});

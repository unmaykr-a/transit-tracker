import { defineConfig, devices } from "@playwright/test";

// End-to-end tests run the real Go binary (built by `make web build`) with
// TRAM_FAKE_FEEDS=1, so vehicle data is deterministic and offline. Map tiles
// still come from OpenFreeMap.
const port = 4173;
const upstreamProxy = process.env.HTTPS_PROXY; // only set in sandboxed dev environments

export default defineConfig({
  testDir: "e2e",
  timeout: 60_000,
  fullyParallel: true,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["github"], ["list"]] : "list",
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    trace: "retain-on-failure",
    ignoreHTTPSErrors: !!upstreamProxy,
    proxy: upstreamProxy ? { server: upstreamProxy, bypass: "127.0.0.1,localhost" } : undefined,
    launchOptions: {
      executablePath: process.env.PW_CHROMIUM_PATH || undefined,
      // Software WebGL so MapLibre renders on CI runners without a GPU.
      args: ["--use-angle=swiftshader", "--enable-unsafe-swiftshader"],
    },
  },
  projects: [
    { name: "mobile", use: { ...devices["Pixel 7"], browserName: "chromium" } },
    { name: "desktop", use: { ...devices["Desktop Chrome"] } },
  ],
  webServer: {
    command: "../bin/tram",
    url: `http://127.0.0.1:${port}/healthz`,
    reuseExistingServer: !process.env.CI,
    env: { TRAM_ADDR: `127.0.0.1:${port}`, TRAM_FAKE_FEEDS: "1", TRAM_LOG_LEVEL: "warn" },
  },
});

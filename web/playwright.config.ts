import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { defineConfig, devices } from "@playwright/test";

// End-to-end tests run the real Go binary (built by `make web build`) with
// BUS_FAKE_FEEDS=1, so vehicle data is deterministic and offline. Map tiles
// still come from OpenFreeMap.
const port = 4173;
const idpPort = 4174;
// A fresh database per run, with a seeded admin so tests never land on /setup.
export const E2E_ADMIN = { username: "admin", password: "e2e-admin-password" };
const dataDir =
  process.env.BUS_E2E_DATA_DIR ?? mkdtempSync(join(tmpdir(), "bus-e2e-"));
process.env.BUS_E2E_DATA_DIR = dataDir; // workers re-evaluate this file; keep one directory
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
    proxy: upstreamProxy
      ? { server: upstreamProxy, bypass: "127.0.0.1,localhost" }
      : undefined,
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
  webServer: [
    // A fake OpenID provider, so single sign-on and account linking run for real.
    {
      command: `node e2e/fake-idp.mjs`,
      url: `http://127.0.0.1:${idpPort}/healthz`,
      reuseExistingServer: false,
      env: { IDP_PORT: String(idpPort) },
    },
    {
      command: "../bin/bus",
      url: `http://127.0.0.1:${port}/healthz`,
      reuseExistingServer: false,
      env: {
        BUS_ADDR: `127.0.0.1:${port}`,
        BUS_FAKE_FEEDS: "1",
        BUS_LOG_LEVEL: "warn",
        BUS_DATA_DIR: dataDir,
        BUS_ADMIN_USER: E2E_ADMIN.username,
        BUS_ADMIN_PASSWORD: E2E_ADMIN.password,
        BUS_BASE_URL: `http://127.0.0.1:${port}`,
        BUS_OIDC_ISSUER: `http://127.0.0.1:${idpPort}`,
        BUS_OIDC_CLIENT_ID: "bus-e2e",
        BUS_OIDC_CLIENT_SECRET: "e2e-secret",
        BUS_OIDC_NAME: "Test IdP",
        BUS_OIDC_AUTO_CREATE: "false",
      },
    },
  ],
});

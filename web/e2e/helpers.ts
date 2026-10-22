import type { APIRequestContext, Locator, Page } from "@playwright/test";

import { E2E_ADMIN } from "../playwright.config";

export interface RenderedVehicle {
  id: string;
  mode: string;
  text: string;
  x: number;
  y: number;
}

/** Waits until the map has drawn at least min vehicle pills and returns them with screen positions. */
export async function renderedPills(page: Page, min = 5): Promise<RenderedVehicle[]> {
  await page.waitForFunction(
    (n) => {
      const m = (globalThis as { __tramMap?: { queryRenderedFeatures: (o: object) => unknown[] } }).__tramMap;
      return !!m && m.queryRenderedFeatures({ layers: ["vehicle-pills"] }).length >= n;
    },
    min,
    { timeout: 30_000 },
  );
  return page.evaluate(() => {
    type F = { properties: { id: string; mode: string; text: string }; geometry: { coordinates: [number, number] } };
    const m = (globalThis as unknown as {
      __tramMap: {
        queryRenderedFeatures: (o: object) => F[];
        project: (c: [number, number]) => { x: number; y: number };
        getCanvas: () => HTMLCanvasElement;
      };
    }).__tramMap;
    // project() is relative to the map canvas; on desktop the nav rail shifts it right.
    const box = m.getCanvas().getBoundingClientRect();
    return m.queryRenderedFeatures({ layers: ["vehicle-pills"] }).map((f) => {
      const p = m.project(f.geometry.coordinates);
      return { id: f.properties.id, mode: f.properties.mode, text: f.properties.text, x: box.left + p.x, y: box.top + p.y };
    });
  });
}

/** Map sheets open compact on phones; expand one if it is. */
export async function expandSheet(sheet: Locator) {
  const more = sheet.getByRole("button", { name: "Show details" });
  if (await more.isVisible()) await more.click();
}

const csrfOf = async (req: APIRequestContext): Promise<string> => (await (await req.get("/api/v1/session")).json()).csrf;

/**
 * Signs `req` (a fresh browser context's request) up as a new user through an
 * admin invite, so tests that change saved lines don't race each other.
 */
export async function signUpFreshUser(req: APIRequestContext, admin: APIRequestContext, prefix: string): Promise<string> {
  if ((await admin.post("/api/v1/auth/login", { data: E2E_ADMIN })).status() !== 200) throw new Error("admin login failed");
  const inv = await (await admin.post("/api/v1/admin/invites", { headers: { "X-CSRF-Token": await csrfOf(admin) }, data: {} })).json();
  const username = `${prefix}-${Date.now() % 1_000_000}`;
  const res = await req.post("/api/v1/auth/signup", { data: { username, password: "e2e-user-password", invite: inv.token } });
  if (res.status() !== 200) throw new Error(`signup failed: ${res.status()} ${await res.text()}`);
  return username;
}

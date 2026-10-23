import { expect, request, test } from "@playwright/test";

import { E2E_ADMIN } from "../playwright.config";

test("API token: create in settings, read /ha/lines with it, revoke as admin", async ({ page, baseURL }) => {
  await page.goto("/login");
  await page.getByLabel("Username").fill(E2E_ADMIN.username);
  await page.getByLabel("Password").fill(E2E_ADMIN.password);
  await page.getByRole("button", { name: "Log in" }).click();
  await expect(page.getByRole("heading", { name: "My lines" })).toBeVisible();

  await page.goto("/settings");
  const section = page.getByRole("region", { name: "Integrations" });
  const name = `HA ${Date.now()}`;
  await section.getByLabel("Name").fill(name);
  await section.getByRole("button", { name: "Create token" }).click();
  const field = section.getByLabel("New API token");
  await expect(field).toHaveValue(/^bus_/);
  const token = await field.inputValue();
  await expect(section.getByRole("list", { name: "API tokens" }).getByText(name)).toBeVisible();

  // The token works without cookies, read-only.
  const api = await request.newContext({ baseURL, extraHTTPHeaders: { Authorization: `Bearer ${token}` } });
  const res = await api.get("/api/v1/ha/lines?origin=59.4405,24.7375");
  expect(res.status()).toBe(200);
  expect(Array.isArray((await res.json()).lines)).toBe(true);
  expect((await api.post("/api/v1/me/lines", { data: {} })).status()).toBe(403);

  // The admin page lists it; revoking cuts it off.
  await page.goto("/admin/tokens");
  const row = page.getByRole("listitem").filter({ hasText: name });
  await row.getByRole("button", { name: "Revoke" }).click();
  await row.getByRole("button", { name: "Confirm" }).click();
  await expect(row).toHaveCount(0);
  expect((await api.get("/api/v1/ha/lines")).status()).toBe(401);
  await api.dispose();
});

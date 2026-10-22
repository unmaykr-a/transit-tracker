import { expect, test } from "@playwright/test";

import { E2E_ADMIN } from "../playwright.config";

test("Trips: pick two stops, see the lines between them, save the trip", async ({ browser, baseURL }) => {
  const ctx = await browser.newContext({ geolocation: { latitude: 59.4405, longitude: 24.7375 }, permissions: ["geolocation"], baseURL });
  const page = await ctx.newPage();
  await page.goto("/login");
  await page.getByLabel("Username").fill(E2E_ADMIN.username);
  await page.getByLabel("Password").fill(E2E_ADMIN.password);
  await page.getByRole("button", { name: "Log in" }).click();
  await expect(page.getByRole("heading", { name: "My lines" })).toBeVisible();
  const csrf: string = (await (await page.request.get("/api/v1/session")).json()).csrf;
  for (const l of (await (await page.request.get("/api/v1/me/lines")).json()).lines) {
    await page.request.delete(`/api/v1/me/lines/${l.id}`, { headers: { "X-CSRF-Token": csrf } });
  }

  // Two stops a few apart on T1's main pattern.
  const line = await (await page.request.get("/api/v1/lines/tram/T1")).json();
  const stops: { area_id: string; name: string }[] = line.patterns.find((p: { main: boolean }) => p.main).stops;
  const [a, b] = [stops[1]!, stops[4]!];

  // Start from the stop page's "Trip from here".
  await page.goto(`/stops/${a.area_id}`);
  await page.getByRole("link", { name: "Trip from here" }).click();
  await expect(page.getByRole("heading", { name: "Trip between two stops" })).toBeVisible();
  await expect(page.getByText(a.name, { exact: true })).toBeVisible();

  await page.getByLabel("To").fill(b.name);
  await page.getByRole("list", { name: "To: results" }).getByRole("button", { name: new RegExp(b.name) }).first().click();
  const found = page.getByRole("region", { name: "Lines that ride this trip" });
  await expect(found.getByRole("listitem").first()).toBeVisible();
  await expect(found.getByText("T1", { exact: true }).first()).toBeVisible();

  await page.getByRole("button", { name: "Save trip" }).click();
  await expect(page.getByRole("heading", { name: "My lines" })).toBeVisible();
  const card = page.getByRole("article");
  await expect(card).toHaveCount(1);
  await expect(card).toHaveAttribute("aria-label", new RegExp(`${a.name} → ${b.name}`));
  await expect(card.getByText(/^(now|\d+ min)$/).first()).toBeVisible();
  await ctx.close();
});

import { expect, test } from "@playwright/test";

import { E2E_ADMIN } from "../playwright.config";

test("My lines: save three lines and see countdowns with verdicts", async ({ browser, baseURL }) => {
  // Standing next to Balti jaam with location allowed.
  const ctx = await browser.newContext({ geolocation: { latitude: 59.4405, longitude: 24.7375 }, permissions: ["geolocation"], baseURL });
  const page = await ctx.newPage();
  await page.goto("/login");
  await page.getByLabel("Username").fill(E2E_ADMIN.username);
  await page.getByLabel("Password").fill(E2E_ADMIN.password);
  await page.getByRole("button", { name: "Log in" }).click();
  await expect(page.getByRole("heading", { name: "My lines" })).toBeVisible();

  // Start from an empty list (tests share one database).
  const csrf: string = (await (await page.request.get("/api/v1/session")).json()).csrf;
  for (const l of (await (await page.request.get("/api/v1/me/lines")).json()).lines) {
    await page.request.delete(`/api/v1/me/lines/${l.id}`, { headers: { "X-CSRF-Token": csrf } });
  }

  // Save tram T1 through the UI: line page → Save line → boarding stop → Save.
  await page.goto("/lines/tram/T1");
  await page.getByRole("button", { name: "Save line" }).click();
  const sheet = page.getByRole("region", { name: /Save T1/ });
  await expect(sheet.getByRole("radio", { checked: true })).toBeVisible(); // nearest stop preselected
  await sheet.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByRole("heading", { name: "My lines" })).toBeVisible();

  // Two more through the API.
  for (const route of ["bus:24", "trolley:72"]) {
    const line = await (await page.request.get(`/api/v1/lines/${route.replace(":", "/")}`)).json();
    const main = line.patterns.find((p: { main: boolean }) => p.main);
    const res = await page.request.post("/api/v1/me/lines", {
      headers: { "X-CSRF-Token": csrf },
      data: { route_id: route, direction: main.direction, board_stop_id: main.stops[1].id },
    });
    expect(res.status()).toBe(201);
  }

  await page.reload();
  const cards = page.getByRole("article");
  await expect(cards).toHaveCount(3);
  for (const card of await cards.all()) {
    await expect(card.getByText(/^(now|\d+ min)$/).first()).toBeVisible(); // countdown
    await expect(card.getByText(/min walk/)).toBeVisible();
    await expect(card.getByText(/^(Leave in \d+ min|Leave now|Too tight.*)$/)).toBeVisible(); // verdict chip
  }

  // Reorder: move the first card down.
  const firstTitle = await cards.first().getAttribute("aria-label");
  await page.getByRole("button", { name: "Edit" }).click();
  await cards.first().getByRole("button", { name: "Move down" }).click();
  await expect(cards.nth(1)).toHaveAttribute("aria-label", firstTitle!);
  await ctx.close();
});

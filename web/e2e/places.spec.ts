import { expect, test } from "@playwright/test";

import { signUpFreshUser } from "./helpers";

test("add a saved place by address; it shows on the live map", async ({ browser, baseURL }, info) => {
  const admin = await browser.newContext({ baseURL });
  const ctx = await browser.newContext({ baseURL });
  await signUpFreshUser(ctx.request, admin.request, `places-${info.project.name}`);
  const page = await ctx.newPage();
  await page.goto("/settings#places");
  await page.getByPlaceholder("Home, Work…").fill("Gym");
  await page.getByLabel("or search an address or stop").fill("Pärnu mnt 20");
  // Fake feeds answer the address search from a recorded In-ADS response; Tallinn comes first.
  const results = page.getByRole("list", { name: "Addresses and stops" });
  await results.getByRole("button", { name: /Pärnu mnt 20.*Tallinn/ }).first().click();
  await expect(page.locator("#places").getByText("Gym", { exact: true })).toBeVisible();

  await page.goto("/map");
  await page.waitForFunction(() => {
    const m = (globalThis as { __tramMap?: { querySourceFeatures: (s: string) => { properties: { label: string } }[] } }).__tramMap;
    return !!m && m.querySourceFeatures("places").some((f) => f.properties.label === "Gym");
  });
  await ctx.close();
  await admin.close();
});

test("add a place from Home; it becomes the walking origin", async ({ browser, baseURL }, info) => {
  const admin = await browser.newContext({ baseURL });
  const ctx = await browser.newContext({ baseURL });
  await signUpFreshUser(ctx.request, admin.request, `homeplace-${info.project.name}`);
  const csrf = (await (await ctx.request.get("/api/v1/session")).json()).csrf;
  const line = await (await ctx.request.get("/api/v1/lines/tram/T4")).json();
  const main = line.patterns.find((p: { main: boolean }) => p.main);
  expect((await ctx.request.post("/api/v1/me/lines", { headers: { "X-CSRF-Token": csrf }, data: { route_id: "tram:T4", direction: main.direction, board_stop_id: main.stops[1].id } })).status()).toBe(201);

  const page = await ctx.newPage();
  await page.goto("/");
  await page.getByRole("button", { name: "Add place" }).click();
  const panel = page.getByRole("region", { name: "Add place" });
  await expect(panel.getByPlaceholder("Home, Work…")).toBeFocused();
  await panel.getByPlaceholder("Home, Work…").fill("Office");
  await panel.getByLabel("or search an address or stop").fill("Pärnu mnt 20");
  await panel.getByRole("list", { name: "Addresses and stops" }).getByRole("button", { name: /Pärnu mnt 20.*Tallinn/ }).first().click();
  await expect(panel).toBeHidden();
  await expect(page.getByRole("radio", { name: "Office" })).toHaveAttribute("aria-checked", "true");
  await expect(page.getByRole("article").getByText(/min walk/)).toBeVisible();
  await ctx.close();
  await admin.close();
});

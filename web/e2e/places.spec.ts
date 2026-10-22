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

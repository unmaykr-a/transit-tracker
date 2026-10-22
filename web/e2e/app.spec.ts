import { expect, test } from "@playwright/test";

test("navigation, language and theme", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "My lines" })).toBeVisible();
  const nav = page.getByRole("navigation", { name: "Main navigation" });
  await nav.getByRole("link", { name: "Settings" }).click();
  await expect(page).toHaveURL(/\/settings$/);

  await page.getByRole("radio", { name: "Dark" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");

  await page.getByRole("radio", { name: "Eesti" }).click();
  await expect(page.getByRole("heading", { name: "Seaded" })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("lang", "et");
  await page.getByRole("navigation", { name: "Peamenüü" }).getByRole("link", { name: "Kaart" }).click();
  await expect(page).toHaveURL(/\/map$/);
  await expect(page.getByText(/^\d+ sõidukit$/)).toBeVisible();

  // Preferences survive a reload.
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect(page.getByRole("button", { name: "Tramm" })).toBeVisible();
});

test("unknown API paths are JSON 404s, app routes fall back to the SPA", async ({ request }) => {
  const api = await request.get("/api/v1/nope");
  expect(api.status()).toBe(404);
  expect(await api.json()).toEqual({ error: "not found" });
  const spa = await request.get("/settings");
  expect(spa.status()).toBe(200);
  expect(await spa.text()).toContain('<div id="root">');
});

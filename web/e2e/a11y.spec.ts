import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

import { E2E_ADMIN } from "../playwright.config";

// WCAG 2.1 A/AA checks with axe on every main page, light and dark. The map
// canvas is excluded: it is a picture of tiles; its content is also reachable
// through search, stop and line pages.
async function scan(page: Page, label: string) {
  await page.waitForLoadState("networkidle");
  const res = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"]).exclude(".maplibregl-canvas").analyze();
  const problems = res.violations.map((v) => `${label}: [${v.impact}] ${v.id}: ${v.help} — ${v.nodes.map((n) => n.target.join(" ")).slice(0, 3).join(" | ")}`);
  expect(problems, problems.join("\n")).toEqual([]);
}

async function areaPath(page: Page): Promise<string> {
  const res = await (await page.request.get("/api/v1/search?q=balti")).json();
  return `/stops/${res.stops[0].id}`;
}

for (const scheme of ["light", "dark"] as const) {
  test.describe(`accessibility (${scheme})`, () => {
    test.use({ colorScheme: scheme });

    test("public pages", async ({ page }) => {
      for (const path of ["/map", "/search?q=balti", await areaPath(page), "/lines/tram/T1", "/login", "/settings"]) {
        await page.goto(path);
        await scan(page, path);
      }
    });

    test("signed-in pages", async ({ page }) => {
      await page.goto("/login");
      await page.getByLabel("Username").fill(E2E_ADMIN.username);
      await page.getByLabel("Password").fill(E2E_ADMIN.password);
      await page.getByRole("button", { name: "Log in" }).click();
      await expect(page.getByRole("heading", { name: "My lines" })).toBeVisible();
      for (const path of ["/", "/settings", "/admin/users", "/admin/invites", "/admin/tokens", "/admin/settings", "/admin/health", "/admin/audit", "/admin/about", "/trips/new"]) {
        await page.goto(path);
        await scan(page, path);
      }
    });
  });
}

test("keyboard only: search, open a stop, switch platform, open a line", async ({ page }) => {
  await page.goto("/search");
  const box = page.getByRole("searchbox", { name: "Search lines and stops" });
  await box.focus();
  await page.keyboard.type("balti jaam");
  const first = page.getByRole("link", { name: /^Balti jaam/ }).first();
  await expect(first).toBeVisible();
  // Tab from the search box until the first result has focus, then follow it.
  for (let i = 0; i < 10 && !(await first.evaluate((el) => el === document.activeElement)); i++) await page.keyboard.press("Tab");
  await expect(first).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: "Balti jaam" })).toBeVisible();

  const tab = page.getByRole("tablist").getByRole("tab", { name: /Platform A/ });
  await tab.focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/[?&]p=/);

  const line = page.getByRole("link", { name: /^T\d$/ }).first();
  await line.focus();
  await expect(line).toBeFocused();
  // A visible focus indicator (outline or ring) on the focused link.
  const ring = await line.evaluate((el) => {
    const s = getComputedStyle(el);
    return s.outlineStyle !== "none" || s.boxShadow !== "none";
  });
  expect(ring).toBe(true);
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/lines\/tram\/T\d/);
});

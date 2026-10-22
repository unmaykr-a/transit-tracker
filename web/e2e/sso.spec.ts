import { expect, test, type Browser } from "@playwright/test";

import { E2E_ADMIN } from "../playwright.config";

const IDP = "http://127.0.0.1:4174";

/** A fresh browser that is signed in at the fake provider as sub. */
async function atProvider(browser: Browser, baseURL: string | undefined, sub: string) {
  const ctx = await browser.newContext({ baseURL });
  const page = await ctx.newPage();
  await page.goto(`${IDP}/as?sub=${sub}`);
  return { ctx, page };
}

test("link the admin account to single sign-on, sign in with it, unlink", async ({ browser, baseURL }, info) => {
  const sub = `e2e-admin-${info.project.name}`;
  const { ctx, page } = await atProvider(browser, baseURL, sub);

  // Before linking, the provider account has no Bus account.
  await page.goto("/login");
  await page.getByRole("link", { name: "Sign in with Test IdP" }).click();
  await expect(page.getByText(/There is no account for this sign-in/)).toBeVisible();

  // Log in with the password and link from Settings.
  await page.getByLabel("Username").fill(E2E_ADMIN.username);
  await page.getByLabel("Password").fill(E2E_ADMIN.password);
  await page.getByRole("button", { name: "Log in" }).click();
  await expect(page.getByRole("heading", { name: "My lines" })).toBeVisible();
  await page.goto("/settings");
  await page.getByRole("button", { name: "Link Test IdP account" }).click();
  await expect(page.getByRole("status").filter({ hasText: "Linked." })).toBeVisible();
  const linked = page.getByRole("list", { name: "Single sign-on" });
  await expect(linked.getByText(`${sub}@example.com`)).toBeVisible();

  // A second browser signs straight in to the admin account through the provider.
  const other = await atProvider(browser, baseURL, sub);
  await other.page.goto("/login?next=/admin/users");
  await other.page.getByRole("link", { name: "Sign in with Test IdP" }).click();
  await expect(other.page).toHaveURL(/\/admin\/users$/);
  const me = await (await other.page.request.get("/api/v1/me")).json();
  expect([me.user.username, me.user.role]).toEqual([E2E_ADMIN.username, "admin"]);
  await other.ctx.close();

  // Unlinking ends that.
  await linked.getByRole("listitem").filter({ hasText: `${sub}@example.com` }).getByRole("button", { name: "Unlink" }).click();
  await expect(linked.getByText(`${sub}@example.com`)).toHaveCount(0);
  const again = await atProvider(browser, baseURL, sub);
  await again.page.goto("/login");
  await again.page.getByRole("link", { name: "Sign in with Test IdP" }).click();
  await expect(again.page.getByText(/There is no account for this sign-in/)).toBeVisible();
  await again.ctx.close();
  await ctx.close();
});

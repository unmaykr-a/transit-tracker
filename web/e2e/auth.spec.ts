import { expect, test, type Page } from "@playwright/test";

import { E2E_ADMIN } from "../playwright.config";

async function login(page: Page, username: string, password: string) {
  await page.goto("/login");
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Log in" }).click();
  await expect(page.getByRole("heading", { name: "My lines" })).toBeVisible();
}

test("admin invites a second user and can disable them", async ({ page, browser }, info) => {
  const username = `rider-${info.project.name}-${Date.now() % 100000}`;
  await login(page, E2E_ADMIN.username, E2E_ADMIN.password);

  // Create an invite link.
  await page.goto("/admin/invites");
  await page.getByRole("button", { name: "Create invite link" }).click();
  const link = await page.getByRole("textbox", { name: /Invite link/ }).inputValue();
  expect(link).toMatch(/\/signup\?invite=/);

  // The invitee signs up in a separate browser context.
  const other = await browser.newContext();
  const invitee = await other.newPage();
  await invitee.goto(link);
  await expect(invitee.getByText("This invite creates a user account.")).toBeVisible();
  await invitee.getByLabel("Username").fill(username);
  await invitee.getByLabel("Password").fill("rider-password-1");
  await invitee.getByRole("button", { name: "Create account" }).click();
  await expect(invitee.getByRole("heading", { name: "My lines" })).toBeVisible();
  await invitee.goto("/settings");
  await expect(invitee.getByText(`Signed in as ${username}`)).toBeVisible();
  // Not an admin.
  await expect(invitee.getByRole("link", { name: "Administration" })).toHaveCount(0);

  // The invite shows as used; the admin disables the new user.
  await page.reload();
  await expect(page.getByText(`used by ${username}`)).toBeVisible();
  await page.goto("/admin/users");
  const row = page.getByRole("listitem", { name: username });
  await expect(row.getByText("Active")).toBeVisible();
  await row.getByRole("button", { name: "Disable" }).click();
  await expect(row.getByText("Disabled")).toBeVisible();

  // The disabled user is signed out and cannot log in again.
  await invitee.reload();
  await expect(invitee.getByText("You are not logged in.")).toBeVisible();
  await invitee.goto("/login");
  await invitee.getByLabel("Username").fill(username);
  await invitee.getByLabel("Password").fill("rider-password-1");
  await invitee.getByRole("button", { name: "Log in" }).click();
  await expect(invitee.getByRole("alert")).toHaveText("invalid username or password");

  // Both actions are in the audit log.
  await page.goto("/admin/audit");
  const log = page.getByRole("list");
  await expect(log.getByText("user.disable").first()).toBeVisible();
  await expect(log.getByText("user.signup").first()).toBeVisible();
  await other.close();
});

test("admin pages: health shows both feeds, settings save and reset", async ({ page }) => {
  await login(page, E2E_ADMIN.username, E2E_ADMIN.password);
  await page.goto("/admin/health");
  await expect(page.getByRole("region", { name: "gps" })).toBeVisible();
  await expect(page.getByRole("region", { name: "elron" })).toBeVisible();

  await page.goto("/admin/settings");
  const name = page.getByLabel("Instance name");
  await name.fill("Bus e2e");
  await page.getByRole("button", { name: "Save" }).click();
  await expect(page.getByText("default: Bus")).toBeVisible();
  await expect(page).toHaveTitle("Bus e2e");
  await page.getByRole("button", { name: "Reset to default" }).click();
  await expect(page.getByText("default: Bus")).toHaveCount(0);
  await expect(name).toHaveValue("Bus");

  await page.goto("/admin/about");
  await expect(page.getByRole("link", { name: "Download backup" })).toHaveAttribute("href", "/api/v1/admin/backup");
});

test("wrong password shows an error", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Username").fill(E2E_ADMIN.username);
  await page.getByLabel("Password").fill("definitely-wrong");
  await page.getByRole("button", { name: "Log in" }).click();
  await expect(page.getByRole("alert")).toHaveText("invalid username or password");
});

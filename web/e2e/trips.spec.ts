import { expect, test } from "@playwright/test";

import { signUpFreshUser } from "./helpers";

test("Trips: pick two stops, see the lines between them, save the trip", async ({ browser, baseURL }, info) => {
  // Its own user: other tests clear the admin's saved lines.
  const admin = await browser.newContext({ baseURL });
  const ctx = await browser.newContext({ geolocation: { latitude: 59.4405, longitude: 24.7375 }, permissions: ["geolocation"], baseURL });
  const user = await signUpFreshUser(ctx.request, admin.request, `trips-${info.project.name}`);
  const page = await ctx.newPage();
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "My lines" })).toBeVisible();

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

  // Without location, a usual starting point gives the trip its walk; the trip is editable.
  const noGps = await browser.newContext({ baseURL, storageState: await ctx.storageState() });
  await ctx.close();
  const placeName = `Home ${info.project.name}`;
  const res = await noGps.request.post("/api/v1/me/places", {
    headers: { "X-CSRF-Token": (await (await noGps.request.get("/api/v1/session")).json()).csrf },
    data: { label: placeName, lat: 59.4405, lon: 24.7375 },
  });
  expect(res.status()).toBe(201);
  const p2 = await noGps.newPage();
  await p2.goto("/");
  await expect(p2.getByRole("heading", { name: "My lines" })).toBeVisible();
  await p2.getByRole("button", { name: "Edit" }).click();
  await p2.getByRole("link", { name: "Edit trip" }).first().click();
  await expect(p2.getByRole("heading", { name: "Edit trip" })).toBeVisible();
  await p2.getByLabel("Label (optional)").fill("Work run");
  await p2.getByRole("radio", { name: placeName }).click();
  await p2.getByRole("button", { name: "Save changes" }).click();
  const edited = p2.getByRole("article", { name: "Work run" });
  await expect(edited).toBeVisible();
  await expect(edited.getByText(new RegExp(`min walk from ${placeName}`))).toBeVisible();
  await noGps.close();
  await admin.close();
  expect(user).toMatch(/^trips-/);
});

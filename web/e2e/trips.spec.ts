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
  const found = page.getByRole("region", { name: "How to ride" });
  await expect(found.getByRole("radio", { name: /Direct/ })).toHaveAttribute("aria-checked", "true");
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

test("Trips with a change: no line rides directly, so the form offers ways with a change", async ({ browser, baseURL }, info) => {
  const admin = await browser.newContext({ baseURL });
  // No location: the card follows the next departure (fake feeds only list the next hour of departures).
  const ctx = await browser.newContext({ baseURL });
  await signUpFreshUser(ctx.request, admin.request, `change-${info.project.name}`);
  const page = await ctx.newPage();

  // Two stops of the fake city no single line connects, found through the API.
  const opts = await (await page.request.get("/api/v1/trips?from=a13012&to=a2947")).json();
  expect(opts.lines).toHaveLength(0);
  expect(opts.options.length).toBeGreaterThan(0);

  await page.goto("/trips/new?from=a13012");
  await expect(page.getByText(opts.from.name, { exact: true })).toBeVisible();
  await page.getByLabel("To").fill(opts.to.name);
  await page.getByRole("list", { name: "To: results" }).getByRole("button", { name: new RegExp(opts.to.name) }).first().click();

  const how = page.getByRole("region", { name: "How to ride" });
  await expect(how.getByText("No line rides directly between these stops — pick a way with a change.")).toBeVisible();
  await expect(how.getByRole("radio", { name: /Direct/ })).toHaveCount(0);
  const first = how.getByRole("radio").first();
  await expect(first).toHaveAttribute("aria-checked", "true");
  await expect(first).toContainText(/1 change/);
  const changeStop = opts.options[0].transfers[0].alight.name as string;
  await expect(how.getByText(`Change at ${changeStop}`).or(how.getByText(new RegExp(`walk \\d+ m from ${changeStop}`)))).toBeVisible();

  // Choosing a change stop by hand that does not work says why.
  await how.getByRole("radio", { name: "Choose where to change" }).click();
  await page.getByLabel("Change 1 at").fill(opts.from.name);
  await page.getByRole("list", { name: "Change 1 at: results" }).getByRole("button").first().click();
  await expect(page.getByRole("alert")).toBeVisible();
  await first.click();

  await page.getByRole("button", { name: "Save trip" }).click();
  await expect(page.getByRole("heading", { name: "My lines" })).toBeVisible();
  const card = page.getByRole("article");
  await expect(card).toHaveCount(1);
  await expect(card.getByText(`via ${changeStop}`)).toBeVisible();
  await expect(card.getByRole("list", { name: "Journey" })).toContainText(/Arrive ≈ \d\d:\d\d|Later rides not known yet/);
  await expect(card.getByRole("list", { name: "Journey" })).toContainText(`at ${opts.options[0].transfers[0].board.name}`);
  await ctx.close();
  await admin.close();
});

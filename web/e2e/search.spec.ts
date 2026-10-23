import { expect, test } from "@playwright/test";

import { expandSheet } from "./helpers";

test.describe("search, stops and lines", () => {
  test("searching Balti jaam shows city platforms and Elron departures", async ({ page }) => {
    await page.goto("/search");
    await page.getByRole("searchbox", { name: "Search lines, stops and places" }).fill("balti jaam");
    const stop = page.getByRole("link", { name: /^Balti jaam/ }).first();
    await expect(stop).toBeVisible();
    await stop.click();

    await expect(page.getByRole("heading", { name: "Balti jaam" })).toBeVisible();
    const tabs = page.getByRole("tablist");
    await expect(tabs.getByRole("tab", { name: /Platform A/ })).toBeVisible();
    // Departures from both trams (city, live) and trains (Elron, timetable).
    const list = page.getByRole("list").filter({ has: page.locator("li") }).last();
    await expect(list.locator("li").first()).toBeVisible();
    await expect(page.getByText("scheduled").first()).toBeVisible();
    await expect(page.getByRole("link", { name: /^T\d$/ }).first()).toBeVisible();
    await expect(page.getByRole("link", { name: /^R\d+$/ }).first()).toBeVisible();

    // Platform tabs filter the board.
    await tabs.getByRole("tab", { name: /Platform A/ }).click();
    await expect(page).toHaveURL(/[?&]p=/);
    await expect(page.getByText(/Platform [B-Z] ·/)).toHaveCount(0);
  });

  test("a place name finds the stops near it", async ({ page }) => {
    // Fake feeds answer every address search with Pärnu mnt 20 (a recorded In-ADS response).
    await page.goto("/search?q=stockmann");
    const places = page.getByRole("region", { name: "Places" }).or(page.locator("section").filter({ hasText: "Places" }));
    await expect(places.getByText("Pärnu mnt 20").first()).toBeVisible();
    await expect(places.getByText("Nearest stops").first()).toBeVisible();
    const stop = places.getByRole("link").filter({ hasText: / m$/ }).first();
    await expect(stop).toBeVisible();
    await stop.click();
    await expect(page).toHaveURL(/\/stops\//);
  });

  test("diacritics are optional in stop search", async ({ page }) => {
    await page.goto("/search?q=ulemiste");
    await expect(page.getByRole("link", { name: /^Ülemiste/ }).first()).toBeVisible();
  });

  test("line page shows directions, ordered stops and a map", async ({ page }) => {
    await page.goto("/search?q=4");
    await page.getByRole("link", { name: /T4.*Tondi - Lennujaam/ }).click();
    await expect(page).toHaveURL(/\/lines\/tram\/T4$/);
    await expect(page.getByRole("heading", { name: "Tondi - Lennujaam" })).toBeVisible();
    const dirs = page.getByRole("radiogroup", { name: "Direction" }).getByRole("radio");
    await expect(dirs).toHaveCount(2);
    const firstStops = await page.getByRole("listitem").filter({ has: page.getByRole("link") }).allTextContents();
    await dirs.nth(1).click();
    await expect(dirs.nth(1)).toHaveAttribute("aria-checked", "true");
    const secondStops = await page.getByRole("listitem").filter({ has: page.getByRole("link") }).allTextContents();
    expect(secondStops.length).toBeGreaterThan(5);
    expect(secondStops[0]).not.toEqual(firstStops[0]);

    // A stop on the line opens its stop page.
    await page.getByRole("link", { name: /^Hobujaama/ }).first().click();
    await expect(page.getByRole("heading", { name: "Hobujaama" })).toBeVisible();
  });

  test("map deep link opens a stop sheet", async ({ page, request }) => {
    const res = await request.get("/api/v1/search?q=balti%20jaam");
    const areaId: string = (await res.json()).stops[0].id;
    const stop = await (await request.get(`/api/v1/stops/${areaId}`)).json();
    await page.goto(`/map?stop=${stop.platforms[0].id}`);
    const sheet = page.locator("section[aria-label^='Balti jaam']");
    await expect(sheet).toBeVisible();
    await expandSheet(sheet);
    await expect(sheet.getByRole("link", { name: "Open stop page" })).toBeVisible();
    await expect(page).toHaveURL(/stop=/);
  });
});

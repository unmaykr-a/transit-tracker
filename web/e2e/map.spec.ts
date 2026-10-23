import { expect, test } from "@playwright/test";

import { expandSheet, renderedPills } from "./helpers";

test.describe("live map", () => {
  test("shows vehicles from the feed and opens a vehicle sheet", async ({ page }) => {
    // Style mistakes (a bad expression) only show up as console errors: the layer is silently dropped.
    const styleErrors: string[] = [];
    page.on("console", (m) => {
      if (m.type() === "error" && /layers\.|source/.test(m.text())) styleErrors.push(m.text());
    });
    await page.goto("/map");
    await expect(page.getByText(/\d{3} vehicles/)).toBeVisible();
    const pills = await renderedPills(page);
    // Pick a pill away from the overlays at the top and bottom of the screen.
    const vp = page.viewportSize()!;
    const target = pills.find((p) => p.y > 200 && p.y < vp.height - 250 && p.x > 120 && p.x < vp.width - 60) ?? pills[0]!;
    await page.mouse.click(target.x, target.y);
    const sheet = page.locator("section[aria-label]").filter({ hasText: "towards" });
    await expect(sheet).toBeVisible();
    await expandSheet(sheet);
    await expect(sheet).toContainText("Updated");
    await sheet.getByRole("button", { name: "Close" }).click();
    await expect(sheet).toBeHidden();
    // The selection drew its route (with every layer valid) and the railways are on the map.
    const layers = await page.evaluate(() => {
      const m = (globalThis as { __tramMap?: { getLayer: (id: string) => unknown } }).__tramMap!;
      return ["selected-route-casing", "selected-route", "selected-route-stops", "rail", "places"].filter((l) => !m.getLayer(l));
    });
    expect(layers).toEqual([]);
    expect(styleErrors).toEqual([]);
  });

  test("mode chips and line filter narrow the vehicles", async ({ page }) => {
    await page.goto("/map");
    const count = page.getByText(/^\d+ vehicles?$/);
    await expect(count).toHaveText(/^\d{3} vehicles$/);
    const all = Number((await count.textContent())!.split(" ")[0]);

    await page.getByRole("button", { name: "Bus", exact: true }).click();
    await expect(page.getByRole("button", { name: "Bus", exact: true })).toHaveAttribute("aria-pressed", "false");
    await expect.poll(async () => Number((await count.textContent())!.split(" ")[0])).toBeLessThan(all / 2);

    await page.getByPlaceholder("e.g. 4, 17A").fill("4");
    await expect.poll(async () => Number((await count.textContent())!.split(" ")[0])).toBeLessThan(20);
    // "4" matches tram T4 and trolleybus 4, never a bus once buses are off. The map
    // redraws on its next animation frame, so poll.
    await expect
      .poll(async () => {
        const pills = await renderedPills(page, 1);
        return pills.every((p) => (p.text === "T4" || p.text === "4") && p.mode !== "bus");
      })
      .toBe(true);
  });

  test("vehicles glide between feed updates", async ({ page }) => {
    await page.goto("/map");
    await renderedPills(page, 20);
    const position = () =>
      page.evaluate(() => {
        const m = (globalThis as unknown as { __tramMap: { getSource: (id: string) => { _data?: unknown; serialize: () => { data: { features: { id: string; geometry: { coordinates: number[] } }[] } } } } }).__tramMap;
        return Object.fromEntries(m.getSource("vehicles").serialize().data.features.map((f) => [f.id, f.geometry.coordinates.join(",")]));
      });
    const movedSince = async (prev: Record<string, string>) => {
      const cur = await position();
      return { cur, n: Object.keys(cur).filter((id) => prev[id] && prev[id] !== cur[id]).length };
    };
    // Nothing moves until the first diff after the snapshot (fake feed: every 10 s,
    // server poll: every 5 s). Wait for motion to start…
    let prev = await position();
    await expect
      .poll(async () => {
        const r = await movedSince(prev);
        prev = r.cur;
        return r.n;
      }, { timeout: 40_000, intervals: [1_000] })
      .toBeGreaterThan(30);
    // …then markers must keep gliding every second rather than jumping once per fix.
    const moved: number[] = [];
    for (let i = 0; i < 8; i++) {
      await page.waitForTimeout(1_000);
      const r = await movedSince(prev);
      moved.push(r.n);
      prev = r.cur;
    }
    expect(moved.filter((n) => n > 30).length, `vehicles moved per second: ${moved.join(", ")}`).toBeGreaterThanOrEqual(7);
  });
});

test("a matched vehicle's sheet lists its next stops", async ({ page, request }) => {
  await page.goto("/map");
  const pills = await renderedPills(page, 10);
  // Pick an on-screen vehicle the server has matched to a route pattern.
  const vehicles: { id: string; pattern_id?: string }[] = (await (await request.get("/api/v1/vehicles")).json()).vehicles;
  const matched = new Set(vehicles.filter((v) => v.pattern_id).map((v) => v.id));
  const vp = page.viewportSize()!;
  const target = pills.find((p) => matched.has(p.id) && p.y > 200 && p.y < vp.height - 250 && p.x > 120 && p.x < vp.width - 60);
  test.skip(!target, "no matched vehicle on screen");
  await page.mouse.click(target!.x, target!.y);
  const sheet = page.locator("section[aria-label]").filter({ hasText: "towards" });
  // On phones the sheet opens compact, with the next stop as its summary, so the map stays visible.
  if (page.viewportSize()!.width < 768) {
    await expect(sheet.getByRole("button", { name: /^Next: .*(now|\d+ min)$/ })).toBeVisible();
    const box = await sheet.boundingBox();
    expect(box!.height).toBeLessThan(page.viewportSize()!.height * 0.3);
  }
  await expandSheet(sheet);
  await expect(sheet.getByText("Next stops")).toBeVisible();
  await expect(sheet.getByRole("listitem").first()).toContainText(/(now|\d+ min)$/);
  await expect(sheet.getByRole("link", { name: "Open line" })).toBeVisible();
  // Its route is drawn with the stops along it.
  await page.waitForFunction(() => {
    const m = (globalThis as { __tramMap?: { querySourceFeatures: (s: string) => unknown[] } }).__tramMap;
    return !!m && m.querySourceFeatures("selected-route").length > 0 && m.querySourceFeatures("selected-route-stops").length > 1;
  });
});

test("railway lines are off until switched on in settings", async ({ page }) => {
  const railVisibility = () =>
    page.waitForFunction(() => {
      const m = (globalThis as { __tramMap?: { getLayer: (id: string) => unknown; getLayoutProperty: (id: string, p: string) => string } }).__tramMap;
      return m?.getLayer("rail") ? (m.getLayoutProperty("rail", "visibility") ?? "visible") : undefined;
    });
  await page.goto("/map");
  expect(await (await railVisibility()).jsonValue()).toBe("none");

  // Even with railway lines off, a selected train shows its own line (trains are matched to rail patterns).
  const trains = (await (await page.request.get("/api/v1/vehicles?mode=train")).json()).vehicles as { id: string; lat: number; lon: number; pattern_id?: string }[];
  const train = trains.find((v) => v.pattern_id);
  expect(train, "a fake train matched to a rail pattern").toBeTruthy();
  await page.evaluate(([lon, lat]) => (globalThis as unknown as { __tramMap: { jumpTo: (o: object) => void } }).__tramMap.jumpTo({ center: [lon, lat], zoom: 14 }), [train!.lon, train!.lat]);
  const pill = (await renderedPills(page, 1)).find((p) => p.id === train!.id);
  expect(pill, "the train's marker on screen").toBeTruthy();
  await page.mouse.click(pill!.x, pill!.y);
  await page.waitForFunction(() => {
    const m = (globalThis as { __tramMap?: { queryRenderedFeatures: (o: object) => unknown[] } }).__tramMap;
    return (m?.queryRenderedFeatures({ layers: ["selected-route"] }).length ?? 0) > 0;
  });

  await page.goto("/settings");
  const toggle = page.getByRole("switch", { name: "Railway lines on the map" });
  await expect(toggle).toHaveAttribute("aria-checked", "false");
  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-checked", "true");

  await page.goto("/map");
  await expect.poll(async () => (await railVisibility()).jsonValue()).toBe("visible");
});

test("follow a vehicle: the map keeps it in the centre until it is dragged", async ({ page }) => {
  await page.goto("/map");
  const pills = await renderedPills(page);
  const vp = page.viewportSize()!;
  const target = pills.find((p) => p.y > 200 && p.y < vp.height - 250 && p.x > 120 && p.x < vp.width - 60) ?? pills[0]!;
  await page.mouse.click(target.x, target.y);
  const sheet = page.locator("section[aria-label]").filter({ hasText: "towards" });
  const follow = sheet.getByRole("button", { name: "Follow on the map" });
  await follow.click();
  await expect(follow).toHaveAttribute("aria-pressed", "true");

  // Across feed updates the followed vehicle stays at the centre of the map.
  const offCentre = () =>
    page.evaluate((id) => {
      const m = (globalThis as unknown as {
        __tramMap: { queryRenderedFeatures: (o: object) => { properties: { id: string }; geometry: { coordinates: [number, number] } }[]; project: (c: [number, number]) => { x: number; y: number }; getCanvas: () => HTMLCanvasElement };
      }).__tramMap;
      const f = m.queryRenderedFeatures({ layers: ["vehicle-pills", "vehicle-pill-selected", "vehicle-dots"] }).find((x) => x.properties.id === id);
      if (!f) return Infinity;
      const p = m.project(f.geometry.coordinates);
      const c = m.getCanvas();
      return Math.hypot(p.x - c.clientWidth / 2, p.y - c.clientHeight / 2);
    }, target.id);
  await page.waitForTimeout(6000);
  expect(await offCentre()).toBeLessThan(30);

  // Dragging the map looks elsewhere: following stops.
  await page.mouse.move(vp.width / 2, vp.height / 3);
  await page.mouse.down();
  await page.mouse.move(vp.width / 2 + 150, vp.height / 3 + 80, { steps: 8 });
  await page.mouse.up();
  await expect(follow).toHaveAttribute("aria-pressed", "false");
});

test("wide screens keep one live map from a line to a stop to the map page", async ({ page }) => {
  test.skip(page.viewportSize()!.width < 1024, "phones show a small line map instead");
  await page.goto("/lines/tram/T4");
  await renderedPills(page, 1);
  await page.evaluate(() => ((globalThis as unknown as { __tramMap: { __mark?: number } }).__tramMap.__mark = 1));
  const sameMap = () => page.evaluate(() => (globalThis as unknown as { __tramMap?: { __mark?: number } }).__tramMap?.__mark === 1);
  await page.locator("main a[href^='/stops/']").nth(2).click();
  await expect(page).toHaveURL(/\/stops\//);
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  expect(await sameMap()).toBe(true);
  await page.getByRole("navigation").getByRole("link", { name: "Map" }).click();
  await expect(page.getByText(/\d{3} vehicles/)).toBeVisible();
  expect(await sameMap()).toBe(true);
  // The full-page map takes clicks again (the page above it lets them through). It
  // kept the stop page's close-up camera: zoom out to find vehicles of every line.
  await page.evaluate(() => (globalThis as unknown as { __tramMap: { jumpTo: (o: object) => void } }).__tramMap.jumpTo({ center: [24.75, 59.435], zoom: 13 }));
  const pills = await renderedPills(page);
  const vp = page.viewportSize()!;
  const target = pills.find((p) => p.y > 200 && p.y < vp.height - 250 && p.x > 120 && p.x < vp.width - 60) ?? pills[0]!;
  await page.mouse.click(target.x, target.y);
  await expect(page.locator("section[aria-label]").filter({ hasText: "towards" })).toBeVisible();
});

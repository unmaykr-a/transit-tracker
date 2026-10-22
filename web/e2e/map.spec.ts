import { expect, test } from "@playwright/test";

import { renderedPills } from "./helpers";

test.describe("live map", () => {
  test("shows vehicles from the feed and opens a vehicle sheet", async ({ page }) => {
    await page.goto("/map");
    await expect(page.getByText(/\d{3} vehicles/)).toBeVisible();
    const pills = await renderedPills(page);
    // Pick a pill away from the overlays at the top and bottom of the screen.
    const vp = page.viewportSize()!;
    const target = pills.find((p) => p.y > 200 && p.y < vp.height - 250 && p.x > 120 && p.x < vp.width - 60) ?? pills[0]!;
    await page.mouse.click(target.x, target.y);
    const sheet = page.locator("section[aria-label]").filter({ hasText: "towards" });
    await expect(sheet).toBeVisible();
    await expect(sheet).toContainText("Updated");
    await sheet.getByRole("button", { name: "Close" }).click();
    await expect(sheet).toBeHidden();
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
  await expect(sheet.getByText("Next stops")).toBeVisible();
  await expect(sheet.getByRole("listitem").first()).toContainText(/(now|\d+ min)$/);
  await expect(sheet.getByRole("link", { name: "Open line" })).toBeVisible();
  // Its route is drawn with the stops along it.
  await page.waitForFunction(() => {
    const m = (globalThis as { __tramMap?: { querySourceFeatures: (s: string) => unknown[] } }).__tramMap;
    return !!m && m.querySourceFeatures("selected-route").length > 0 && m.querySourceFeatures("selected-route-stops").length > 1;
  });
});

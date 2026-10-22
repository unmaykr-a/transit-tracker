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
    // The fake feed publishes a new snapshot every 10 s and markers glide toward
    // each one over about the same time, so most one-second windows show motion.
    await page.waitForTimeout(11_000);
    let prev = await position();
    let movingWindows = 0;
    for (let i = 0; i < 10; i++) {
      await page.waitForTimeout(1_000);
      const cur = await position();
      if (Object.keys(cur).filter((id) => prev[id] && prev[id] !== cur[id]).length > 30) movingWindows++;
      prev = cur;
    }
    expect(movingWindows).toBeGreaterThanOrEqual(6);
  });
});

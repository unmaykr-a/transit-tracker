import type { Page } from "@playwright/test";

export interface RenderedVehicle {
  id: string;
  mode: string;
  text: string;
  x: number;
  y: number;
}

/** Waits until the map has drawn at least min vehicle pills and returns them with screen positions. */
export async function renderedPills(page: Page, min = 5): Promise<RenderedVehicle[]> {
  await page.waitForFunction(
    (n) => {
      const m = (globalThis as { __tramMap?: { queryRenderedFeatures: (o: object) => unknown[] } }).__tramMap;
      return !!m && m.queryRenderedFeatures({ layers: ["vehicle-pills"] }).length >= n;
    },
    min,
    { timeout: 30_000 },
  );
  return page.evaluate(() => {
    type F = { properties: { id: string; mode: string; text: string }; geometry: { coordinates: [number, number] } };
    const m = (globalThis as unknown as {
      __tramMap: {
        queryRenderedFeatures: (o: object) => F[];
        project: (c: [number, number]) => { x: number; y: number };
        getCanvas: () => HTMLCanvasElement;
      };
    }).__tramMap;
    // project() is relative to the map canvas; on desktop the nav rail shifts it right.
    const box = m.getCanvas().getBoundingClientRect();
    return m.queryRenderedFeatures({ layers: ["vehicle-pills"] }).map((f) => {
      const p = m.project(f.geometry.coordinates);
      return { id: f.properties.id, mode: f.properties.mode, text: f.properties.text, x: box.left + p.x, y: box.top + p.y };
    });
  });
}

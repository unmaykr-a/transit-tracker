import * as maplibregl from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
// MapLibre finds its worker next to its own module, which bundling breaks; hand it Vite's build.
import workerUrl from "maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url";

import type { Theme } from "@/lib/theme";

maplibregl.setWorkerUrl(workerUrl);

export const STYLE_URL: Record<Theme, string> = {
  light: "https://tiles.openfreemap.org/styles/positron",
  dark: "https://tiles.openfreemap.org/styles/dark",
};

export const TALLINN: [number, number] = [24.7536, 59.437];

export { maplibregl };

/** MapLibre opens the compact attribution expanded on wide maps; start collapsed (it stays one tap away). */
export function collapseAttribution(map: maplibregl.Map) {
  map.once("load", () => {
    map.getContainer().querySelector(".maplibregl-ctrl-attrib")?.classList.remove("maplibregl-compact-show");
  });
}

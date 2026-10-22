// Kept apart from map/setup so the session bootstrap can configure maps without loading MapLibre.
import type { Theme } from "./theme";

const STYLE_URL: Record<Theme, string> = {
  light: "https://tiles.openfreemap.org/styles/positron",
  dark: "https://tiles.openfreemap.org/styles/dark",
};

let view = { center: [24.7536, 59.437] as [number, number], zoom: 13 };

/** Applies the admin's map settings (basemap styles, start view) to maps created from now on. */
export function configureMap(s: { map_style_light?: string; map_style_dark?: string; map_center?: [number, number]; map_zoom?: number }) {
  if (s.map_style_light) STYLE_URL.light = s.map_style_light;
  if (s.map_style_dark) STYLE_URL.dark = s.map_style_dark;
  if (s.map_center?.length === 2 && s.map_zoom) view = { center: [s.map_center[0], s.map_center[1]], zoom: s.map_zoom };
}

export const styleUrl = (theme: Theme) => STYLE_URL[theme];

/** Where a map without anything to fit starts: the configured centre and zoom. */
export const startView = () => view;

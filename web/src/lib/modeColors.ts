import type { Kind, Mode } from "./types";

/** One accent per mode, close to Tallinn's own colours. Mirrored as CSS tokens in index.css. */
export const MODE_COLORS: Record<Mode, string> = {
  tram: "#d02f2a",
  bus: "#0b7d75",
  trolley: "#2d66c3",
  train: "#b8540a",
};

/** Tailwind background classes per mode (tokens defined in index.css). */
export const MODE_BG: Record<Mode, string> = {
  tram: "bg-tram",
  bus: "bg-bus",
  trolley: "bg-trolley",
  train: "bg-train",
};

/** Map colours per kind: the modes plus county buses (purple, ≥ 4.5:1 with white). */
export const KIND_COLORS: Record<Kind, string> = { ...MODE_COLORS, county: "#6b3fb8" };

export const KIND_BG: Record<Kind, string> = { ...MODE_BG, county: "bg-county" };

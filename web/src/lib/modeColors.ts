import type { Mode } from "./types";

/** One accent per mode, close to Tallinn's own colours. Mirrored as CSS tokens in index.css. */
export const MODE_COLORS: Record<Mode, string> = {
  tram: "#d02f2a",
  bus: "#0e8f86",
  trolley: "#2d66c3",
  train: "#e8751a",
};

/** Tailwind background classes per mode (tokens defined in index.css). */
export const MODE_BG: Record<Mode, string> = {
  tram: "bg-tram",
  bus: "bg-bus",
  trolley: "bg-trolley",
  train: "bg-train",
};

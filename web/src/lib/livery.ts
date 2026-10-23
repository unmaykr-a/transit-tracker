import type { Vehicle } from "./types";

/**
 * The paint scheme a vehicle wears, after the fleets running in 2026 (photos of
 * Pesa Twist/CAF Urbos trams, Solaris CNG buses and Škoda 32Tr/33Tr trolleybuses
 * of TLT, GoBus's Scanias in Tartu, Elron's Škoda and FLIRT units, and the
 * operators' white county coaches):
 *  - TLT: one design for all modes — white body, black window band, slanted
 *    blocks along the skirt and a dashed line under the roof in the mode's
 *    colour (trams red, buses turquoise, trolleybuses blue).
 *  - Tartu: red lower body with the city skyline drawn in white, white roof.
 *  - Elron: orange nose and skirt, white sides with black swooshes and a cyan line.
 *  - County: plain white coaches (no common livery; the stripe is the app's county colour).
 */
export type Livery = "tlt-tram" | "tlt-bus" | "tlt-trolley" | "tartu" | "elron" | "county";

export const LIVERIES: readonly Livery[] = ["tlt-tram", "tlt-bus", "tlt-trolley", "tartu", "elron", "county"];

export interface LiveryColors {
  body: string;
  /** Skirt blocks, roof dashes, nose. */
  accent: string;
  /** Window band. */
  band: string;
  /** A second accent (Elron's cyan line). */
  line?: string;
}

export const LIVERY_COLORS: Record<Livery, LiveryColors> = {
  "tlt-tram": { body: "#f8fafc", accent: "#e3411e", band: "#16181d" },
  "tlt-bus": { body: "#f8fafc", accent: "#14ada2", band: "#16181d" },
  "tlt-trolley": { body: "#f8fafc", accent: "#1f6fd1", band: "#16181d" },
  tartu: { body: "#f8fafc", accent: "#d71f2b", band: "#16181d" },
  elron: { body: "#f8fafc", accent: "#f26a1b", band: "#16181d", line: "#1aa7df" },
  county: { body: "#f8fafc", accent: "#6b3fb8", band: "#334155" },
};

export function liveryOf(v: Pick<Vehicle, "id" | "mode" | "line" | "county">): Livery {
  if (v.county) return "county";
  if (v.mode === "train") return "elron";
  if (v.mode === "bus" && (v.line.startsWith("tartu-") || v.id.startsWith("bus-tartu-"))) return "tartu";
  return `tlt-${v.mode}`;
}

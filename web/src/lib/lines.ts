import type { Mode, Vehicle } from "./types";

/** Splits a free-text line filter ("4, 17a") into normalised tokens. */
export function parseLineQuery(q: string): string[] {
  return q
    .split(/[,\s]+/)
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean);
}

/** Tram lines are published as "T4"; a user typing "4" means tram 4 as well as bus 4. */
export function matchesLine(v: Pick<Vehicle, "line" | "label" | "mode">, tokens: string[]): boolean {
  if (tokens.length === 0) return true;
  const line = v.line.toLowerCase();
  const label = v.label.toLowerCase();
  return tokens.some((t) => t === line || t === label || (v.mode === "tram" && "t" + t === line));
}

export function matchesFilter(v: Vehicle, modes: ReadonlySet<Mode>, tokens: string[]): boolean {
  return modes.has(v.mode) && matchesLine(v, tokens);
}

/** Display text for a line pill: trams drop nothing, Elron shows the trip number. */
export function pillText(v: Pick<Vehicle, "label" | "line">): string {
  return v.label || v.line;
}

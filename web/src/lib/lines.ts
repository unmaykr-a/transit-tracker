import { kindOf, type Kind, type Vehicle } from "./types";

/** Splits a free-text line filter ("4, 17a") into normalised tokens. */
export function parseLineQuery(q: string): string[] {
  return q
    .split(/[,\s]+/)
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean);
}

/**
 * Tram lines are published as "T4"; a user typing "4" means tram 4 as well as
 * bus 4 (in Tallinn and Tartu: Tartu's are keyed "tartu-4", labelled "4").
 * A token "=<line>" matches that line only (a line page's own vehicles).
 */
export function matchesLine(v: Pick<Vehicle, "line" | "label" | "mode">, tokens: string[]): boolean {
  if (tokens.length === 0) return true;
  const line = v.line.toLowerCase();
  const label = v.label.toLowerCase();
  return tokens.some((t) => (t.startsWith("=") ? t.slice(1) === line : t === line || t === label || (v.mode === "tram" && "t" + t === line)));
}

export function matchesFilter(v: Vehicle, kinds: ReadonlySet<Kind>, tokens: string[]): boolean {
  return kinds.has(kindOf(v)) && matchesLine(v, tokens);
}

/** A line as shown: Tartu's "tartu-2" is "2" (its key stays in links and ids). */
export function displayLine(line: string): string {
  return line.startsWith("tartu-") ? line.slice(6) : line;
}

/** The city of a line other than Tallinn's, for a small tag ("Tartu"), else "". */
export function lineCity(line: string): string {
  return line.startsWith("tartu-") ? "Tartu" : "";
}

/** Display text for a line pill: trams drop nothing, Elron shows the trip number. */
export function pillText(v: Pick<Vehicle, "label" | "line">): string {
  return v.label || v.line;
}

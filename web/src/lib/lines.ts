import { kindOf, type Kind, type Mode, type Vehicle } from "./types";

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

/**
 * Lines of other regions are keyed with a prefix, because numbers repeat
 * (Tartu's "tartu-2", Rapla's "raplamaa-5", long-distance "intercity-164"); the
 * region names a small tag. Mirrors staticdata.LinePrefixes.
 */
export type LineRegion = "tartu" | "raplamaa" | "laane-virumaa" | "laanemaa" | "intercity";
const PREFIXES: readonly LineRegion[] = ["tartu", "raplamaa", "laane-virumaa", "laanemaa", "intercity"];

/** The region of a prefixed line, or undefined for Tallinn's and Harju county's. */
export function lineRegion(line: string): LineRegion | undefined {
  return PREFIXES.find((p) => line.startsWith(p + "-"));
}

/** A line as shown: "tartu-2" is "2", "raplamaa-5" is "5" (the key stays in links and ids). */
export function displayLine(line: string): string {
  const r = lineRegion(line);
  return r ? line.slice(r.length + 1) : line;
}

/** Display text for a line pill: trams drop nothing, Elron shows the trip number. */
export function pillText(v: Pick<Vehicle, "label" | "line">): string {
  return v.label || v.line;
}

/** County and long-distance lines share the county colour (they are the regional buses). */
export function routeKind(r: { mode: Mode; source?: string }): Kind {
  return r.source === "county" || r.source === "intercity" ? "county" : r.mode;
}

type Translate = (key: string, opts?: Record<string, string>) => string;

/** A line's short tag in lists: "Tartu", "Rapla", "Long-distance", "County", or its mode. */
export function lineTag(r: { mode: Mode; source?: string; short_name: string }, t: Translate): string {
  const region = lineRegion(r.short_name);
  if (r.source === "tartu") return t("search.tartu");
  if (r.source === "intercity") return t("search.intercity");
  if (r.source === "county") return region ? t(`region.${region}`) : t("modeShort.county");
  return t(`modeShort.${r.mode}`);
}

/** A line page's subtitle: "Rapla county bus", "Long-distance bus", "Tartu city bus", or its mode. */
export function lineKindLabel(r: { mode: Mode; source?: string; short_name: string }, t: Translate): string {
  if (r.source === "tartu") return t("line.tartu");
  if (r.source === "intercity") return t("line.intercity");
  if (r.source === "county") return t("line.county", { region: t(`region.${lineRegion(r.short_name) ?? "harjumaa"}`) });
  return t(`mode.${r.mode}`);
}

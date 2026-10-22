/** Pure helpers shared by the card and its tests: no DOM, no Lit. */

export type Mode = "tram" | "bus" | "trolley" | "train";
export type Level = "neutral" | "green" | "amber" | "red" | "none";

/** Same accents as the web app (web/src/lib/modeColors.ts). */
export const MODE_COLORS: Record<Mode, string> = {
  tram: "#d02f2a",
  bus: "#0b7d75",
  trolley: "#2d66c3",
  train: "#b8540a",
};

export interface Departure {
  time: string;
  minutes: number;
  delay_s: number;
  realtime: boolean;
  headsign: string;
  verdict: string;
  leave_in_min: number | null;
  /** Set for trips, where departures can be of different lines. */
  line?: string;
  mode?: Mode;
}

/** Attributes of a sensor.bus_*_next entity. */
export interface NextAttributes {
  kind?: "line" | "trip";
  /** A trip's connecting lines. */
  lines?: string[];
  /** Position in the user's saved-line order. */
  order?: number;
  /** The line's own starting place, when walks start there. */
  walk_from?: string;
  /** A trip's destination stop (headsign is the next departure's). */
  destination?: string;
  line?: string;
  mode?: Mode;
  label?: string;
  headsign?: string;
  stop_name?: string;
  platform?: string;
  minutes?: number | null;
  next_departures?: Departure[];
  walk_min?: number | null;
  walk_approx?: boolean;
  verdict?: Level;
  leave_in_min?: number | null;
  catchable?: boolean | null;
  url?: string;
  friendly_name?: string;
}

/** Whole minutes until t, floored, like the web app's countdown. */
export function minutesUntil(t: string | Date, now: Date): number {
  const ms = new Date(t).getTime() - now.getTime();
  return Math.floor(ms / 60000);
}

/** The verdict table from PLAN.md, mirrored from dashboard.Judge / web lib/verdict.ts. */
export function level(slack: number): Exclude<Level, "none"> {
  if (slack >= 5) return "neutral";
  if (slack >= 1) return "green";
  if (slack === 0) return "amber";
  return "red";
}

/**
 * The slack (minutes to spare before leaving) for a departure, recomputed for
 * now: the server's leave_in_min was right when it computed `minutes`, and
 * both count down together between polls.
 */
export function slackNow(d: Departure, now: Date): number | null {
  if (d.leave_in_min === null || d.leave_in_min === undefined) return null;
  return d.leave_in_min - (d.minutes - minutesUntil(d.time, now));
}

export interface View {
  line: string;
  mode: Mode;
  label: string;
  headsign: string;
  /** "→ Kopli" for a line, "Viru → Kopli" for a trip. */
  title: string;
  trip: boolean;
  /** A trip's lines (up to four), coloured by a departure's mode when one is listed. */
  pills: { line: string; mode: Mode }[];
  walkFrom: string;
  stop: string;
  walkMin: number | null;
  walkApprox: boolean;
  /** Upcoming departures that have not left yet, soonest first. */
  departures: { time: string; minutes: number; delayS: number; realtime: boolean; slack: number | null; level: Level; line?: string; mode?: Mode }[];
  url: string;
}

/** What the card shows for one entity at time now. */
export function view(attrs: NextAttributes, now: Date, count: number): View {
  const deps = (attrs.next_departures ?? [])
    .map((d) => {
      const slack = slackNow(d, now);
      return {
        time: d.time,
        minutes: minutesUntil(d.time, now),
        delayS: d.delay_s,
        realtime: d.realtime,
        slack,
        level: slack === null ? ("none" as const) : level(slack),
        line: d.line,
        mode: d.mode,
      };
    })
    .filter((d) => d.minutes >= 0);
  // Skip departures that can no longer be caught, unless none can.
  const firstOk = deps.findIndex((d) => d.level !== "red");
  const from = firstOk > 0 ? firstOk : 0;
  const stop = [attrs.stop_name, attrs.platform].filter(Boolean).join(" · ");
  const shown = deps.slice(from, from + count);
  // A trip's departures carry their own line: the pill follows the first one shown.
  const head = shown[0];
  const trip = attrs.kind === "trip";
  const mode = head?.mode ?? attrs.mode ?? "bus";
  const modeOf = (line: string) => (attrs.next_departures ?? []).find((d) => d.line === line)?.mode ?? mode;
  return {
    line: head?.line ?? attrs.line ?? "?",
    mode,
    label: attrs.label ?? attrs.line ?? "",
    headsign: attrs.headsign ?? "",
    title: trip ? `${attrs.stop_name ?? "?"} → ${attrs.destination ?? attrs.headsign ?? "?"}` : `→ ${attrs.headsign ?? ""}`,
    trip,
    pills: trip ? (attrs.lines ?? []).slice(0, 4).map((line) => ({ line, mode: modeOf(line) })) : [],
    walkFrom: attrs.walk_from ?? "",
    stop,
    walkMin: attrs.walk_min ?? null,
    walkApprox: Boolean(attrs.walk_approx),
    departures: shown,
    url: attrs.url ?? "",
  };
}

/** 24-hour clock time in Tallinn, as the web app shows it. */
export function clock(t: string, lang: string | undefined): string {
  return new Intl.DateTimeFormat(lang, { hour: "2-digit", minute: "2-digit", hourCycle: "h23", timeZone: "Europe/Tallinn" }).format(new Date(t));
}

/** Every Bus "next" sensor, in the user's saved-line order. */
export function allBusEntities(states: Record<string, { entity_id: string; attributes: Record<string, unknown> }>): string[] {
  return Object.values(states)
    .filter((e) => e.entity_id.startsWith("sensor.") && e.entity_id.endsWith("_next") && Array.isArray(e.attributes.next_departures))
    .sort((a, b) => Number(a.attributes.order ?? 99) - Number(b.attributes.order ?? 99) || a.entity_id.localeCompare(b.entity_id))
    .map((e) => e.entity_id);
}

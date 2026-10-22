/** Pure helpers shared by the card and its tests: no DOM, no Lit. */

export type Mode = "tram" | "bus" | "trolley" | "train";
export type Level = "neutral" | "green" | "amber" | "red" | "none";

/** Same accents as the web app (web/src/lib/modeColors.ts). */
export const MODE_COLORS: Record<Mode, string> = {
  tram: "#d02f2a",
  bus: "#0e8f86",
  trolley: "#2d66c3",
  train: "#e8751a",
};

export interface Departure {
  time: string;
  minutes: number;
  delay_s: number;
  realtime: boolean;
  headsign: string;
  verdict: string;
  leave_in_min: number | null;
}

/** Attributes of a sensor.tram_*_next entity. */
export interface NextAttributes {
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
  stop: string;
  walkMin: number | null;
  walkApprox: boolean;
  /** Upcoming departures that have not left yet, soonest first. */
  departures: { minutes: number; delayS: number; realtime: boolean; slack: number | null; level: Level }[];
  url: string;
}

/** What the card shows for one entity at time now. */
export function view(attrs: NextAttributes, now: Date, count: number): View {
  const deps = (attrs.next_departures ?? [])
    .map((d) => {
      const slack = slackNow(d, now);
      return {
        minutes: minutesUntil(d.time, now),
        delayS: d.delay_s,
        realtime: d.realtime,
        slack,
        level: slack === null ? ("none" as const) : level(slack),
      };
    })
    .filter((d) => d.minutes >= 0);
  // Skip departures that can no longer be caught, unless none can.
  const firstOk = deps.findIndex((d) => d.level !== "red");
  const from = firstOk > 0 ? firstOk : 0;
  const stop = [attrs.stop_name, attrs.platform].filter(Boolean).join(" · ");
  return {
    line: attrs.line ?? "?",
    mode: attrs.mode ?? "bus",
    label: attrs.label ?? attrs.line ?? "",
    headsign: attrs.headsign ?? "",
    stop,
    walkMin: attrs.walk_min ?? null,
    walkApprox: Boolean(attrs.walk_approx),
    departures: deps.slice(from, from + count),
    url: attrs.url ?? "",
  };
}

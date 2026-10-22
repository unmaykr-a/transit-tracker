import type { TFunction } from "i18next";

/** Minutes until t, floored, never negative. */
export function minutesUntil(t: string | number, now: number): number {
  const ms = (typeof t === "string" ? Date.parse(t) : t) - now;
  return Math.max(0, Math.floor(ms / 60_000));
}

/** "now", "4 min", or the clock time when more than an hour away. */
export function countdown(t: string, now: number, tr: TFunction, locale?: string): string {
  const ms = Date.parse(t) - now;
  if (ms < 45_000) return tr("time.now");
  const min = Math.round(ms / 60_000);
  if (min < 60) return tr("time.minutes", { count: min });
  return clock(t, locale);
}

export function clock(t: string, locale?: string): string {
  // Timetables in Estonia are 24-hour in every language.
  return new Date(t).toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit", hourCycle: "h23", timeZone: "Europe/Tallinn" });
}

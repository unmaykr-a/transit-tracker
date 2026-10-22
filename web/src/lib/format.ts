import type { TFunction } from "i18next";

/** Minutes until t, floored, never negative. */
export function minutesUntil(t: string | number, now: number): number {
  const ms = (typeof t === "string" ? Date.parse(t) : t) - now;
  return Math.max(0, Math.floor(ms / 60_000));
}

/**
 * "now", "4 min", or the clock time when more than an hour away. Minutes are
 * floored, like transit boards and the verdict's slack, so the two agree.
 */
export function countdown(t: string, now: number, tr: TFunction, locale?: string): string {
  const ms = Date.parse(t) - now;
  if (ms < 60_000) return tr("time.now");
  const min = Math.floor(ms / 60_000);
  if (min < 60) return tr("time.minutes", { count: min });
  return clock(t, locale);
}

export function clock(t: string, locale?: string): string {
  // Timetables in Estonia are 24-hour in every language.
  return new Date(t).toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit", hourCycle: "h23", timeZone: "Europe/Tallinn" });
}

/** "3 minutes ago", "in 2 days" in the given language. */
/** "5 min ago"; `now` comes from useNow() so the label stays pure and ticks. */
export function relative(t: string, lang: string | undefined, now: number): string {
  const s = Math.round((Date.parse(t) - now) / 1000);
  const rtf = new Intl.RelativeTimeFormat(lang, { numeric: "auto" });
  if (Math.abs(s) < 60) return rtf.format(s, "second");
  if (Math.abs(s) < 3600) return rtf.format(Math.round(s / 60), "minute");
  if (Math.abs(s) < 86400) return rtf.format(Math.round(s / 3600), "hour");
  return rtf.format(Math.round(s / 86400), "day");
}

export function bytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 ** 2) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 ** 2).toFixed(1)} MB`;
}

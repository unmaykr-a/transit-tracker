import type { VerdictLevel } from "@/api";

/** Whole minutes until t, floored like a countdown; negative once departed. Mirrors dashboard.MinutesUntil. */
export function minutesUntilSigned(t: string, now: number): number {
  const ms = Date.parse(t) - now;
  return ms >= 0 ? Math.floor(ms / 60_000) : -Math.ceil(-ms / 60_000);
}

/** slack = minutes until departure − walk − buffer (PLAN.md). Mirrors dashboard.Judge. */
export function judge(expectedAt: string, now: number, walkMin: number, bufferMin: number): { level: VerdictLevel; slack: number } {
  const slack = minutesUntilSigned(expectedAt, now) - walkMin - bufferMin;
  if (slack >= 5) return { level: "neutral", slack };
  if (slack >= 1) return { level: "green", slack };
  if (slack === 0) return { level: "amber", slack };
  return { level: "red", slack };
}

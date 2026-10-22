import { useTranslation } from "react-i18next";

import { clock } from "@/lib/format";

/**
 * One-series sparkline of a feed's item count per minute. No legend (the
 * caption names it); a neutral ink line, since mode and status colours carry
 * other meanings; per-minute hover targets wider than the line.
 */
export function Sparkline({ points, label }: { points: { t: number; count: number }[]; label: string }) {
  const { i18n } = useTranslation();
  const w = 240;
  const h = 40;
  if (points.length < 2) return null;
  const max = Math.max(...points.map((p) => p.count), 1);
  const t0 = points[0]!.t;
  const span = Math.max(points[points.length - 1]!.t - t0, 60);
  const x = (t: number) => ((t - t0) / span) * (w - 4) + 2;
  const y = (c: number) => h - 3 - (c / max) * (h - 6);
  const d = points.map((p, i) => `${i ? "L" : "M"}${x(p.t).toFixed(1)},${y(p.count).toFixed(1)}`).join(" ");
  const min = Math.min(...points.map((p) => p.count));
  const step = (w - 4) / Math.max(points.length - 1, 1);
  return (
    <figure className="flex flex-col gap-1">
      <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} role="img" aria-label={`${label}: ${min}–${max}`} className="text-foreground">
        <line x1={0} x2={w} y1={h - 3} y2={h - 3} className="stroke-border" strokeWidth={1} />
        <path d={d} fill="none" stroke="currentColor" strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />
        {points.map((p) => (
          <rect key={p.t} x={x(p.t) - step / 2} y={0} width={step} height={h} fill="transparent">
            <title>{`${clock(new Date(p.t * 1000).toISOString(), i18n.resolvedLanguage)} — ${p.count}`}</title>
          </rect>
        ))}
      </svg>
      <figcaption className="text-xs text-muted-foreground">{label}</figcaption>
    </figure>
  );
}

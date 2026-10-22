import { Link } from "@tanstack/react-router";
import { ArrowDown, ArrowUp, Footprints, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";

import type { DashboardEntry } from "@/api";
import { Button } from "@/components/ui/button";
import { clock, countdown, minutesUntil } from "@/lib/format";
import { judge } from "@/lib/verdict";
import { cn } from "@/lib/utils";

import { LinePill } from "./LinePill";
import { VerdictChip } from "./VerdictChip";

interface Props {
  entry: DashboardEntry;
  now: number;
  bufferMin: number;
  editing?: boolean;
  onMove?: (delta: -1 | 1) => void;
  onRemove?: () => void;
  first?: boolean;
  last?: boolean;
}

export function LineCard({ entry, now, bufferMin, editing, onMove, onRemove, first, last }: Props) {
  const { t, i18n } = useTranslation();
  const lang = i18n.resolvedLanguage;
  const l = entry.line;
  // Verdicts are recomputed every tick from the departure times, so chips stay
  // right between dashboard refreshes.
  const deps = entry.departures
    .filter((d) => Date.parse(d.expected_at) > now - 30_000)
    .map((d) => ({ ...d, v: entry.walk ? judge(d.expected_at, now, entry.walk.minutes, bufferMin) : { level: "none" as const, slack: 0 } }));
  const head = deps[0];
  const catchIdx = deps.findIndex((d) => d.v.level !== "red");
  const nextCatchable = catchIdx >= 0 ? deps[catchIdx] : undefined;
  const title = `${l.short_name} → ${l.headsign}`;

  return (
    <article aria-label={l.label || title} className="flex flex-col gap-3 rounded-2xl border bg-card p-4 shadow-sm">
      <header className="flex items-start gap-3">
        {l.mode && <LinePill mode={l.mode} line={l.short_name} link className="h-8 min-w-11 text-base" />}
        <div className="min-w-0 flex-1">
          <p className="truncate font-semibold">
            {l.label ? <>{l.label} · </> : null}→ {l.headsign}
          </p>
          <p className="flex min-w-0 items-center gap-1 text-sm text-muted-foreground">
            {l.board_stop && (
              <Link to="/stops/$id" params={{ id: l.board_stop.id }} className="min-w-0 truncate hover:underline">
                {t("home.from", { stop: `${l.board_stop.name}${l.board_stop.platform ? " " + l.board_stop.platform : ""}` })}
              </Link>
            )}
            {entry.walk && (
              <span className="inline-flex shrink-0 items-center gap-1 whitespace-nowrap">
                · <Footprints className="size-3.5" aria-hidden />
                {entry.walk.approx ? t("home.walkApprox", { min: entry.walk.minutes }) : t("home.walk", { min: entry.walk.minutes })}
              </span>
            )}
          </p>
        </div>
        {editing && (
          <div className="flex gap-1">
            <Button variant="ghost" size="icon" disabled={first} onClick={() => onMove?.(-1)} aria-label={t("home.moveUp")}>
              <ArrowUp className="size-4" />
            </Button>
            <Button variant="ghost" size="icon" disabled={last} onClick={() => onMove?.(1)} aria-label={t("home.moveDown")}>
              <ArrowDown className="size-4" />
            </Button>
            <Button variant="ghost" size="icon" onClick={onRemove} aria-label={t("home.remove")}>
              <Trash2 className="size-4" />
            </Button>
          </div>
        )}
      </header>

      {l.unavailable ? (
        <p className="text-sm text-muted-foreground">{t("home.unavailable")}</p>
      ) : !head ? (
        <p className="text-sm text-muted-foreground">{entry.error ? t("stop.error") : t("home.noDepartures")}</p>
      ) : (
        <div className="flex flex-wrap items-end justify-between gap-x-4 gap-y-2">
          <div className="flex items-baseline gap-3" aria-live="polite">
            <span className="text-4xl font-semibold tracking-tight tabular-nums">{countdown(head.expected_at, now, t, lang)}</span>
            <span className="text-sm text-muted-foreground tabular-nums">
              {clock(head.expected_at, lang)}
              {!head.realtime && <> · {t("stop.scheduled")}</>}
            </span>
          </div>
          <VerdictChip level={head.v.level} slack={head.v.slack} nextIn={nextCatchable ? minutesUntil(nextCatchable.expected_at, now) : undefined} />
          {deps.length > 1 && (
            <p className="w-full text-sm text-muted-foreground">
              {t("home.then")}{" "}
              {deps.slice(1, 3).map((d, i) => (
                <span
                  key={d.scheduled_at + i}
                  className={cn("mr-2 tabular-nums", head.v.level === "red" && d === nextCatchable && "rounded bg-muted px-1.5 py-0.5 font-medium text-foreground")}
                >
                  {countdown(d.expected_at, now, t, lang)}
                  {!d.realtime && "*"}
                </span>
              ))}
            </p>
          )}
        </div>
      )}
    </article>
  );
}

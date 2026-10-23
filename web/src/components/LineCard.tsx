import { Link } from "@tanstack/react-router";
import { ArrowDown, ArrowUp, CornerDownRight, Flag, Footprints, Pencil, Trash2 } from "lucide-react";
import { useState } from "react";
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
  const [confirming, setConfirming] = useState(false);
  // Verdicts are recomputed every tick from the departure times, so chips stay
  // right between dashboard refreshes.
  // A trip's departures leave from several platforms, each with its own walk.
  const walkFor = (stopId: string) => entry.walks?.[stopId] ?? entry.walk;
  const deps = entry.departures
    .filter((d) => Date.parse(d.expected_at) > now - 30_000)
    .map((d) => {
      const w = walkFor(d.stop_id);
      return { ...d, v: w ? judge(d.expected_at, now, w.minutes, bufferMin) : { level: "none" as const, slack: 0 } };
    });
  const head = deps[0];
  const catchIdx = deps.findIndex((d) => d.v.level !== "red");
  const nextCatchable = catchIdx >= 0 ? deps[catchIdx] : undefined;
  const trip = l.kind === "trip";
  const tripLines = trip ? [...new Map((l.lines ?? []).map((x) => [x.route_id, x])).values()] : [];
  const title = trip ? `${l.from?.name ?? "?"} → ${l.to?.name ?? "?"}` : `${l.short_name} → ${l.headsign}`;
  const walk = head ? walkFor(head.stop_id) : entry.walk;
  const via = (l.transfers ?? []).map((x) => x.alight?.name).filter(Boolean);
  // The journey worth spelling out: of the departure to aim for.
  const focus = head && head.v.level === "red" && nextCatchable ? nextCatchable : head;
  const journey = focus?.journey;

  return (
    <article aria-label={l.label || title} className="flex flex-col gap-3 rounded-2xl border bg-card p-4 shadow-sm">
      <header className="flex items-start gap-3">
        {trip ? (
          <div className="flex max-w-28 shrink-0 flex-wrap gap-1" aria-label={t("trip.linesLabel")}>
            {tripLines.slice(0, 4).map((x) => (
              <LinePill key={x.route_id} mode={x.mode} line={x.short_name} link className="h-7 min-w-9 text-sm" />
            ))}
            {tripLines.length > 4 && <span className="self-center text-xs text-muted-foreground">+{tripLines.length - 4}</span>}
          </div>
        ) : (
          l.mode && <LinePill mode={l.mode} line={l.short_name} link className="h-8 min-w-11 text-base" />
        )}
        <div className="min-w-0 flex-1">
          <p className="truncate font-semibold">
            {l.label ? <>{l.label} · </> : null}
            {trip ? title : <>→ {l.headsign}</>}
          </p>
          <p className="flex min-w-0 items-center gap-1 text-sm text-muted-foreground">
            {trip && l.from && (
              <Link to="/stops/$id" params={{ id: l.from.id }} className="min-w-0 truncate hover:underline">
                {t("home.from", { stop: `${l.from.name}${head?.platform ? " " + head.platform : ""}` })}
              </Link>
            )}
            {!trip && l.board_stop && (
              <Link to="/stops/$id" params={{ id: l.board_stop.id }} className="min-w-0 truncate hover:underline">
                {t("home.from", { stop: `${l.board_stop.name}${l.board_stop.platform ? " " + l.board_stop.platform : ""}` })}
              </Link>
            )}
            {walk && (
              <span className="inline-flex shrink-0 items-center gap-1 whitespace-nowrap">
                · <Footprints className="size-3.5" aria-hidden />
                {walk.approx ? t("home.walkApprox", { min: walk.minutes }) : t("home.walk", { min: walk.minutes })}
                {entry.walk_from && <> {t("home.walkFromPlace", { place: entry.walk_from })}</>}
              </span>
            )}
          </p>
          {via.length > 0 && <p className="truncate text-sm text-muted-foreground">{t("home.via", { stops: via.join(", ") })}</p>}
        </div>
        {editing && (
          <div className="flex gap-1">
            <Button variant="ghost" size="icon" disabled={first} onClick={() => onMove?.(-1)} aria-label={t("home.moveUp")}>
              <ArrowUp className="size-4" />
            </Button>
            <Button variant="ghost" size="icon" disabled={last} onClick={() => onMove?.(1)} aria-label={t("home.moveDown")}>
              <ArrowDown className="size-4" />
            </Button>
            {trip && (
              <Button variant="ghost" size="icon" asChild>
                <Link to="/trips/new" search={{ edit: l.id }} aria-label={t("trip.edit")}>
                  <Pencil className="size-4" />
                </Link>
              </Button>
            )}
            <Button variant="ghost" size="icon" onClick={() => setConfirming(true)} aria-label={t("home.remove")} aria-expanded={confirming}>
              <Trash2 className="size-4" />
            </Button>
          </div>
        )}
      </header>
      {editing && confirming && (
        // Its own row: inline in the header it squeezed the title away on phones.
        <div role="group" aria-label={t("home.removeLine")} className="flex flex-wrap items-center justify-between gap-2 rounded-lg bg-muted px-3 py-2 text-sm">
          <span>{t("home.removeLine")}</span>
          <span className="flex gap-2">
            <Button size="sm" variant="ghost" onClick={() => setConfirming(false)}>
              {t("admin.cancel")}
            </Button>
            <Button size="sm" className="bg-red-600 text-white hover:bg-red-700" onClick={() => onRemove?.()}>
              {t("home.remove")}
            </Button>
          </span>
        </div>
      )}

      {l.unavailable ? (
        <p className="text-sm text-muted-foreground">{t("home.unavailable")}</p>
      ) : !head ? (
        <p className="text-sm text-muted-foreground">{entry.error ? t("stop.error") : t("home.noDepartures")}</p>
      ) : (
        <div className="flex flex-wrap items-end justify-between gap-x-4 gap-y-2">
          <div className="flex items-baseline gap-3">
            {trip && <LinePill mode={head.mode} line={head.line} className="h-7 min-w-9 self-center text-sm" />}
            <span className="text-4xl font-semibold tracking-tight tabular-nums">{countdown(head.expected_at, now, t, lang)}</span>
            <span className="text-sm text-muted-foreground tabular-nums">
              {clock(head.expected_at, lang)}
              {!head.realtime && <> · {t("stop.scheduled")}</>}
            </span>
          </div>
          <VerdictChip level={head.v.level} slack={head.v.slack} nextIn={nextCatchable ? minutesUntil(nextCatchable.expected_at, now) : undefined} />
          {journey && (
            <ul className="w-full space-y-1 text-sm" aria-label={t("home.journey")}>
              {journey.legs.slice(1).map((leg, i) => (
                <li key={i} className="flex flex-wrap items-center gap-x-1.5 gap-y-0.5">
                  <CornerDownRight className="size-4 text-muted-foreground" aria-hidden />
                  <LinePill mode={leg.mode} line={leg.line} className="h-5 min-w-7 px-1.5 text-xs" />
                  <span className="tabular-nums">{clock(leg.depart_at, lang)}</span>
                  <span className="min-w-0 truncate text-muted-foreground">
                    {t("home.changeAt", { stop: leg.board_stop.name })}
                    {leg.walk_min > 0 && <> · {t("home.walkMin", { min: leg.walk_min })}</>}
                    {" · "}
                    <span className={cn(leg.wait_min === 0 && "font-medium text-amber-700 dark:text-amber-400")}>
                      {leg.wait_min === 0 ? t("home.tightChange") : t("home.waitMin", { min: leg.wait_min })}
                    </span>
                  </span>
                </li>
              ))}
              <li className="flex items-center gap-1.5 text-muted-foreground">
                <Flag className="size-4" aria-hidden />
                {journey.arrive_at ? (
                  <span>
                    {t("home.arrive")} <span className="font-medium text-foreground tabular-nums">≈ {clock(journey.arrive_at, lang)}</span>
                  </span>
                ) : (
                  <span>{t("home.laterUnknown")}</span>
                )}
              </li>
            </ul>
          )}
          {deps.length > 1 && (
            <p className="w-full text-sm text-muted-foreground">
              {t("home.then")}{" "}
              {deps.slice(1, 3).map((d, i) => (
                <span
                  key={d.scheduled_at + i}
                  className={cn("mr-2 tabular-nums", head.v.level === "red" && d === nextCatchable && "rounded bg-muted px-1.5 py-0.5 font-medium text-foreground")}
                >
                  {trip && <span className="font-medium text-foreground">{d.line} </span>}
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

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { BellRing, LocateFixed, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { fetchTrip } from "@/api";
import { Button } from "@/components/ui/button";
import { countdown } from "@/lib/format";
import { pillText } from "@/lib/lines";
import { buzz, loadRide, ridePhase, saveRide, type Ride, type RidePhase } from "@/lib/ride";
import { useLiveState, useNow } from "@/lib/useLive";
import { cn } from "@/lib/utils";

import { MapSheet } from "./MapSheet";
import { ModePill } from "./ModePill";

export function VehicleSheet({
  id,
  onClose,
  following,
  onFollow,
}: {
  id: string;
  onClose: () => void;
  /** Whether the map keeps this vehicle centred; onFollow toggles it (no button without it). */
  following?: boolean;
  onFollow?: (on: boolean) => void;
}) {
  const { t, i18n } = useTranslation();
  const state = useLiveState(1000);
  const now = useNow(1000);
  const v = state.vehicles.get(id);
  const ageS = v ? Math.max(0, Math.round((now + state.clockSkewMs - Date.parse(v.updated_at)) / 1000)) : 0;
  // On-board mode: pick the stop to get off at, then the sheet counts down to it and buzzes.
  const [ride, setRide] = useState<Ride | null>(() => {
    const r = loadRide();
    return r?.vehicle === id ? r : null;
  });
  const [choosing, setChoosing] = useState(false);
  // A fresh prediction with every new position fix (the previous one stays on screen meanwhile).
  const limit = ride || choosing ? 60 : 6;
  const trip = useQuery({
    queryKey: ["trip", id, v?.updated_at, limit],
    queryFn: ({ signal }) => fetchTrip(id, signal, limit),
    placeholderData: keepPreviousData,
    enabled: !!v?.pattern_id,
  });
  const phase: RidePhase | null = ride ? ridePhase(ride, trip.data?.trip?.pattern_id, trip.data?.trip?.ahead) : null;
  const startRide = (r: Ride) => {
    setRide(r);
    saveRide(r);
    setChoosing(false);
    onFollow?.(true);
  };
  const endRide = () => {
    setRide(null);
    saveRide(null);
  };
  // Alerts once per phase change: a stop before yours, and at the next stop.
  const lastPhase = useRef<string>(undefined);
  useEffect(() => {
    const kind = phase?.kind;
    if (kind === lastPhase.current) return;
    lastPhase.current = kind;
    if (kind === "prepare") buzz([200, 100, 200]);
    else if (kind === "next") buzz([400, 150, 400, 150, 400]);
  }, [phase?.kind]);

  const next = trip.data?.trip?.ahead[0];
  const rideText = (ph: RidePhase, r: Ride) =>
    ph.kind === "riding"
      ? t("ride.stopsLeft", { count: ph.stopsLeft, stop: r.name })
      : ph.kind === "prepare"
        ? t("ride.prepare", { stop: r.name })
        : ph.kind === "next"
          ? t("ride.next", { stop: r.name })
          : ph.kind === "arrived"
            ? t("ride.arrived", { stop: r.name })
            : t("ride.waiting", { stop: r.name });
  return (
    <MapSheet
      onClose={onClose}
      label={v ? `${t(`mode.${v.mode}`)} ${pillText(v)}` : undefined}
      summary={
        v && ride && phase ? (
          <span className="font-medium">{rideText(phase, ride)}</span>
        ) : v && next ? (
          <>
            <span className="text-muted-foreground">{t("vehicle.nextStop")} </span>
            {next.name}
            {next.platform && <span className="text-muted-foreground"> · {next.platform}</span>}
            <span className="font-medium tabular-nums"> · {countdown(next.eta, now, t, i18n.resolvedLanguage)}</span>
          </>
        ) : undefined
      }
      header={
        <div className="flex items-start gap-3">
          {v ? (
            <div className="flex min-w-0 flex-1 items-center gap-3">
              <ModePill mode={v.mode}>{pillText(v)}</ModePill>
              <div className="min-w-0">
                <p className="truncate font-semibold">{t("vehicle.towards", { destination: v.destination || "—" })}</p>
                <p className="text-sm text-muted-foreground">
                  {v.mode === "train" ? `${v.line} · ${t("vehicle.trip", { trip: v.label })}` : v.county ? t("vehicle.county") : t(`mode.${v.mode}`)}
                </p>
              </div>
            </div>
          ) : (
            <p className="flex-1 text-muted-foreground">{t("vehicle.gone")}</p>
          )}
          {v && onFollow && (
            <Button
              variant={following ? "default" : "ghost"}
              size="icon"
              onClick={() => onFollow(!following)}
              aria-pressed={!!following}
              aria-label={t("vehicle.follow")}
              title={t("vehicle.follow")}
            >
              <LocateFixed className="size-5" />
            </Button>
          )}
          <Button variant="ghost" size="icon" onClick={onClose} aria-label={t("vehicle.close")}>
            <X className="size-5" />
          </Button>
        </div>
      }
    >
      {v && ride && phase && (
        <div
          role="status"
          className={cn(
            "mt-4 flex items-center gap-3 rounded-xl border px-3 py-2 text-sm",
            phase.kind === "next" || phase.kind === "prepare" ? "border-amber-500 bg-amber-50 dark:bg-amber-950" : "bg-muted/50",
          )}
        >
          <BellRing className="size-5 shrink-0" aria-hidden />
          <div className="min-w-0 flex-1">
            <p className="font-medium">{rideText(phase, ride)}</p>
            {"eta" in phase && phase.eta && (
              <p className="text-muted-foreground tabular-nums">{t("ride.eta", { when: countdown(phase.eta, now, t, i18n.resolvedLanguage) })}</p>
            )}
          </div>
          <Button variant="ghost" size="sm" onClick={endRide}>
            {phase.kind === "arrived" ? t("ride.done") : t("ride.stop")}
          </Button>
        </div>
      )}
      {v && (
        <dl className="mt-4 grid grid-cols-3 gap-2 text-sm">
          <div>
            <dt className="text-muted-foreground">{t("vehicle.speed")}</dt>
            <dd className="font-medium tabular-nums">
              {t("vehicle.speedValue", {
                value: Math.round(v.speed_mps * 3.6),
              })}
            </dd>
          </div>
          {v.delay_s !== undefined && (
            <div>
              <dt className="text-muted-foreground">{t("vehicle.delay")}</dt>
              <dd className="font-medium tabular-nums">
                {Math.abs(v.delay_s) < 30
                  ? t("vehicle.onTime")
                  : v.delay_s > 0
                    ? t("vehicle.late", { min: Math.round(v.delay_s / 60) })
                    : t("vehicle.early", { min: Math.round(-v.delay_s / 60) })}
              </dd>
            </div>
          )}
          <div>
            <dt className="text-muted-foreground">{t("vehicle.updated")}</dt>
            <dd className="font-medium tabular-nums">{t("vehicle.secondsAgo", { count: ageS })}</dd>
          </div>
          {v.low_floor && <div className="col-span-3 text-muted-foreground">{t("vehicle.lowFloor")}</div>}
        </dl>
      )}
      {v && trip.data?.trip && trip.data.trip.ahead.length > 0 && (
        <div className="mt-4">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-sm font-medium">{choosing ? t("ride.choose") : t("vehicle.nextStops")}</h3>
            {!ride && (
              <Button variant={choosing ? "default" : "outline"} size="sm" onClick={() => setChoosing((c) => !c)} aria-pressed={choosing}>
                <BellRing className="size-4" aria-hidden />
                {t("ride.onBoard")}
              </Button>
            )}
          </div>
          {choosing && <p className="mt-1 text-xs text-muted-foreground">{t("ride.hint")}</p>}
          <ol className="mt-1 divide-y text-sm">
            {trip.data.trip.ahead.map((a) => (
              <li key={`${a.stop_id}-${a.index}`} className={cn("flex items-center justify-between gap-3 py-1.5", ride?.index === a.index && "font-semibold")}>
                {choosing ? (
                  <button
                    type="button"
                    className="min-w-0 flex-1 truncate rounded text-left hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    onClick={() => startRide({ vehicle: id, pattern: trip.data!.trip!.pattern_id, stop: a.stop_id, index: a.index, name: a.name })}
                    aria-label={t("ride.getOffAt", { stop: a.name })}
                  >
                    {a.name}
                    {a.platform && <span className="text-muted-foreground"> · {a.platform}</span>}
                  </button>
                ) : (
                  <Link to="/stops/$id" params={{ id: a.stop_id }} className="min-w-0 truncate hover:underline">
                    {a.name}
                    {a.platform && <span className="text-muted-foreground"> · {a.platform}</span>}
                  </Link>
                )}
                <span className="font-medium tabular-nums whitespace-nowrap">{countdown(a.eta, now, t, i18n.resolvedLanguage)}</span>
              </li>
            ))}
          </ol>
          <Link to="/lines/$mode/$line" params={{ mode: v.mode, line: v.line }} className="mt-2 inline-block text-sm text-muted-foreground underline">
            {t("vehicle.openLine")}
          </Link>
        </div>
      )}
    </MapSheet>
  );
}

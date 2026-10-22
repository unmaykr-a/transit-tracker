import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { X } from "lucide-react";
import { useTranslation } from "react-i18next";

import { fetchTrip } from "@/api";
import { Button } from "@/components/ui/button";
import { countdown } from "@/lib/format";
import { pillText } from "@/lib/lines";
import { useLiveState, useNow } from "@/lib/useLive";

import { MapSheet } from "./MapSheet";
import { ModePill } from "./ModePill";

export function VehicleSheet({ id, onClose }: { id: string; onClose: () => void }) {
  const { t, i18n } = useTranslation();
  const state = useLiveState(1000);
  const now = useNow(1000);
  const v = state.vehicles.get(id);
  const ageS = v ? Math.max(0, Math.round((now + state.clockSkewMs - Date.parse(v.updated_at)) / 1000)) : 0;
  const trip = useQuery({
    queryKey: ["trip", id],
    queryFn: ({ signal }) => fetchTrip(id, signal),
    refetchInterval: 10_000,
    enabled: !!v?.pattern_id,
  });

  const next = trip.data?.trip?.ahead[0];
  return (
    <MapSheet
      onClose={onClose}
      label={v ? `${t(`mode.${v.mode}`)} ${pillText(v)}` : undefined}
      summary={
        v && next ? (
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
                  {v.mode === "train" ? `${v.line} · ${t("vehicle.trip", { trip: v.label })}` : t(`mode.${v.mode}`)}
                </p>
              </div>
            </div>
          ) : (
            <p className="flex-1 text-muted-foreground">{t("vehicle.gone")}</p>
          )}
          <Button variant="ghost" size="icon" onClick={onClose} aria-label={t("vehicle.close")}>
            <X className="size-5" />
          </Button>
        </div>
      }
    >
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
          <h3 className="text-sm font-medium">{t("vehicle.nextStops")}</h3>
          <ol className="mt-1 divide-y text-sm">
            {trip.data.trip.ahead.map((a) => (
              <li key={`${a.stop_id}-${a.index}`} className="flex items-center justify-between gap-3 py-1.5">
                <Link to="/stops/$id" params={{ id: a.stop_id }} className="min-w-0 truncate hover:underline">
                  {a.name}
                  {a.platform && <span className="text-muted-foreground"> · {a.platform}</span>}
                </Link>
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

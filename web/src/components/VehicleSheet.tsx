import { X } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { pillText } from "@/lib/lines";
import { useLiveState, useNow } from "@/lib/useLive";

import { ModePill } from "./ModePill";

export function VehicleSheet({ id, onClose }: { id: string; onClose: () => void }) {
  const { t } = useTranslation();
  const state = useLiveState(1000);
  const now = useNow(1000);
  const v = state.vehicles.get(id);
  const ageS = v ? Math.max(0, Math.round((now + state.clockSkewMs - Date.parse(v.updated_at)) / 1000)) : 0;

  return (
    <section
      aria-label={v ? `${t(`mode.${v.mode}`)} ${pillText(v)}` : undefined}
      className="pointer-events-auto mx-auto w-full max-w-md rounded-t-2xl border bg-card p-4 shadow-lg md:rounded-2xl"
    >
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
      {v && (
        <dl className="mt-4 grid grid-cols-3 gap-2 text-sm">
          <div>
            <dt className="text-muted-foreground">{t("vehicle.speed")}</dt>
            <dd className="font-medium tabular-nums">{t("vehicle.speedValue", { value: Math.round(v.speed_mps * 3.6) })}</dd>
          </div>
          {v.delay_s !== undefined && (
            <div>
              <dt className="text-muted-foreground">{t("vehicle.delay")}</dt>
              <dd className="font-medium tabular-nums">
                {v.delay_s === 0
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
    </section>
  );
}

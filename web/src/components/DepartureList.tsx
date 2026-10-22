import { useTranslation } from "react-i18next";

import type { Departure } from "@/api";
import { clock, countdown } from "@/lib/format";
import { useNow } from "@/lib/useLive";

import { LinePill } from "./LinePill";

export function DepartureList({ departures, showPlatform = true }: { departures: Departure[]; showPlatform?: boolean }) {
  const { t, i18n } = useTranslation();
  const now = useNow(10_000);
  return (
    <ol className="divide-y">
      {departures.map((d) => {
        const lateMin = Math.round(d.delay_s / 60);
        return (
          <li key={`${d.stop_id}-${d.line}-${d.scheduled_at}-${d.headsign}`} className="flex items-center gap-3 py-3">
            <LinePill mode={d.mode} line={d.line} link />
            <div className="min-w-0 flex-1">
              <p className="truncate font-medium">{d.headsign}</p>
              <p className="text-xs text-muted-foreground">
                {showPlatform && d.platform && <>{t("stop.platform", { letter: d.platform })} · </>}
                {clock(d.expected_at, i18n.resolvedLanguage)}
                {d.realtime && lateMin >= 2 && <span className="text-amber-700 dark:text-amber-400"> · {t("stop.delayed", { min: lateMin })}</span>}
                {!d.realtime && <> · {t("stop.scheduled")}</>}
              </p>
            </div>
            <span className="text-lg font-semibold tabular-nums whitespace-nowrap">{countdown(d.expected_at, now, t, i18n.resolvedLanguage)}</span>
          </li>
        );
      })}
    </ol>
  );
}

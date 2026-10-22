import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { X } from "lucide-react";
import { useTranslation } from "react-i18next";

import { fetchDepartures, fetchStop } from "@/api";
import { Button } from "@/components/ui/button";

import { DepartureList } from "./DepartureList";

/** Bottom sheet for a tapped stop on the map: next departures from that platform. */
export function StopSheet({ id, onClose }: { id: string; onClose: () => void }) {
  const { t } = useTranslation();
  const stop = useQuery({ queryKey: ["stop", id], queryFn: ({ signal }) => fetchStop(id, signal), staleTime: 60 * 60_000 });
  const platform = stop.data?.platforms.find((p) => p.id === id);
  const deps = useQuery({
    queryKey: ["departures", stop.data?.id, id],
    queryFn: ({ signal }) => fetchDepartures(stop.data!.id, { platform: id, limit: 5 }, signal),
    enabled: !!stop.data,
    refetchInterval: 20_000,
  });
  const title = stop.data ? `${stop.data.name}${platform?.platform ? " " + platform.platform : ""}` : t("common.loading");
  return (
    <section aria-label={title} className="pointer-events-auto mx-auto w-full max-w-md rounded-t-2xl border bg-card p-4 shadow-lg md:rounded-2xl">
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <p className="truncate font-semibold">{title}</p>
          {platform && platform.lines.length > 0 && (
            <p className="truncate text-sm text-muted-foreground">
              {t("stop.towards", { list: [...new Set(platform.lines.flatMap((l) => l.headsigns))].slice(0, 3).join(", ") })}
            </p>
          )}
        </div>
        <Button variant="ghost" size="icon" onClick={onClose} aria-label={t("vehicle.close")}>
          <X className="size-5" />
        </Button>
      </div>
      {deps.data && deps.data.departures.length === 0 && <p className="py-3 text-sm text-muted-foreground">{t("stop.noDepartures")}</p>}
      {deps.data && <DepartureList departures={deps.data.departures} showPlatform={false} />}
      {stop.data && (
        <Button variant="outline" className="mt-2 w-full" asChild>
          <Link to="/stops/$id" params={{ id: stop.data.id }}>
            {t("stop.openStop")}
          </Link>
        </Button>
      )}
    </section>
  );
}

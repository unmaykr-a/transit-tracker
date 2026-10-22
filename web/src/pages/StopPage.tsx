import { useQuery } from "@tanstack/react-query";
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router";
import { Map as MapIcon, Route } from "lucide-react";
import { lazy, Suspense, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { fetchDepartures, fetchStop } from "@/api";
import { DepartureList } from "@/components/DepartureList";
import { LinePill } from "@/components/LinePill";
import { SplitLayout } from "@/components/SplitLayout";
import { Button } from "@/components/ui/button";
import { MODES } from "@/lib/types";
import { cn } from "@/lib/utils";

const LiveMapPane = lazy(() => import("@/map/LiveMapPane"));
const ALL_MODES = new Set(MODES);
const NO_TOKENS: string[] = [];

const route = getRouteApi("/stops/$id");
const LINE_CHIPS = 16;

export default function StopPage() {
  const { t } = useTranslation();
  const { id } = route.useParams();
  const { p: platform } = route.useSearch();
  const navigate = useNavigate({ from: "/stops/$id" });
  const [allLines, setAllLines] = useState(false);
  const stop = useQuery({ queryKey: ["stop", id], queryFn: ({ signal }) => fetchStop(id, signal), staleTime: 60 * 60_000 });
  const areaId = stop.data?.id;
  const deps = useQuery({
    queryKey: ["departures", areaId, platform ?? ""],
    queryFn: ({ signal }) => fetchDepartures(areaId!, { platform, limit: 40 }, signal),
    enabled: !!areaId,
    refetchInterval: 20_000,
  });

  const lat = stop.data?.lat;
  const lon = stop.data?.lon;
  // Frame the stop with a few streets around it.
  const fit = useMemo<[number, number][] | undefined>(
    () =>
      lat && lon
        ? [
            [lon - 0.006, lat - 0.003],
            [lon + 0.006, lat + 0.003],
          ]
        : undefined,
    [lat, lon],
  );

  if (stop.isError) return <p className="p-6 text-muted-foreground">{t("stop.notFound")}</p>;
  if (!stop.data) return <p className="p-6 text-muted-foreground">{t("common.loading")}</p>;
  const s = stop.data;
  const multi = s.platforms.length > 1;

  return (
    <SplitLayout
      map={
        <Suspense fallback={null}>
          <LiveMapPane modes={ALL_MODES} lineTokens={NO_TOKENS} fit={fit} stopId={platform ?? s.platforms[0]?.id} />
        </Suspense>
      }
    >
      <div className="mx-auto flex max-w-xl flex-col gap-5 px-4 py-6">
        <header className="flex items-start justify-between gap-4">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">{s.name}</h1>
            <p className="mt-2 flex flex-wrap items-center gap-1" aria-label={t("stop.lines")}>
              {(allLines ? s.lines : s.lines.slice(0, LINE_CHIPS)).map((l) => (
                <LinePill key={l.route_id} mode={l.mode} line={l.short_name} link className="h-6 min-w-8 px-2 text-xs" />
              ))}
              {!allLines && s.lines.length > LINE_CHIPS && (
                <button
                  type="button"
                  onClick={() => setAllLines(true)}
                  className="h-6 rounded-full border px-2 text-xs text-muted-foreground hover:bg-muted"
                >
                  +{s.lines.length - LINE_CHIPS}
                </button>
              )}
            </p>
          </div>
          <div className="flex shrink-0 flex-col items-end gap-2">
            <Button variant="outline" size="sm" asChild>
              <Link to="/map" search={{ stop: s.platforms[0]?.id }}>
                <MapIcon className="size-4" aria-hidden />
                {t("stop.showOnMap")}
              </Link>
            </Button>
            <Button variant="outline" size="sm" asChild>
              <Link to="/trips/new" search={{ from: s.id }}>
                <Route className="size-4" aria-hidden />
                {t("trip.fromHere")}
              </Link>
            </Button>
          </div>
        </header>

        {multi && (
          <nav aria-label={t("stop.platforms")} className="-mx-4 overflow-x-auto px-4">
            <div role="tablist" className="flex gap-2">
              {[
                { id: undefined, label: t("stop.all"), sub: "" },
                ...s.platforms.map((p) => ({
                  id: p.id,
                  label: p.platform,
                  sub: [...new Set(p.lines.flatMap((l) => l.headsigns))].slice(0, 3).join(", "),
                })),
              ].map((tab) => (
                <button
                  key={tab.id ?? "all"}
                  type="button"
                  role="tab"
                  aria-selected={platform === tab.id}
                  onClick={() => void navigate({ search: { p: tab.id }, replace: true })}
                  className={cn(
                    "flex max-w-48 shrink-0 flex-col items-start rounded-xl border px-3 py-2 text-left text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                    platform === tab.id ? "border-foreground bg-card" : "bg-background text-muted-foreground hover:bg-muted",
                  )}
                >
                  <span className="font-semibold">{tab.id ? t("stop.platform", { letter: tab.label }) : tab.label}</span>
                  {tab.sub && <span className="w-full truncate text-xs">{t("stop.towards", { list: tab.sub })}</span>}
                </button>
              ))}
            </div>
          </nav>
        )}

        <section className="rounded-xl border bg-card px-4">
          <h2 className="sr-only">{t("stop.departures")}</h2>
          {deps.isError && (
            <div className="flex items-center justify-between py-4">
              <p className="text-muted-foreground">{t("stop.error")}</p>
              <Button variant="outline" size="sm" onClick={() => void deps.refetch()}>
                {t("common.retry")}
              </Button>
            </div>
          )}
          {deps.isPending && !deps.isError && <p className="py-4 text-muted-foreground">{t("common.loading")}</p>}
          {deps.data && deps.data.departures.length === 0 && <p className="py-4 text-muted-foreground">{t("stop.noDepartures")}</p>}
          {deps.data && <DepartureList departures={deps.data.departures} showPlatform={multi && !platform} />}
        </section>
      </div>
    </SplitLayout>
  );
}

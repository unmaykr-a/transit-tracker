import { useQuery } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { BookmarkPlus, Map as MapIcon } from "lucide-react";
import { lazy, Suspense, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { fetchLine } from "@/api";
import { LinePill } from "@/components/LinePill";
import { SplitLayout } from "@/components/SplitLayout";
import { SaveLineSheet } from "@/components/SaveLineSheet";
import { Button } from "@/components/ui/button";
import { placeVehicles } from "@/lib/lineVehicles";
import { MODE_COLORS } from "@/lib/modeColors";
import { useSession } from "@/lib/session";
import { useLiveState } from "@/lib/useLive";
import { useWide } from "@/lib/useWide";
import { cn } from "@/lib/utils";

const LineMap = lazy(() => import("@/map/LineMap"));
const LiveMapPane = lazy(() => import("@/map/LiveMapPane"));
const route = getRouteApi("/lines/$mode/$line");

export default function LinePage() {
  const { t } = useTranslation();
  const { mode, line } = route.useParams();
  const q = useQuery({ queryKey: ["line", mode, line], queryFn: ({ signal }) => fetchLine(mode, line, signal), staleTime: 60 * 60_000 });
  const live = useLiveState(2000);
  const mains = useMemo(() => q.data?.patterns.filter((p) => p.main) ?? [], [q.data]);
  const [dir, setDir] = useState(0);
  const [saving, setSaving] = useState(false);
  const user = useSession().data?.user;
  const pattern = mains.find((p) => p.direction === dir) ?? mains[0];
  const wide = useWide();
  const lineMode = q.data?.route.mode;
  const lineName = q.data?.route.short_name;
  const modes = useMemo(() => new Set(lineMode ? [lineMode] : []), [lineMode]);
  const tokens = useMemo(() => (lineName ? [lineName.toLowerCase()] : []), [lineName]);

  const vehicles = useMemo(() => {
    if (!q.data) return [];
    const r = q.data.route;
    return [...live.vehicles.values()].filter((v) => v.mode === r.mode && v.line.toLowerCase() === r.short_name.toLowerCase());
  }, [live, q.data]);
  const placements = useMemo(() => (q.data ? placeVehicles(q.data.patterns, vehicles) : []), [q.data, vehicles]);

  if (q.isError) return <p className="p-6 text-muted-foreground">{t("line.notFound")}</p>;
  if (!q.data || !pattern) return <p className="p-6 text-muted-foreground">{t("common.loading")}</p>;
  const r = q.data.route;
  const color = MODE_COLORS[r.mode];
  const here = placements.filter((p) => p.patternId === pattern.id);

  return (
    <SplitLayout
      map={
        <Suspense fallback={null}>
          <LiveMapPane modes={modes} lineTokens={tokens} fit={pattern.shape} highlightPattern={pattern.id} highlightMode={r.mode} />
        </Suspense>
      }
    >
      <div className="mx-auto flex max-w-xl flex-col gap-5 px-4 py-6">
        <header className="flex items-start gap-3">
          <LinePill mode={r.mode} line={r.short_name} className="h-9 min-w-12 text-base" />
          <div className="min-w-0 flex-1">
            <h1 className="text-xl font-semibold tracking-tight">{r.long_name}</h1>
            <p className="text-sm text-muted-foreground">
              {r.source === "county" ? t("line.county") : `${t(`mode.${r.mode}`)} · ${t("line.live", { count: vehicles.length })}`}
            </p>
          </div>
          <Button variant="outline" size="sm" asChild>
            <Link to="/map" search={{ lines: r.short_name }}>
              <MapIcon className="size-4" aria-hidden />
              <span className="sr-only sm:not-sr-only">{t("line.showOnMap")}</span>
            </Link>
          </Button>
        </header>

        {saving ? (
          <SaveLineSheet route={r} pattern={pattern} onClose={() => setSaving(false)} />
        ) : user ? (
          <Button onClick={() => setSaving(true)} className="self-start">
            <BookmarkPlus className="size-4" aria-hidden />
            {t("save.button")}
          </Button>
        ) : (
          <Button variant="outline" asChild className="self-start">
            <Link to="/login" search={{ next: `/lines/${r.mode}/${r.short_name}` }}>
              {t("save.loginFirst")}
            </Link>
          </Button>
        )}

        {mains.length > 1 && (
          <div role="radiogroup" aria-label={t("line.direction")} className="grid grid-cols-2 gap-2">
            {mains.map((p) => (
              <button
                key={p.id}
                type="button"
                role="radio"
                aria-checked={p.id === pattern.id}
                onClick={() => setDir(p.direction)}
                className={cn(
                  "truncate rounded-xl border px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                  p.id === pattern.id ? "border-foreground bg-card" : "text-muted-foreground hover:bg-muted",
                )}
              >
                → {p.headsign}
              </button>
            ))}
          </div>
        )}

        {!wide && (
          <div className="relative h-56 overflow-hidden rounded-xl border">
            <Suspense fallback={null}>
              <LineMap
                shape={pattern.shape}
                stops={pattern.stops}
                vehicles={vehicles.filter((v) => here.some((h) => h.vehicle.id === v.id))}
                color={color}
              />
            </Suspense>
          </div>
        )}

        <section>
          <h2 className="mb-2 text-sm font-medium text-muted-foreground">{t("line.stops", { count: pattern.stops.length })}</h2>
          <ol className="relative">
            {pattern.stops.map((s, i) => {
              const at = here.filter((h) => h.stopIndex === i);
              return (
                <li key={`${s.id}-${i}`} className="relative flex min-h-12 items-center gap-3 pl-8">
                  <span
                    aria-hidden
                    className="absolute top-0 bottom-0 left-3 w-1"
                    style={{ background: color, top: i === 0 ? "50%" : 0, bottom: i === pattern.stops.length - 1 ? "50%" : 0 }}
                  />
                  <span aria-hidden className="absolute left-1.5 size-4 rounded-full border-[3px] bg-background" style={{ borderColor: color }} />
                  <Link to="/stops/$id" params={{ id: s.id }} className="min-w-0 flex-1 truncate py-2 hover:underline">
                    {s.name}
                    {s.platform && <span className="text-muted-foreground"> · {s.platform}</span>}
                  </Link>
                  {at.map((h) => (
                    <span key={h.vehicle.id} className="rounded-full px-2 py-0.5 text-xs font-medium text-white" style={{ background: color }}>
                      {h.atStop ? t("line.here") : t("line.approaching")}
                    </span>
                  ))}
                </li>
              );
            })}
          </ol>
        </section>
      </div>
    </SplitLayout>
  );
}

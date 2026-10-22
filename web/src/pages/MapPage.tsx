import { useQuery } from "@tanstack/react-query";
import { getRouteApi, useNavigate } from "@tanstack/react-router";
import { X } from "lucide-react";
import { lazy, Suspense, useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { fetchAllStops } from "@/api";
import { StopSheet } from "@/components/StopSheet";
import type { Selection } from "@/map/VehicleMap";

import { StaleBanner } from "@/components/StaleBanner";
import { VehicleSheet } from "@/components/VehicleSheet";
import { matchesFilter, parseLineQuery } from "@/lib/lines";
import { MODE_BG } from "@/lib/modeColors";
import { load, save } from "@/lib/storage";
import { MODES, type Mode } from "@/lib/types";
import { useLiveState } from "@/lib/useLive";
import { cn } from "@/lib/utils";

const VehicleMap = lazy(() => import("@/map/VehicleMap"));

const NO_STOPS: never[] = [];

interface Filters {
  modes: Mode[];
  lines: string;
}

const FILTER_KEY = "bus.map.filters";
const route = getRouteApi("/map");

export default function MapPage() {
  const { t } = useTranslation();
  const search = route.useSearch();
  const navigate = useNavigate({ from: "/map" });
  // ?lines= (from a line page) overrides the saved line filter for this visit.
  const [filters, setFilters] = useState<Filters>(() => {
    const saved = load(FILTER_KEY, { modes: [...MODES], lines: "" });
    return search.lines ? { modes: [...MODES], lines: search.lines } : saved;
  });
  const [selected, setSelected] = useState<Selection | undefined>(() => (search.stop ? { kind: "stop", id: search.stop } : undefined));
  const state = useLiveState(2000);
  const stops = useQuery({ queryKey: ["all-stops"], queryFn: ({ signal }) => fetchAllStops(signal), staleTime: 6 * 60 * 60_000 });
  const focus = useMemo<[number, number] | undefined>(() => {
    const s = search.stop && stops.data?.stops.find((x) => x.id === search.stop);
    return s ? [s.lon, s.lat] : undefined;
  }, [search.stop, stops.data]);

  const update = (f: Filters) => {
    setFilters(f);
    save(FILTER_KEY, f);
  };
  const modes = useMemo(() => new Set(filters.modes), [filters.modes]);
  const tokens = useMemo(() => parseLineQuery(filters.lines), [filters.lines]);
  const visible = useMemo(() => {
    let n = 0;
    for (const v of state.vehicles.values()) if (matchesFilter(v, modes, tokens)) n++;
    return n;
  }, [state, modes, tokens]);
  const toggle = (m: Mode) =>
    update({ ...filters, modes: modes.has(m) ? filters.modes.filter((x) => x !== m) : [...filters.modes, m] });
  const onSelect = useCallback(
    (sel: Selection | undefined) => {
      setSelected(sel);
      if (search.stop) void navigate({ search: { lines: search.lines }, replace: true });
    },
    [navigate, search.stop, search.lines],
  );

  return (
    <div className="absolute inset-0">
      <Suspense fallback={<p className="p-4 text-muted-foreground">{t("map.loading")}</p>}>
        <VehicleMap
          modes={modes}
          lineTokens={tokens}
          stops={stops.data?.stops ?? NO_STOPS}
          selected={selected}
          onSelect={onSelect}
          focus={focus}
          highlightPattern={selected?.kind === "vehicle" ? state.vehicles.get(selected.id)?.pattern_id : undefined}
        />
      </Suspense>

      <div className="pointer-events-none absolute inset-x-0 top-0 z-10 flex flex-col gap-2 p-3 pr-14">
        <div className="pointer-events-auto flex flex-wrap items-center gap-2" role="group" aria-label={t("map.filters")}>
          {MODES.map((m) => (
            <button
              key={m}
              type="button"
              aria-pressed={modes.has(m)}
              onClick={() => toggle(m)}
              className={cn(
                "h-8 rounded-full border px-3 text-sm font-medium shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                modes.has(m) ? cn(MODE_BG[m], "border-transparent text-white") : "bg-background text-muted-foreground",
              )}
            >
              {t(`modeShort.${m}`)}
            </button>
          ))}
          <label className="relative flex items-center">
            <span className="sr-only">{t("map.lineFilter")}</span>
            <input
              value={filters.lines}
              onChange={(e) => update({ ...filters, lines: e.target.value })}
              placeholder={t("map.lineFilterPlaceholder")}
              inputMode="text"
              autoCapitalize="characters"
              className="h-8 w-32 rounded-full border bg-background px-3 pr-7 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
            {filters.lines && (
              <button
                type="button"
                onClick={() => update({ ...filters, lines: "" })}
                aria-label={t("map.clearLines")}
                className="absolute right-1.5 text-muted-foreground hover:text-foreground"
              >
                <X className="size-4" />
              </button>
            )}
          </label>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <span className="rounded-full bg-background/90 px-3 py-1 text-xs text-muted-foreground shadow-sm tabular-nums" aria-live="polite">
            {t("live.vehicles", { count: visible })}
          </span>
          <StaleBanner />
        </div>
      </div>

      {selected && (
        <div className="pointer-events-none absolute inset-x-0 bottom-0 z-10 md:bottom-4 md:left-4 md:right-auto md:w-96">
          {selected.kind === "vehicle" ? (
            <VehicleSheet id={selected.id} onClose={() => setSelected(undefined)} />
          ) : (
            <StopSheet id={selected.id} onClose={() => setSelected(undefined)} />
          )}
        </div>
      )}
    </div>
  );
}

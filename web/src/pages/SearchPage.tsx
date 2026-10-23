import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router";
import { Building2, MapPin, Search } from "lucide-react";
import { useDeferredValue, useState } from "react";
import { useTranslation } from "react-i18next";

import { fetchPlaces, fetchSearch } from "@/api";
import { LinePill } from "@/components/LinePill";
import { MODE_BG } from "@/lib/modeColors";
import { useSession } from "@/lib/session";
import { cn } from "@/lib/utils";

const route = getRouteApi("/search");

export default function SearchPage() {
  const { t } = useTranslation();
  const { q: initial = "" } = route.useSearch();
  const navigate = useNavigate({ from: "/search" });
  const [q, setQ] = useState(initial);
  const deferred = useDeferredValue(q.trim());
  const res = useQuery({
    queryKey: ["search", deferred],
    queryFn: ({ signal }) => fetchSearch(deferred, signal),
    enabled: deferred.length > 0,
    placeholderData: keepPreviousData,
    staleTime: 5 * 60_000,
  });
  const data = deferred ? res.data : undefined;
  const empty = data && data.lines.length === 0 && data.stops.length === 0;
  // "4", "24A", "T4" → lines first; anything else is probably a stop name.
  const linesFirst = /^t?\d/i.test(deferred);
  // Addresses and named places ("Stockmann") with their nearest stops: for when the stop's name is unknown.
  const placesQ = deferred.length >= 3 && !/^t?\d+[a-z]?$/i.test(deferred) ? deferred : "";
  const placeRes = useQuery({
    queryKey: ["places-search", placesQ],
    queryFn: ({ signal }) => fetchPlaces(placesQ, signal),
    enabled: !!placesQ,
    staleTime: 60 * 60_000,
    retry: false,
  });
  const placeHits = placesQ ? (placeRes.data?.places ?? []) : [];
  const signedIn = !!useSession().data?.user;

  return (
    <div className="mx-auto flex max-w-xl flex-col gap-6 px-4 py-6">
      <label className="relative block">
        <span className="sr-only">{t("search.label")}</span>
        <Search className="pointer-events-none absolute top-1/2 left-3 size-5 -translate-y-1/2 text-muted-foreground" aria-hidden />
        <input
          type="search"
          autoFocus
          value={q}
          onChange={(e) => {
            setQ(e.target.value);
            void navigate({ search: { q: e.target.value || undefined }, replace: true });
          }}
          placeholder={t("search.placeholder")}
          className="h-12 w-full rounded-xl border bg-card pr-4 pl-10 text-base shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </label>

      {!deferred && <p className="text-muted-foreground">{t("search.hint")}</p>}
      {empty && placeHits.length === 0 && !placeRes.isFetching && <p className="text-muted-foreground">{t("search.noResults", { q: deferred })}</p>}

      <div className={cn("flex gap-6", linesFirst ? "flex-col" : "flex-col-reverse")}>
      {data && data.lines.length > 0 && (
        <section className="flex flex-col gap-2">
          <h2 className="text-sm font-medium text-muted-foreground">{t("search.lines")}</h2>
          <ul className="divide-y rounded-xl border bg-card">
            {data.lines.map((l) => (
              <li key={l.id}>
                <Link
                  to="/lines/$mode/$line"
                  params={{ mode: l.mode, line: l.short_name }}
                  className="flex items-center gap-3 px-4 py-3 hover:bg-muted focus-visible:bg-muted focus-visible:outline-none"
                >
                  {/* A fixed column, so the names line up whether the pill says 2 or 24A. */}
                  <span className="flex w-12 shrink-0">
                    <LinePill mode={l.mode} line={l.short_name} />
                  </span>
                  <span className="min-w-0 flex-1 truncate">{l.long_name}</span>
                  <span className="text-xs text-muted-foreground">{t(`modeShort.${l.mode}`)}</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}

      {data && data.stops.length > 0 && (
        <section className="flex flex-col gap-2">
          <h2 className="text-sm font-medium text-muted-foreground">{t("search.stops")}</h2>
          <ul className="divide-y rounded-xl border bg-card">
            {data.stops.map((s) => (
              <li key={s.id}>
                <Link
                  to="/stops/$id"
                  params={{ id: s.id }}
                  className="flex items-start gap-3 px-4 py-3 hover:bg-muted focus-visible:bg-muted focus-visible:outline-none"
                >
                  <MapPin className="mt-0.5 size-5 shrink-0 text-muted-foreground" aria-hidden />
                  <div className="min-w-0 flex-1">
                    <p className="flex items-center gap-2 font-medium">
                      {s.name}
                      <span className="flex gap-1" aria-hidden>
                        {s.modes.map((m) => (
                          <span key={m} className={cn("size-2 rounded-full", MODE_BG[m])} />
                        ))}
                      </span>
                    </p>
                    <p className="mt-1 flex flex-wrap gap-1">
                      {s.lines.slice(0, 12).map((l) => (
                        <LinePill key={l.route_id} mode={l.mode} line={l.short_name} className="h-5 min-w-7 px-1.5 text-xs" />
                      ))}
                      {s.lines.length > 12 && <span className="text-xs text-muted-foreground">+{s.lines.length - 12}</span>}
                    </p>
                  </div>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}
      </div>

      {placeHits.length > 0 && (
        <section className="flex flex-col gap-2">
          <h2 className="text-sm font-medium text-muted-foreground">{t("search.places")}</h2>
          <ul className="flex flex-col gap-3">
            {placeHits.map((p) => (
              <li key={`${p.lat},${p.lon}`} className="rounded-xl border bg-card">
                <div className="flex items-start gap-3 px-4 pt-3">
                  <Building2 className="mt-0.5 size-5 shrink-0 text-muted-foreground" aria-hidden />
                  <div className="min-w-0 flex-1">
                    <p className="font-medium">{p.label}</p>
                    <p className="text-xs text-muted-foreground">{p.detail}</p>
                  </div>
                </div>
                <p className="px-4 pt-2 text-xs font-medium text-muted-foreground">{t("search.nearestStops")}</p>
                <ul className="divide-y">
                  {p.stops.map((s) => (
                    <li key={s.id} className="flex items-center gap-2 pr-2">
                      <Link
                        to="/stops/$id"
                        params={{ id: s.id }}
                        className="flex min-w-0 flex-1 items-center gap-3 px-4 py-2 hover:bg-muted focus-visible:bg-muted focus-visible:outline-none"
                      >
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-sm font-medium">{s.name}</span>
                          <span className="mt-1 flex flex-wrap gap-1">
                            {s.lines.slice(0, 8).map((l) => (
                              <LinePill key={l.route_id} mode={l.mode} line={l.short_name} className="h-5 min-w-7 px-1.5 text-xs" />
                            ))}
                            {s.lines.length > 8 && <span className="text-xs text-muted-foreground">+{s.lines.length - 8}</span>}
                          </span>
                        </span>
                        <span className="shrink-0 text-xs text-muted-foreground tabular-nums">{t("search.distance", { m: Math.round(s.dist_m / 10) * 10 })}</span>
                      </Link>
                      {signedIn && (
                        <Link
                          to="/trips/new"
                          search={{ to: s.id }}
                          className="shrink-0 rounded-md px-2 py-1 text-xs font-medium text-primary hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          aria-label={t("search.tripToLabel", { stop: s.name })}
                        >
                          {t("search.tripTo")}
                        </Link>
                      )}
                    </li>
                  ))}
                </ul>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}

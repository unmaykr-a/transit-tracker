import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router";
import { MapPin, Search } from "lucide-react";
import { useDeferredValue, useState } from "react";
import { useTranslation } from "react-i18next";

import { fetchSearch } from "@/api";
import { LinePill } from "@/components/LinePill";
import { MODE_BG } from "@/lib/modeColors";
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
      {empty && <p className="text-muted-foreground">{t("search.noResults", { q: deferred })}</p>}

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
    </div>
  );
}

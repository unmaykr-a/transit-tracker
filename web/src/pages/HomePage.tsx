import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { LocateFixed, MapPin } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { fetchDashboard, myLines, places, type Origin } from "@/api";
import { LineCard } from "@/components/LineCard";
import { StaleBanner } from "@/components/StaleBanner";
import { Button } from "@/components/ui/button";
import { useGeolocation } from "@/lib/geolocation";
import { useSession } from "@/lib/session";
import { load, save } from "@/lib/storage";
import { useNow } from "@/lib/useLive";
import { cn } from "@/lib/utils";

type OriginChoice = "gps" | number;
const ORIGIN_KEY = "bus.origin";

/** GPS positions are rounded to ~100 m in the query key so jitter does not refetch. */
const coarse = (v: number) => Math.round(v * 1000) / 1000;

function Dashboard() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const now = useNow(5_000);
  const geo = useGeolocation();
  const [choice, setChoice] = useState<OriginChoice>(() => load<OriginChoice>(ORIGIN_KEY, "gps"));
  const [editing, setEditing] = useState(false);
  const saved = useQuery({ queryKey: ["places"], queryFn: ({ signal }) => places.list(signal) });

  let origin: Origin = null;
  if (choice === "gps" && geo.state.status === "on") origin = { lat: coarse(geo.state.lat), lon: coarse(geo.state.lon) };
  if (typeof choice === "number" && saved.data?.places.some((p) => p.id === choice)) origin = { place: choice };

  const dash = useQuery({
    queryKey: ["dashboard", origin],
    queryFn: ({ signal }) => fetchDashboard(origin, signal),
    refetchInterval: 20_000,
    placeholderData: keepPreviousData,
  });
  const reorder = useMutation({ mutationFn: myLines.reorder, onSuccess: () => qc.invalidateQueries({ queryKey: ["dashboard"] }) });
  const remove = useMutation({ mutationFn: myLines.remove, onSuccess: () => qc.invalidateQueries({ queryKey: ["dashboard"] }) });

  const pickOrigin = (c: OriginChoice) => {
    setChoice(c);
    save(ORIGIN_KEY, c);
    if (c === "gps" && !geo.enabled) geo.enable();
  };
  const entries = dash.data?.entries ?? [];
  const move = (i: number, delta: -1 | 1) => {
    const ids = entries.map((e) => e.line.id);
    const j = i + delta;
    [ids[i], ids[j]] = [ids[j]!, ids[i]!];
    reorder.mutate(ids);
  };

  const gpsLabel =
    geo.state.status === "locating" ? t("home.locating") : geo.state.status === "denied" ? t("home.locationDenied") : t("home.myLocation");

  if (dash.data && entries.length === 0) {
    return (
      <section className="flex flex-col items-start gap-4 rounded-xl border bg-card p-4">
        <p className="text-muted-foreground">{t("home.noLines")}</p>
        <Button asChild>
          <Link to="/search">{t("home.findLine")}</Link>
        </Button>
      </section>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2" role="radiogroup" aria-label={t("home.origin")}>
        <span className="text-sm text-muted-foreground">{t("home.origin")}:</span>
        <button
          type="button"
          role="radio"
          aria-checked={choice === "gps"}
          onClick={() => pickOrigin("gps")}
          className={cn(
            "inline-flex h-8 items-center gap-1.5 rounded-full border px-3 text-sm",
            choice === "gps" ? "border-foreground bg-card font-medium" : "text-muted-foreground",
          )}
        >
          <LocateFixed className="size-4" aria-hidden />
          {choice === "gps" && !geo.enabled ? t("home.useLocation") : gpsLabel}
        </button>
        {saved.data?.places.map((p) => (
          <button
            key={p.id}
            type="button"
            role="radio"
            aria-checked={choice === p.id}
            onClick={() => pickOrigin(p.id)}
            className={cn(
              "inline-flex h-8 items-center gap-1.5 rounded-full border px-3 text-sm",
              choice === p.id ? "border-foreground bg-card font-medium" : "text-muted-foreground",
            )}
          >
            <MapPin className="size-4" aria-hidden />
            {p.label}
          </button>
        ))}
        <Button variant="ghost" size="sm" className="ml-auto" onClick={() => setEditing((e) => !e)}>
          {editing ? t("home.done") : t("home.edit")}
        </Button>
      </div>
      {dash.data && !dash.data.has_origin && <p className="text-sm text-muted-foreground">{t("home.noOrigin")}</p>}
      {entries.map((e, i) => (
        <LineCard
          key={e.line.id}
          entry={e}
          now={now}
          bufferMin={dash.data?.prefs.buffer_min ?? 1}
          editing={editing}
          first={i === 0}
          last={i === entries.length - 1}
          onMove={(d) => move(i, d)}
          onRemove={() => remove.mutate(e.line.id)}
        />
      ))}
    </div>
  );
}

export default function HomePage() {
  const { t } = useTranslation();
  const session = useSession();
  const user = session.data?.user;
  return (
    <div className="mx-auto flex max-w-xl flex-col gap-6 px-4 py-6">
      <header className="flex flex-col gap-1">
        <h1 className="text-2xl font-semibold tracking-tight">{t("home.title")}</h1>
        {!user && <p className="text-muted-foreground">{t("home.intro")}</p>}
      </header>
      <StaleBanner className="self-start" />
      {user ? (
        <Dashboard />
      ) : (
        session.data && (
          <section className="flex flex-col items-start gap-4 rounded-xl border bg-card p-4">
            <p className="text-muted-foreground">{t("home.loginToSave")}</p>
            <div className="flex gap-2">
              <Button asChild>
                <Link to="/login">{t("auth.login")}</Link>
              </Button>
              <Button variant="outline" asChild>
                <Link to="/map">{t("home.openMap")}</Link>
              </Button>
            </div>
          </section>
        )
      )}
    </div>
  );
}

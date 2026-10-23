import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { LocateFixed, MapPin, Plus, X } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { fetchDashboard, myLines, places, type Dashboard as DashboardData, type Origin } from "@/api";
import { AddPlaceForm } from "@/components/AddPlaceForm";
import { LineCard } from "@/components/LineCard";
import { StaleBanner } from "@/components/StaleBanner";
import { Button } from "@/components/ui/button";
import { useGeolocation } from "@/lib/geolocation";
import { useSession } from "@/lib/session";
import { load, save } from "@/lib/storage";
import { useNow } from "@/lib/useLive";
import { cn } from "@/lib/utils";
import { radioKeyDown, radioTabIndex } from "@/lib/radioGroup";

type OriginChoice = "gps" | number;
const chip = "inline-flex h-8 shrink-0 items-center gap-1.5 whitespace-nowrap rounded-full border px-3 text-sm";
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
  const [adding, setAdding] = useState(false);
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
  // Reorder optimistically, so a second quick tap moves from the order the first one produced.
  const reorder = useMutation({
    mutationFn: myLines.reorder,
    onMutate: async (ids: number[]) => {
      await qc.cancelQueries({ queryKey: ["dashboard"] });
      qc.setQueriesData<DashboardData>({ queryKey: ["dashboard"] }, (old) =>
        old ? { ...old, entries: ids.flatMap((id) => old.entries.filter((e) => e.line.id === id)) } : old,
      );
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ["dashboard"] }),
  });
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

  const originChecked = choice === "gps" || !!saved.data?.places.some((p) => p.id === choice);
  const gpsLabel =
    geo.state.status === "locating" ? t("home.locating") : geo.state.status === "denied" ? t("home.locationDenied") : t("home.myLocation");

  if (!dash.data) {
    return dash.isError ? (
      <div className="flex items-center justify-between gap-4 rounded-xl border bg-card p-4">
        <p className="text-muted-foreground">{t("common.error")}</p>
        <Button variant="outline" size="sm" onClick={() => void dash.refetch()}>
          {t("common.retry")}
        </Button>
      </div>
    ) : (
      <p className="text-muted-foreground">{t("common.loading")}</p>
    );
  }

  if (entries.length === 0) {
    return (
      <section className="flex flex-col items-start gap-4 rounded-xl border bg-card p-4">
        <p className="text-muted-foreground">{t("home.noLines")}</p>
        <div className="flex flex-wrap gap-2">
          <Button asChild>
            <Link to="/search">{t("home.findLine")}</Link>
          </Button>
          <Button variant="outline" asChild>
            <Link to="/trips/new">{t("trip.add")}</Link>
          </Button>
        </div>
      </section>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <div className="flex items-center justify-between gap-2">
          <span id="origin-label" className="text-sm text-muted-foreground">
            {t("home.origin")}
          </span>
          <div className="flex gap-1">
            <Button variant="ghost" size="sm" asChild>
              <Link to="/trips/new">{t("trip.add")}</Link>
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setEditing((e) => !e)} aria-pressed={editing}>
              {editing ? t("home.done") : t("home.edit")}
            </Button>
          </div>
        </div>
        {/* One scrolling row on phones, so the cards stay near the top; wraps on wide screens. */}
        <div className="-mx-4 flex gap-2 overflow-x-auto px-4 pb-1 lg:flex-wrap lg:overflow-visible">
          <div role="radiogroup" aria-labelledby="origin-label" className="flex shrink-0 gap-2 lg:shrink lg:flex-wrap">
            <button
              type="button"
              role="radio"
              aria-checked={choice === "gps"}
              tabIndex={radioTabIndex(choice === "gps", 0, originChecked)}
              onKeyDown={radioKeyDown}
              onClick={() => pickOrigin("gps")}
              className={cn(chip, choice === "gps" ? "border-foreground bg-card font-medium" : "text-muted-foreground")}
            >
              <LocateFixed className="size-4 shrink-0" aria-hidden />
              {choice === "gps" && !geo.enabled ? t("home.useLocation") : gpsLabel}
            </button>
            {saved.data?.places.map((p, i) => (
              <button
                key={p.id}
                type="button"
                role="radio"
                aria-checked={choice === p.id}
                tabIndex={radioTabIndex(choice === p.id, i + 1, originChecked)}
                onKeyDown={radioKeyDown}
                onClick={() => pickOrigin(p.id)}
                className={cn(chip, choice === p.id ? "border-foreground bg-card font-medium" : "text-muted-foreground")}
              >
                <MapPin className="size-4 shrink-0" aria-hidden />
                <span className="max-w-48 truncate">{p.label}</span>
              </button>
            ))}
          </div>
          <button
            type="button"
            aria-expanded={adding}
            aria-controls="add-place"
            onClick={() => setAdding((a) => !a)}
            className={cn(chip, "border-dashed text-muted-foreground hover:text-foreground")}
          >
            <Plus className="size-4 shrink-0" aria-hidden />
            {t("home.addPlace")}
          </button>
        </div>
        {adding && (
          <section id="add-place" aria-labelledby="add-place-title" className="flex flex-col gap-3 rounded-xl border bg-card p-4 shadow-sm">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h2 id="add-place-title" className="font-semibold">
                  {t("home.addPlace")}
                </h2>
                <p className="text-sm text-muted-foreground">{t("home.addPlaceHint")}</p>
              </div>
              <Button variant="ghost" size="icon" onClick={() => setAdding(false)} aria-label={t("vehicle.close")}>
                <X className="size-5" />
              </Button>
            </div>
            <AddPlaceForm
              autoFocus
              onAdded={(p) => {
                setAdding(false);
                pickOrigin(p.id);
              }}
            />
          </section>
        )}
      </div>
      {dash.data && !dash.data.has_origin && <p className="text-sm text-muted-foreground">{t("home.noOrigin")}</p>}
      <div className="grid gap-4 lg:grid-cols-2">
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
    </div>
  );
}

export default function HomePage() {
  const { t } = useTranslation();
  const session = useSession();
  const user = session.data?.user;
  return (
    <div className="mx-auto flex max-w-xl flex-col gap-6 px-4 py-6 lg:max-w-5xl">
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

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router";
import { ArrowUpDown, ChevronRight, Footprints, MapPin, Plus, X } from "lucide-react";
import { useDeferredValue, useId, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  ApiError,
  fetchSearch,
  fetchStop,
  fetchTripLines,
  myLines,
  places,
  transfersOf,
  viaOf,
  type TransferRef,
  type TripLeg,
  type TripLine,
} from "@/api";
import { LinePill } from "@/components/LinePill";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { useSession } from "@/lib/session";
import { cn } from "@/lib/utils";
import { radioKeyDown, radioTabIndex } from "@/lib/radioGroup";

const tripRoute = getRouteApi("/trips/new");

interface Pick {
  id: string;
  name: string;
}

/** Search for a stop area; shows the choice once made. */
function StopPicker({ label, value, onChange }: { label: string; value?: Pick; onChange: (p?: Pick) => void }) {
  const { t } = useTranslation();
  const [q, setQ] = useState("");
  const query = useDeferredValue(q.trim());
  const listId = useId();
  const results = useQuery({
    queryKey: ["search", query],
    queryFn: ({ signal }) => fetchSearch(query, signal),
    enabled: !value && query.length >= 2,
  });
  if (value) {
    return (
      <div className="flex flex-col gap-1.5">
        <span className="text-sm font-medium">{label}</span>
        <div className="flex items-center justify-between gap-2 rounded-lg border bg-card px-3 py-2">
          <span className="font-medium">{value.name}</span>
          <Button variant="ghost" size="icon" onClick={() => onChange(undefined)} aria-label={t("trip.change", { what: label })}>
            <X className="size-4" />
          </Button>
        </div>
      </div>
    );
  }
  const stops = results.data?.stops ?? [];
  return (
    <div className="flex flex-col gap-1.5">
      <Field label={label}>
        <Input
          type="search"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={t("trip.searchStop")}
          aria-controls={query.length >= 2 ? listId : undefined}
          autoComplete="off"
        />
      </Field>
      {query.length >= 2 && (
        <ul id={listId} className="divide-y rounded-lg border bg-card" aria-label={t("trip.results", { what: label })}>
          {stops.length === 0 && !results.isPending && <li className="px-3 py-2 text-sm text-muted-foreground">{t("search.noResults", { q: query })}</li>}
          {stops.map((s) => (
            <li key={s.id}>
              <button
                type="button"
                className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left hover:bg-muted focus-visible:bg-muted focus-visible:outline-none"
                onClick={() => {
                  onChange({ id: s.id, name: s.name });
                  setQ("");
                }}
              >
                <span className="font-medium">{s.name}</span>
                <span className="flex flex-wrap justify-end gap-1">
                  {s.lines.slice(0, 5).map((l) => (
                    <LinePill key={l.route_id} mode={l.mode} line={l.short_name} className="h-6 min-w-8 text-xs" />
                  ))}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/** A few line pills for a leg, then "+n". */
function LegPills({ lines }: { lines: TripLine[] }) {
  const unique = [...new Map(lines.map((l) => [l.route_id, l])).values()];
  return (
    <span className="inline-flex flex-wrap items-center gap-1">
      {unique.slice(0, 3).map((l) => (
        <LinePill key={l.route_id} mode={l.mode} line={l.short_name} className="h-6 min-w-8 text-xs" />
      ))}
      {unique.length > 3 && <span className="text-xs text-muted-foreground">+{unique.length - 3}</span>}
    </span>
  );
}

/** One way to ride, on a line: T4 › Viru › 17 · about 32 min. */
function RouteSummary({ legs, transfers }: { legs: TripLeg[]; transfers: TransferRef[] }) {
  return (
    <span className="flex flex-wrap items-center gap-x-1.5 gap-y-1">
      {legs.map((leg, i) => (
        <span key={i} className="inline-flex items-center gap-1.5">
          {i > 0 && (
            <>
              <ChevronRight className="size-3.5 text-muted-foreground" aria-hidden />
              <span className="text-sm">{transfers[i - 1]?.alight?.name}</span>
              <ChevronRight className="size-3.5 text-muted-foreground" aria-hidden />
            </>
          )}
          <LegPills lines={leg.lines} />
        </span>
      ))}
    </span>
  );
}

/** The chosen way, leg by leg: where each ride goes, its lines, and each change. */
function LegList({ legs, transfers }: { legs: TripLeg[]; transfers: TransferRef[] }) {
  const { t } = useTranslation();
  return (
    <ol className="flex flex-col gap-2">
      {legs.map((leg, i) => (
        <li key={i} className="flex flex-col gap-2">
          {i > 0 && transfers[i - 1] && (
            <p className="flex items-center gap-1.5 px-1 text-sm text-muted-foreground">
              <Footprints className="size-4" aria-hidden />
              {transfers[i - 1]!.walk_m > 0
                ? t("trip.walkChange", { m: Math.round(transfers[i - 1]!.walk_m), from: transfers[i - 1]!.alight?.name, to: transfers[i - 1]!.board?.name })
                : t("trip.changeAt", { stop: transfers[i - 1]!.alight?.name })}
            </p>
          )}
          <div className="rounded-lg border bg-card">
            <p className="border-b px-3 py-2 text-sm font-medium">{t("trip.leg", { from: leg.from?.name, to: leg.to?.name })}</p>
            <ul className="divide-y">
              {leg.lines.map((l) => (
                <li key={l.route_id + l.board_stop.id} className="flex items-center gap-3 px-3 py-2">
                  <span className="flex w-12 shrink-0">
                    <LinePill mode={l.mode} line={l.short_name} className="h-7 min-w-10" />
                  </span>
                  <div className="min-w-0 flex-1 text-sm">
                    <p className="truncate font-medium">→ {l.headsigns[0]}</p>
                    <p className="truncate text-muted-foreground">
                      {t("home.from", { stop: `${l.board_stop.name}${l.board_stop.platform ? " " + l.board_stop.platform : ""}` })} ·{" "}
                      {t("trip.stops", { count: l.stops })}
                    </p>
                  </div>
                </li>
              ))}
            </ul>
          </div>
        </li>
      ))}
    </ol>
  );
}

const CUSTOM = "custom";

export default function TripPage() {
  const { t } = useTranslation();
  const session = useSession();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const search = tripRoute.useSearch();
  const editId = search.edit;
  const saved = useQuery({ queryKey: ["my-lines"], queryFn: ({ signal }) => myLines.list(signal), enabled: !!editId });
  const placeList = useQuery({ queryKey: ["places"], queryFn: ({ signal }) => places.list(signal), enabled: !!session.data?.user });
  const existing = editId ? saved.data?.lines.find((l) => l.id === editId && l.kind === "trip") : undefined;
  // undefined: untouched (the saved trip or a stop page's ?from= preset applies); null: cleared.
  const [from, setFrom] = useState<Pick | null>();
  const [to, setTo] = useState<Pick | null>();
  const [label, setLabel] = useState<string>();
  const [place, setPlace] = useState<number | null>();
  // How to ride: "" direct, a via string ("a1,a2>a3") for a way with changes, or CUSTOM. undefined: the default.
  const [route, setRoute] = useState<string>();
  const [changeStops, setChangeStops] = useState<Pick[]>([]);
  // A stop page can open this with ?from=<area>: look its name up once.
  const preset = useQuery({
    queryKey: ["stop", search.from],
    queryFn: ({ signal }) => fetchStop(search.from!, signal),
    enabled: !!search.from && !editId,
  });
  const initialFrom = existing?.from ?? (preset.data ? { id: preset.data.id, name: preset.data.name } : undefined);
  const fromValue = from === undefined ? initialFrom : (from ?? undefined);
  const toValue = to === undefined ? existing?.to : (to ?? undefined);
  const labelValue = label ?? existing?.label ?? "";
  const placeValue = place === undefined ? (existing?.origin_place_id ?? null) : place;

  const lines = useQuery({
    queryKey: ["trip-lines", fromValue?.id, toValue?.id],
    queryFn: ({ signal }) => fetchTripLines(fromValue!.id, toValue!.id, signal),
    enabled: !!fromValue && !!toValue,
  });
  const customVia = viaOf(changeStops.map((p) => ({ alight: p.id, board: p.id })));
  const custom = useQuery({
    queryKey: ["trip-lines", fromValue?.id, toValue?.id, customVia],
    queryFn: ({ signal }) => fetchTripLines(fromValue!.id, toValue!.id, signal, customVia),
    enabled: !!fromValue && !!toValue && route === CUSTOM && changeStops.length > 0,
    retry: false,
  });
  const direct = lines.data?.lines ?? [];
  const options = lines.data?.options ?? [];
  // A saved trip's changes stay on offer even if the suggestions no longer list them.
  const savedVia =
    existing && fromValue?.id === existing.from?.id && toValue?.id === existing.to?.id
      ? viaOf((existing.transfers ?? []).map((t) => ({ alight: t.alight?.id ?? "", board: t.board?.id ?? "" })))
      : undefined;
  const choices = [
    ...(savedVia && !options.some((o) => o.via === savedVia) && existing?.legs ? [{ via: savedVia, legs: existing.legs, transfers: existing.transfers ?? [], minutes: 0 }] : []),
    ...options,
  ];
  const routeValue = route ?? savedVia ?? (direct.length > 0 ? "" : (choices[0]?.via ?? ""));
  const chosen =
    routeValue === CUSTOM
      ? custom.data?.legs && { legs: custom.data.legs, transfers: custom.data.transfers ?? [] }
      : routeValue === ""
        ? undefined
        : choices.find((c) => c.via === routeValue);
  const canSave = routeValue === "" ? direct.length > 0 : !!chosen;
  const routeOptions = [...(direct.length > 0 ? [""] : []), ...choices.map((c) => c.via), CUSTOM];

  const save = useMutation({
    mutationFn: () => {
      const via = routeValue === CUSTOM ? customVia : routeValue;
      const body = {
        from_area_id: fromValue!.id,
        to_area_id: toValue!.id,
        transfers: transfersOf(via),
        label: labelValue.trim(),
        origin_place_id: placeValue,
      };
      return editId ? myLines.update(editId, body) : myLines.createTrip(body);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["dashboard"] });
      void qc.invalidateQueries({ queryKey: ["my-lines"] });
      void navigate({ to: "/" });
    },
  });

  if (session.data && !session.data.user) {
    return (
      <div className="mx-auto flex max-w-xl flex-col gap-4 px-4 py-8">
        <h1 className="text-2xl font-semibold tracking-tight">{t("trip.title")}</h1>
        <p className="text-muted-foreground">{t("auth.loginRequired")}</p>
        <Button asChild className="self-start">
          <Link to="/login">{t("auth.login")}</Link>
        </Button>
      </div>
    );
  }
  if (editId && saved.isSuccess && !existing) {
    return (
      <div className="mx-auto flex max-w-xl flex-col gap-4 px-4 py-8">
        <h1 className="text-2xl font-semibold tracking-tight">{t("trip.editTitle")}</h1>
        <p className="text-muted-foreground">{t("trip.notFound")}</p>
      </div>
    );
  }

  const placeOptions = placeList.data?.places ?? [];
  return (
    <div className="mx-auto flex max-w-xl flex-col gap-5 px-4 py-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{editId ? t("trip.editTitle") : t("trip.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("trip.intro")}</p>
      </div>
      <StopPicker label={t("trip.from")} value={fromValue} onChange={(p) => setFrom(p ?? null)} />
      <Button
        variant="outline"
        size="sm"
        className="self-center"
        disabled={!fromValue && !toValue}
        onClick={() => {
          const f = fromValue;
          setFrom(toValue ?? null);
          setTo(f ?? null);
        }}
      >
        <ArrowUpDown className="size-4" aria-hidden />
        {t("trip.swap")}
      </Button>
      <StopPicker label={t("trip.to")} value={toValue} onChange={(p) => setTo(p ?? null)} />

      {fromValue && toValue && (
        <section aria-labelledby="trip-how" className="flex flex-col gap-3">
          <h2 id="trip-how" className="text-sm font-medium">
            {t("trip.how")}
          </h2>
          {lines.isPending ? (
            <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
          ) : lines.isError ? (
            <div className="flex items-center justify-between gap-3 rounded-lg border bg-card p-3 text-sm text-muted-foreground">
              {t("common.error")}
              <Button variant="outline" size="sm" onClick={() => void lines.refetch()}>
                {t("common.retry")}
              </Button>
            </div>
          ) : (
            <>
              {direct.length === 0 && <p className="text-sm text-muted-foreground">{t("trip.noneTryChange")}</p>}
              <div role="radiogroup" aria-labelledby="trip-how" className="flex flex-col gap-2">
                {routeOptions.map((value, i) => {
                  const checked = routeValue === value;
                  const choice = choices.find((c) => c.via === value);
                  return (
                    <button
                      key={value || "direct"}
                      type="button"
                      role="radio"
                      aria-checked={checked}
                      tabIndex={radioTabIndex(checked, i, routeOptions.includes(routeValue))}
                      onKeyDown={radioKeyDown}
                      onClick={() => setRoute(value)}
                      className={cn(
                        "flex flex-col gap-1.5 rounded-xl border px-3 py-2.5 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                        checked ? "border-foreground bg-card" : "hover:bg-muted",
                      )}
                    >
                      {value === "" ? (
                        <>
                          <span className="flex items-center justify-between gap-2 text-sm font-medium">
                            {t("trip.direct")}
                            <span className="font-normal text-muted-foreground">{t("trip.noChange")}</span>
                          </span>
                          <LegPills lines={direct} />
                        </>
                      ) : value === CUSTOM ? (
                        <span className="flex items-center gap-1.5 text-sm font-medium">
                          <Plus className="size-4" aria-hidden />
                          {t("trip.custom")}
                        </span>
                      ) : (
                        choice && (
                          <>
                            <span className="flex items-center justify-between gap-2 text-sm font-medium">
                              {t("trip.changes", { count: choice.transfers.length })}
                              {choice.minutes > 0 && <span className="font-normal text-muted-foreground">{t("trip.aboutMin", { min: choice.minutes })}</span>}
                            </span>
                            <RouteSummary legs={choice.legs} transfers={choice.transfers} />
                          </>
                        )
                      )}
                    </button>
                  );
                })}
              </div>

              {routeValue === CUSTOM && (
                <div className="flex flex-col gap-3 rounded-xl border p-3">
                  <p className="text-sm text-muted-foreground">{t("trip.customHint")}</p>
                  {changeStops.map((stop, i) => (
                    <StopPicker
                      key={i}
                      label={t("trip.changeStop", { n: i + 1 })}
                      value={stop}
                      onChange={(p) => setChangeStops((cur) => (p ? cur.map((x, k) => (k === i ? p : x)) : cur.filter((_, k) => k !== i)))}
                    />
                  ))}
                  {changeStops.length < 2 && (
                    <StopPicker
                      key={`new-${changeStops.length}`}
                      label={t("trip.changeStop", { n: changeStops.length + 1 })}
                      onChange={(p) => p && setChangeStops((cur) => [...cur, p])}
                    />
                  )}
                  {custom.isError && (
                    <p role="alert" className="text-sm text-red-600 dark:text-red-400">
                      {custom.error instanceof ApiError ? custom.error.message : t("common.error")}
                    </p>
                  )}
                </div>
              )}

              {routeValue === "" && direct.length > 0 && (
                <LegList legs={[{ from: lines.data?.from ?? null, to: lines.data?.to ?? null, lines: direct }]} transfers={[]} />
              )}
              {chosen && <LegList legs={chosen.legs} transfers={chosen.transfers} />}
            </>
          )}
        </section>
      )}

      {fromValue && toValue && canSave && (
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <Field label={t("save.label")} error={save.error instanceof ApiError ? save.error.message : undefined}>
            <Input value={labelValue} maxLength={40} onChange={(e) => setLabel(e.target.value)} placeholder={`${fromValue.name} → ${toValue.name}`} />
          </Field>
          <div className="flex flex-col gap-2">
            <span className="text-sm font-medium" id="trip-start">
              {t("trip.startPlace")}
            </span>
            <p className="text-sm text-muted-foreground">{t("trip.startPlaceHint")}</p>
            <div role="radiogroup" aria-labelledby="trip-start" className="flex flex-wrap gap-2">
              {[{ id: null, label: t("trip.noStartPlace") }, ...placeOptions].map((p, i, all) => (
                <button
                  key={p.id ?? "none"}
                  type="button"
                  role="radio"
                  aria-checked={placeValue === p.id}
                  tabIndex={radioTabIndex(placeValue === p.id, i, all.some((x) => x.id === placeValue))}
                  onKeyDown={radioKeyDown}
                  onClick={() => setPlace(p.id)}
                  className={cn(
                    "inline-flex h-8 items-center gap-1.5 rounded-full border px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                    placeValue === p.id ? "border-foreground bg-card font-medium" : "text-muted-foreground",
                  )}
                >
                  {p.id !== null && <MapPin className="size-4" aria-hidden />}
                  {p.label}
                </button>
              ))}
              <Link to="/settings" hash="places" className="inline-flex h-8 items-center px-2 text-sm underline underline-offset-2">
                {t("trip.addPlace")}
              </Link>
            </div>
          </div>
          <Button type="submit" disabled={save.isPending}>
            {editId ? t("trip.update") : t("trip.save")}
          </Button>
        </form>
      )}
    </div>
  );
}

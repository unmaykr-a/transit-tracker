import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LocateFixed, Map as MapIcon, MapPin, Trash2 } from "lucide-react";
import { lazy, Suspense, useDeferredValue, useState } from "react";
import { useTranslation } from "react-i18next";

import { ApiError, fetchAddresses, fetchSearch, places, prefs } from "@/api";
import { ConfirmButton } from "@/components/ConfirmButton";
import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/input";

const PointPicker = lazy(() => import("@/map/PointPicker"));

const SPEEDS = [3.6, 4.2, 4.8, 5.4, 6.0];
const BUFFERS = [0, 1, 2, 3, 5];

export function WalkingSection() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const p = useQuery({ queryKey: ["prefs"], queryFn: ({ signal }) => prefs.get(signal) });
  const list = useQuery({ queryKey: ["places"], queryFn: ({ signal }) => places.list(signal) });
  const updatePrefs = useMutation({
    mutationFn: prefs.update,
    onSuccess: (v) => {
      qc.setQueryData(["prefs"], v);
      void qc.invalidateQueries({ queryKey: ["dashboard"] });
    },
  });
  const [label, setLabel] = useState("");
  const [stopQ, setStopQ] = useState("");
  const q = useDeferredValue(stopQ.trim());
  const stops = useQuery({ queryKey: ["search", q], queryFn: ({ signal }) => fetchSearch(q, signal), enabled: q.length > 1 });
  const addresses = useQuery({ queryKey: ["geocode", q], queryFn: ({ signal }) => fetchAddresses(q, signal), enabled: q.length > 2, retry: false });
  const done = () => {
    setLabel("");
    setStopQ("");
    void qc.invalidateQueries({ queryKey: ["places"] });
  };
  const [picking, setPicking] = useState(false);
  const add = useMutation({
    mutationFn: places.create,
    onSuccess: () => {
      setPicking(false);
      done();
    },
  });
  const remove = useMutation({ mutationFn: places.remove, onSuccess: done });
  const [locErr, setLocErr] = useState(false);

  const addHere = () => {
    setLocErr(false);
    navigator.geolocation?.getCurrentPosition(
      (pos) => add.mutate({ label: label.trim(), lat: pos.coords.latitude, lon: pos.coords.longitude }),
      () => setLocErr(true),
      { enableHighAccuracy: true, timeout: 15_000 },
    );
  };

  return (
    <div className="flex flex-col gap-4">
      {p.data && (
        <div className="grid grid-cols-2 gap-3">
          <Field label={t("walking.speed")}>
            <Select value={p.data.walk_speed_kmh} onChange={(e) => updatePrefs.mutate({ walk_speed_kmh: Number(e.target.value) })}>
              {[...new Set([...SPEEDS, p.data.walk_speed_kmh])].sort((a, b) => a - b).map((v) => (
                <option key={v} value={v}>
                  {t("walking.speedValue", { value: v.toFixed(1) })}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t("walking.buffer")}>
            <Select value={p.data.buffer_min} onChange={(e) => updatePrefs.mutate({ buffer_min: Number(e.target.value) })}>
              {[...new Set([...BUFFERS, p.data.buffer_min])].sort((a, b) => a - b).map((v) => (
                <option key={v} value={v}>
                  {t("walking.bufferValue", { value: v })}
                </option>
              ))}
            </Select>
          </Field>
        </div>
      )}

      <div id="places" className="flex scroll-mt-4 flex-col gap-2">
        <h3 className="text-sm font-medium">{t("walking.places")}</h3>
        <p className="text-sm text-muted-foreground">{t("walking.placesHint")}</p>
        <ul className="divide-y rounded-xl border bg-card">
          {list.data?.places.length === 0 && <li className="px-4 py-3 text-sm text-muted-foreground">{t("walking.noPlaces")}</li>}
          {list.data?.places.map((pl) => (
            <li key={pl.id} className="flex items-center justify-between gap-3 px-4 py-2 text-sm">
              <span className="flex items-center gap-2">
                <MapPin className="size-4 text-muted-foreground" aria-hidden />
                {pl.label}
              </span>
              <ConfirmButton
                variant="ghost"
                size="icon"
                onConfirm={() => remove.mutate(pl.id)}
                question={t("walking.removePlace")}
                aria-label={`${t("home.remove")} ${pl.label}`}
              >
                <Trash2 className="size-4" />
              </ConfirmButton>
            </li>
          ))}
        </ul>
        <div className="flex flex-col gap-3 rounded-xl border bg-card p-4">
          <Field label={t("walking.placeLabel")}>
            <Input value={label} onChange={(e) => setLabel(e.target.value)} placeholder={t("walking.placeLabelPlaceholder")} maxLength={40} />
          </Field>
          <Button variant="outline" disabled={!label.trim() || add.isPending} onClick={addHere} className="self-start">
            <LocateFixed className="size-4" aria-hidden />
            {t("walking.hereNow")}
          </Button>
          {locErr && <p className="text-sm text-red-600 dark:text-red-400">{t("home.locationDenied")}</p>}
          {picking ? (
            <Suspense fallback={<p className="text-sm text-muted-foreground">{t("common.loading")}</p>}>
              <PointPicker
                disabled={!label.trim() || add.isPending}
                onPick={(lat, lon) => add.mutate({ label: label.trim(), lat, lon })}
                onCancel={() => setPicking(false)}
              />
            </Suspense>
          ) : (
            <Button variant="outline" onClick={() => setPicking(true)} className="self-start">
              <MapIcon className="size-4" aria-hidden />
              {t("walking.pickOnMap")}
            </Button>
          )}
          <Field label={t("walking.orAddress")}>
            <Input type="search" value={stopQ} onChange={(e) => setStopQ(e.target.value)} placeholder={t("walking.addressPlaceholder")} />
          </Field>
          {q.length > 1 && (
            <ul className="flex flex-col" aria-label={t("walking.addressResults")}>
              {[
                ...(addresses.data?.addresses ?? []).slice(0, 5).map((a) => ({ key: `a${a.lat},${a.lon}`, name: a.label, sub: a.detail, lat: a.lat, lon: a.lon })),
                ...(stops.data?.stops ?? []).slice(0, 3).map((s) => ({ key: s.id, name: s.name, sub: t("walking.stop"), lat: s.lat, lon: s.lon })),
              ].map((r) => (
                <li key={r.key}>
                  <button
                    type="button"
                    disabled={!label.trim() || add.isPending}
                    onClick={() => add.mutate({ label: label.trim(), lat: r.lat, lon: r.lon })}
                    className="flex w-full items-baseline justify-between gap-3 rounded px-2 py-1.5 text-left text-sm hover:bg-muted disabled:opacity-50"
                  >
                    <span className="truncate font-medium">{r.name}</span>
                    <span className="shrink-0 text-xs text-muted-foreground">{r.sub}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          {q.length > 1 && !label.trim() && <p className="text-xs text-muted-foreground">{t("walking.nameFirst")}</p>}
          {add.error && (
            <p role="alert" className="text-sm text-red-600 dark:text-red-400">
              {add.error instanceof ApiError ? add.error.message : t("common.error")}
            </p>
          )}
        </div>
      </div>
    </div>
  );
}

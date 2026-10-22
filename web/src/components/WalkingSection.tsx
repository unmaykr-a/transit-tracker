import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LocateFixed, MapPin, Trash2 } from "lucide-react";
import { useDeferredValue, useState } from "react";
import { useTranslation } from "react-i18next";

import { ApiError, fetchSearch, places, prefs } from "@/api";
import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/input";

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
  const done = () => {
    setLabel("");
    setStopQ("");
    void qc.invalidateQueries({ queryKey: ["places"] });
  };
  const add = useMutation({ mutationFn: places.create, onSuccess: done });
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
    <section className="flex flex-col gap-4">
      <h2 className="font-medium">{t("walking.title")}</h2>
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

      <div className="flex flex-col gap-2">
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
              <Button variant="ghost" size="icon" onClick={() => remove.mutate(pl.id)} aria-label={`${t("home.remove")} ${pl.label}`}>
                <Trash2 className="size-4" />
              </Button>
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
          <Field label={t("walking.orStop")}>
            <Input type="search" value={stopQ} onChange={(e) => setStopQ(e.target.value)} placeholder={t("search.placeholder")} />
          </Field>
          {stops.data && q.length > 1 && (
            <ul className="flex flex-col">
              {stops.data.stops.slice(0, 5).map((s) => (
                <li key={s.id}>
                  <button
                    type="button"
                    disabled={!label.trim()}
                    onClick={() => add.mutate({ label: label.trim(), lat: s.lat, lon: s.lon })}
                    className="w-full rounded px-2 py-1.5 text-left text-sm hover:bg-muted disabled:opacity-50"
                  >
                    {s.name}
                  </button>
                </li>
              ))}
            </ul>
          )}
          {add.error && (
            <p role="alert" className="text-sm text-red-600 dark:text-red-400">
              {add.error instanceof ApiError ? add.error.message : t("common.error")}
            </p>
          )}
        </div>
      </div>
    </section>
  );
}

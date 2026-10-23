import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LocateFixed, Map as MapIcon } from "lucide-react";
import { lazy, Suspense, useDeferredValue, useState } from "react";
import { useTranslation } from "react-i18next";

import { ApiError, fetchAddresses, fetchSearch, places, type Place } from "@/api";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";

const PointPicker = lazy(() => import("@/map/PointPicker"));

/** Name a place, then add it from here, a point on the map, or an address or stop (Settings and Home). */
export function AddPlaceForm({ onAdded, autoFocus }: { onAdded?: (p: Place) => void; autoFocus?: boolean }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [label, setLabel] = useState("");
  const [stopQ, setStopQ] = useState("");
  const q = useDeferredValue(stopQ.trim());
  const stops = useQuery({ queryKey: ["search", q], queryFn: ({ signal }) => fetchSearch(q, signal), enabled: q.length > 1 });
  const addresses = useQuery({ queryKey: ["geocode", q], queryFn: ({ signal }) => fetchAddresses(q, signal), enabled: q.length > 2, retry: false });
  const [picking, setPicking] = useState(false);
  const add = useMutation({
    mutationFn: places.create,
    onSuccess: (r) => {
      setPicking(false);
      setLabel("");
      setStopQ("");
      void qc.invalidateQueries({ queryKey: ["places"] });
      onAdded?.(r.place);
    },
  });
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
    <div className="flex flex-col gap-3">
      <Field label={t("walking.placeLabel")}>
        <Input value={label} onChange={(e) => setLabel(e.target.value)} placeholder={t("walking.placeLabelPlaceholder")} maxLength={40} autoFocus={autoFocus} />
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
  );
}

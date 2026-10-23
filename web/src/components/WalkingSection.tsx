import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MapPin, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";

import { places, prefs } from "@/api";
import { AddPlaceForm } from "@/components/AddPlaceForm";
import { ConfirmButton } from "@/components/ConfirmButton";
import { Field, Select } from "@/components/ui/input";

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
  const remove = useMutation({ mutationFn: places.remove, onSuccess: () => qc.invalidateQueries({ queryKey: ["places"] }) });

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
        <div className="rounded-xl border bg-card p-4">
          <AddPlaceForm />
        </div>
      </div>
    </div>
  );
}

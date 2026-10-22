import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { X } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { ApiError, myLines, type Pattern, type Route } from "@/api";
import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/input";
import { useGeolocation } from "@/lib/geolocation";
import { approxDistanceM } from "@/lib/tween";
import { cn } from "@/lib/utils";

/** Pick boarding stop → (optional) alighting stop → label. The direction comes from the line page. */
export function SaveLineSheet({ route, pattern, onClose }: { route: Route; pattern: Pattern; onClose: () => void }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const geo = useGeolocation();
  const boardable = pattern.stops.slice(0, -1); // nobody boards at the terminus
  const nearest = useMemo(() => {
    if (geo.state.status !== "on") return -1;
    const { lat, lon } = geo.state;
    let best = -1;
    let bestD = Infinity;
    boardable.forEach((s, i) => {
      const d = approxDistanceM(lat, lon, s.lat, s.lon);
      if (d < bestD) [best, bestD] = [i, d];
    });
    return best;
  }, [geo.state, boardable]);
  const [board, setBoard] = useState<number>(-1);
  const boardIdx = board >= 0 ? board : nearest;
  const [alight, setAlight] = useState("");
  const [label, setLabel] = useState("");
  const save = useMutation({
    mutationFn: () =>
      myLines.create({
        route_id: route.id,
        direction: pattern.direction,
        board_stop_id: boardable[boardIdx]!.id,
        alight_stop_id: alight || undefined,
        label: label || undefined,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["dashboard"] });
      void navigate({ to: "/" });
    },
  });

  return (
    <section
      aria-label={t("save.title", { line: route.short_name, headsign: pattern.headsign })}
      className="flex flex-col gap-4 rounded-2xl border bg-card p-4 shadow-lg"
    >
      <div className="flex items-start justify-between gap-3">
        <h2 className="font-semibold">{t("save.title", { line: route.short_name, headsign: pattern.headsign })}</h2>
        <Button variant="ghost" size="icon" onClick={onClose} aria-label={t("vehicle.close")}>
          <X className="size-5" />
        </Button>
      </div>
      <fieldset className="flex flex-col gap-2">
        <legend className="mb-2 text-sm font-medium">{t("save.board")}</legend>
        <div role="radiogroup" className="flex max-h-64 flex-col overflow-y-auto rounded-xl border">
          {boardable.map((s, i) => (
            <button
              key={`${s.id}-${i}`}
              type="button"
              role="radio"
              aria-checked={i === boardIdx}
              onClick={() => {
                setBoard(i);
                setAlight("");
              }}
              className={cn(
                "flex items-center justify-between gap-2 border-b px-3 py-2 text-left text-sm last:border-b-0",
                i === boardIdx ? "bg-muted font-medium" : "hover:bg-muted/60",
              )}
            >
              <span>
                {s.name}
                {s.platform && <span className="text-muted-foreground"> · {s.platform}</span>}
              </span>
              {i === nearest && <span className="text-xs text-muted-foreground">{t("save.nearest")}</span>}
            </button>
          ))}
        </div>
      </fieldset>
      {boardIdx >= 0 && (
        <>
          <Field label={t("save.alight")}>
            <Select
              value={alight}
              onChange={(e) => {
                // Pin the GPS-picked boarding stop, so a later fix cannot move it past the alighting stop.
                if (board < 0) setBoard(boardIdx);
                setAlight(e.target.value);
              }}
            >
              <option value="">{t("save.none")}</option>
              {pattern.stops.slice(boardIdx + 1).map((s, i) => (
                <option key={`${s.id}-${i}`} value={s.id}>
                  {s.name}
                  {s.platform ? ` · ${s.platform}` : ""}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t("save.label")}>
            <Input value={label} onChange={(e) => setLabel(e.target.value)} placeholder={t("save.labelPlaceholder")} maxLength={40} />
          </Field>
        </>
      )}
      {save.error && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {save.error instanceof ApiError ? save.error.message : t("common.error")}
        </p>
      )}
      <Button disabled={boardIdx < 0 || save.isPending} onClick={() => save.mutate()}>
        {t("save.submit")}
      </Button>
    </section>
  );
}

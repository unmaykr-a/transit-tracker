import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { lazy, Suspense, useState } from "react";
import { useTranslation } from "react-i18next";

import { fetchAllStops, places, type MapStop } from "@/api";
import { VehicleSheet } from "@/components/VehicleSheet";
import { useSession } from "@/lib/session";
import type { Mode } from "@/lib/types";

import type { Selection } from "./VehicleMap";

const VehicleMap = lazy(() => import("./VehicleMap"));
const NO_STOPS: MapStop[] = [];

interface Props {
  modes: ReadonlySet<Mode>;
  lineTokens: string[];
  fit?: [number, number][];
  highlightPattern?: string;
  highlightMode?: Mode;
  /** A platform to mark as selected (the stop page's). */
  stopId?: string;
}

/** The live map as a page pane: animated vehicles, tap for details, saved places. */
export default function LiveMapPane({ modes, lineTokens, fit, highlightPattern, highlightMode, stopId }: Props) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [vehicle, setVehicle] = useState<string>();
  const stops = useQuery({ queryKey: ["all-stops"], queryFn: ({ signal }) => fetchAllStops(signal), staleTime: 6 * 60 * 60_000 });
  const signedIn = !!useSession().data?.user;
  const myPlaces = useQuery({ queryKey: ["places"], queryFn: ({ signal }) => places.list(signal), enabled: signedIn });
  const selected: Selection | undefined = vehicle ? { kind: "vehicle", id: vehicle } : stopId ? { kind: "stop", id: stopId } : undefined;
  const onSelect = (sel: Selection | undefined) => {
    if (sel?.kind === "stop") {
      const s = stops.data?.stops.find((x) => x.id === sel.id);
      if (s) void navigate({ to: "/stops/$id", params: { id: s.area_id }, search: { p: s.id } });
      return;
    }
    setVehicle(sel?.id);
  };
  return (
    <div className="absolute inset-0">
      <Suspense fallback={<p className="p-4 text-muted-foreground">{t("map.loading")}</p>}>
        <VehicleMap
          modes={modes}
          lineTokens={lineTokens}
          stops={stops.data?.stops ?? NO_STOPS}
          selected={selected}
          onSelect={onSelect}
          fit={fit}
          highlightPattern={highlightPattern}
          highlightMode={highlightMode}
          places={myPlaces.data?.places}
        />
      </Suspense>
      {vehicle && (
        <div className="pointer-events-none absolute bottom-4 left-4 z-10 w-96">
          <VehicleSheet id={vehicle} onClose={() => setVehicle(undefined)} />
        </div>
      )}
    </div>
  );
}

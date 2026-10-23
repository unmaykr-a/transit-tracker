import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useMemo, useState } from "react";

import { fetchAllStops, places, type MapStop } from "@/api";
import { VehicleSheet } from "@/components/VehicleSheet";
import { useSession } from "@/lib/session";
import type { Kind, Mode } from "@/lib/types";

import { useMapPane } from "./mapPane";
import type { Selection } from "./VehicleMap";

const NO_STOPS: MapStop[] = [];

interface Props {
  modes: ReadonlySet<Kind>;
  lineTokens: string[];
  fit?: [number, number][];
  highlightPattern?: string;
  highlightMode?: Mode;
  /** A platform to mark as selected (the stop page's). */
  stopId?: string;
}

/** The shared live map as a page pane (beside a wide line or stop page): what it shows, and the vehicle sheet over it. */
export default function LiveMapPane({ modes, lineTokens, fit, highlightPattern, highlightMode, stopId }: Props) {
  const navigate = useNavigate();
  const [vehicle, setVehicle] = useState<string>();
  const [following, setFollowing] = useState(false);
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
    setFollowing(false);
  };
  const selectedKey = selected ? `${selected.kind}:${selected.id}` : "";
  // eslint-disable-next-line react-hooks/exhaustive-deps -- selectedKey stands for selected
  const sel = useMemo(() => selected, [selectedKey]);
  useMapPane("side", {
    modes,
    lineTokens,
    stops: stops.data?.stops ?? NO_STOPS,
    selected: sel,
    onSelect,
    fit,
    highlightPattern,
    highlightMode,
    places: myPlaces.data?.places,
    follow: following ? vehicle : undefined,
    onFollowEnd: () => setFollowing(false),
  });
  return (
    <div className="absolute inset-0">
      {vehicle && (
        <div className="pointer-events-none absolute bottom-4 left-4 z-10 w-96">
          <VehicleSheet
            id={vehicle}
            onClose={() => {
              setVehicle(undefined);
              setFollowing(false);
            }}
            following={following}
            onFollow={setFollowing}
          />
        </div>
      )}
    </div>
  );
}

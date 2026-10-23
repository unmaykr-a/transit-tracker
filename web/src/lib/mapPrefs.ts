import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSyncExternalStore } from "react";

import { prefs, type Prefs } from "@/api";

import { useSession } from "./session";
import { load, save } from "./storage";

/**
 * Map display preferences. Visitors keep them in this browser; signed-in users
 * in their account (user_prefs), so they follow them across devices.
 */
type MapPrefKey = "show_rail" | "vehicle_icons" | "predict_positions";
export type VehicleIcons = "pill" | "topdown";

interface MapPrefValues {
  show_rail: boolean;
  vehicle_icons: VehicleIcons;
  predict_positions: boolean;
}

const DEFAULTS: MapPrefValues = { show_rail: false, vehicle_icons: "pill", predict_positions: true };
const STORAGE: Record<MapPrefKey, string> = {
  show_rail: "bus.map.rail",
  vehicle_icons: "bus.map.icons",
  predict_positions: "bus.map.predict",
};

const listeners = new Set<() => void>();
const local: MapPrefValues = {
  show_rail: load(STORAGE.show_rail, DEFAULTS.show_rail),
  vehicle_icons: load(STORAGE.vehicle_icons, DEFAULTS.vehicle_icons),
  predict_positions: load(STORAGE.predict_positions, DEFAULTS.predict_positions),
};
let localSnapshot = { ...local };

function subscribe(fn: () => void) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

function useMapPref<K extends MapPrefKey>(key: K): [MapPrefValues[K], (v: MapPrefValues[K]) => void] {
  const qc = useQueryClient();
  const signedIn = !!useSession().data?.user;
  const localValues = useSyncExternalStore(subscribe, () => localSnapshot);
  const p = useQuery({ queryKey: ["prefs"], queryFn: ({ signal }) => prefs.get(signal), enabled: signedIn, staleTime: 5 * 60_000 });
  const update = useMutation({
    mutationFn: (v: MapPrefValues[K]) => prefs.update({ [key]: v } as Partial<Prefs>),
    onMutate: (v) => qc.setQueryData<Prefs>(["prefs"], (old) => (old ? { ...old, [key]: v } : old)),
    onSettled: () => qc.invalidateQueries({ queryKey: ["prefs"] }),
  });
  if (!signedIn) {
    return [
      localValues[key],
      (v) => {
        local[key] = v;
        localSnapshot = { ...local };
        save(STORAGE[key], v);
        listeners.forEach((fn) => fn());
      },
    ];
  }
  const value = (p.data?.[key] as MapPrefValues[K] | undefined) ?? DEFAULTS[key];
  return [value, (v) => update.mutate(v)];
}

/** Whether railway lines are drawn on the live map (off by default). */
export const useShowRail = () => useMapPref("show_rail");
/** How vehicles are drawn: line-number pills (default) or vehicles seen from above. */
export const useVehicleIcons = () => useMapPref("vehicle_icons");
/** Whether markers move on along their route between updates (on by default). */
export const usePredictPositions = () => useMapPref("predict_positions");

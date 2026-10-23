import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSyncExternalStore } from "react";

import { prefs, type Prefs } from "@/api";

import { useSession } from "./session";
import { load, save } from "./storage";

// Visitors keep the choice in this browser; signed-in users in their account (user_prefs.show_rail).
const KEY = "bus.map.rail";
const listeners = new Set<() => void>();
let local = load<boolean>(KEY, false);

function subscribe(fn: () => void) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

/** Whether railway lines are drawn on the live map (off by default), and a setter. */
export function useShowRail(): [boolean, (v: boolean) => void] {
  const qc = useQueryClient();
  const signedIn = !!useSession().data?.user;
  const localValue = useSyncExternalStore(subscribe, () => local);
  const p = useQuery({ queryKey: ["prefs"], queryFn: ({ signal }) => prefs.get(signal), enabled: signedIn, staleTime: 5 * 60_000 });
  const update = useMutation({
    mutationFn: (v: boolean) => prefs.update({ show_rail: v }),
    onMutate: (v) => qc.setQueryData<Prefs>(["prefs"], (old) => (old ? { ...old, show_rail: v } : old)),
    onSettled: () => qc.invalidateQueries({ queryKey: ["prefs"] }),
  });
  if (!signedIn) {
    return [
      localValue,
      (v) => {
        local = v;
        save(KEY, v);
        listeners.forEach((fn) => fn());
      },
    ];
  }
  return [p.data?.show_rail ?? false, (v) => update.mutate(v)];
}

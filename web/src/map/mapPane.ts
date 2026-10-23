import { useEffect, useSyncExternalStore } from "react";

import type { VehicleMapProps } from "./VehicleMap";

/**
 * One live map for the whole app (MapHost in the app shell), so moving between
 * the map, a line and a stop moves its camera instead of building a new map.
 * A page asks for it with useMapPane: "full" fills the page (the map page),
 * "side" is the pane beside a wide line or stop page. Pages draw their own
 * overlays (filters, sheets) above it.
 */
export type PaneMode = "full" | "side";

export interface PaneState {
  mode: PaneMode | null;
  props: VehicleMapProps | null;
  /** Bumped by every page that shows the map, so a page's leaving clears only its own request. */
  owner: number;
  /** The map stays loaded (hidden) a while after the last page that showed it: going back is instant. */
  kept: boolean;
}

const KEEP_MS = 60_000;

let state: PaneState = { mode: null, props: null, owner: 0, kept: false };
const listeners = new Set<() => void>();
let nextOwner = 1;
let release: ReturnType<typeof setTimeout> | undefined;

function set(next: PaneState) {
  state = next;
  listeners.forEach((fn) => fn());
}

function subscribe(fn: () => void) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

export function useMapPaneState(): PaneState {
  return useSyncExternalStore(subscribe, () => state);
}

/** Only whether (and how) the map shows: re-renders on that alone, not on every props change. */
export function useMapPaneMode(): PaneMode | null {
  return useSyncExternalStore(subscribe, () => state.mode);
}

/** Shows the shared map in this page with these props while the page is mounted. */
export function useMapPane(mode: PaneMode, props: VehicleMapProps) {
  useEffect(() => {
    const owner = nextOwner++;
    clearTimeout(release);
    set({ ...state, mode, owner, kept: true });
    return () => {
      if (state.owner !== owner) return;
      set({ ...state, mode: null, owner: 0 });
      // Pages swap within one commit (this cleanup, then the next page's effect), so the map survives that.
      release = setTimeout(() => set({ ...state, kept: false }), KEEP_MS);
    };
  }, [mode]);
  useEffect(() => {
    set({ ...state, props });
  });
}

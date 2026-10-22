import { useCallback, useEffect, useState, useSyncExternalStore } from "react";

import { live, type ConnectionStatus } from "./liveClient";
import type { LiveState } from "./liveState";

/**
 * Re-renders at most once per intervalMs with the latest live state. Map
 * animation does not go through React; this is for counts, banners and sheets.
 */
export function useLiveState(intervalMs = 1000): LiveState {
  const subscribe = useCallback(
    (notify: () => void) => {
      let timer: ReturnType<typeof setTimeout> | undefined;
      const unsub = live.subscribe(() => {
        timer ??= setTimeout(() => {
          timer = undefined;
          notify();
        }, intervalMs);
      });
      return () => {
        unsub();
        clearTimeout(timer);
      };
    },
    [intervalMs],
  );
  return useSyncExternalStore(subscribe, () => live.getState());
}

export function useConnectionStatus(): ConnectionStatus {
  return useSyncExternalStore(
    (fn) => live.onStatus(fn),
    () => live.status,
  );
}

/** The current time, refreshed every intervalMs (for "N s ago" labels). */
export function useNow(intervalMs = 1000): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

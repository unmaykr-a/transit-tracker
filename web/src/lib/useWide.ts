import { useSyncExternalStore } from "react";

const QUERY = "(min-width: 1024px)";

function subscribe(cb: () => void) {
  const m = globalThis.matchMedia?.(QUERY);
  m?.addEventListener("change", cb);
  return () => m?.removeEventListener("change", cb);
}

/** True on large screens (lg), where pages show a live map beside their details. */
export function useWide(): boolean {
  return useSyncExternalStore(subscribe, () => globalThis.matchMedia?.(QUERY).matches ?? false);
}

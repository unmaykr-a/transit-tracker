import { useSyncExternalStore } from "react";

import { load, save } from "./storage";

export type ThemePref = "system" | "light" | "dark";
export type Theme = "light" | "dark";

const KEY = "tram.theme";
const listeners = new Set<() => void>();
let pref: ThemePref = load<ThemePref>(KEY, "system");

const media = () => globalThis.matchMedia?.("(prefers-color-scheme: dark)");

function apply() {
  const root = document.documentElement;
  if (pref === "system") delete root.dataset.theme;
  else root.dataset.theme = pref;
  const dark = effectiveTheme() === "dark";
  document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]').forEach((m) => {
    if (pref !== "system") m.content = dark ? "#0a0a0a" : "#fafafa";
  });
}

export function effectiveTheme(): Theme {
  if (pref !== "system") return pref;
  return media()?.matches ? "dark" : "light";
}

export function setThemePref(p: ThemePref) {
  pref = p;
  save(KEY, p);
  apply();
  listeners.forEach((fn) => fn());
}

export function initTheme() {
  apply();
  media()?.addEventListener("change", () => listeners.forEach((fn) => fn()));
}

function subscribe(fn: () => void) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

export function useThemePref(): ThemePref {
  return useSyncExternalStore(subscribe, () => pref);
}

export function useTheme(): Theme {
  return useSyncExternalStore(subscribe, effectiveTheme);
}

export function prefersReducedMotion(): boolean {
  return globalThis.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;
}

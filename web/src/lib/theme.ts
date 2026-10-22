import { useSyncExternalStore } from "react";

import { load, save } from "./storage";

export type ThemePref = "system" | "light" | "dark";
export type Theme = "light" | "dark";

const KEY = "bus.theme";
const listeners = new Set<() => void>();
let chosen = load<ThemePref | null>(KEY, null);
let pref: ThemePref = chosen ?? "system";

const media = () => globalThis.matchMedia?.("(prefers-color-scheme: dark)");

function apply() {
  const root = document.documentElement;
  if (pref === "system") delete root.dataset.theme;
  else root.dataset.theme = pref;
  const dark = effectiveTheme() === "dark";
  // Each meta answers its own media query under "system"; a forced theme sets both.
  document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]').forEach((m) => {
    const forDark = pref === "system" ? m.media.includes("dark") : dark;
    m.content = forDark ? "#0a0a0a" : "#fafafa";
  });
}

export function effectiveTheme(): Theme {
  if (pref !== "system") return pref;
  return media()?.matches ? "dark" : "light";
}

export function setThemePref(p: ThemePref) {
  pref = chosen = p;
  save(KEY, p);
  apply();
  listeners.forEach((fn) => fn());
}

/** The instance default, for visitors who never picked a theme (not saved, so a later change follows it). */
export function applyDefaultTheme(p: ThemePref) {
  if (chosen || pref === p) return;
  pref = p;
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

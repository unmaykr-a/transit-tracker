import { useEffect, useState } from "react";

import { load, save } from "./storage";

export type GeoState =
  | { status: "off" } // not requested yet
  | { status: "locating" }
  | { status: "on"; lat: number; lon: number; accuracy: number }
  | { status: "denied" | "unavailable" };

const KEY = "bus.useLocation";

/**
 * Watches the device position while the page is visible, once the user has
 * opted in (or the browser already granted permission). The position is only
 * ever sent with dashboard requests; it is never stored server-side.
 */
export function useGeolocation() {
  const [enabled, setEnabled] = useState(() => load(KEY, false));
  const [state, setState] = useState<GeoState>({ status: "off" });

  // Start automatically when permission was granted before.
  useEffect(() => {
    if (enabled || !navigator.permissions?.query) return;
    let cancelled = false;
    navigator.permissions
      .query({ name: "geolocation" })
      .then((p) => {
        if (!cancelled && p.state === "granted") setEnabled(true);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [enabled]);

  useEffect(() => {
    if (!enabled) return;
    if (!navigator.geolocation) {
      queueMicrotask(() => setState({ status: "unavailable" }));
      return;
    }
    let watch: number | undefined;
    const start = () => {
      if (watch !== undefined || document.hidden) return;
      watch = navigator.geolocation.watchPosition(
        (p) => setState({ status: "on", lat: p.coords.latitude, lon: p.coords.longitude, accuracy: p.coords.accuracy }),
        (e) => setState({ status: e.code === e.PERMISSION_DENIED ? "denied" : "unavailable" }),
        { enableHighAccuracy: true, maximumAge: 15_000, timeout: 20_000 },
      );
    };
    const stop = () => {
      if (watch !== undefined) navigator.geolocation.clearWatch(watch);
      watch = undefined;
    };
    const onVisibility = () => (document.hidden ? stop() : start());
    queueMicrotask(() => setState((s) => (s.status === "on" ? s : { status: "locating" })));
    start();
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      stop();
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [enabled]);

  const enable = () => {
    save(KEY, true);
    setEnabled(true);
  };
  return { state, enabled, enable };
}

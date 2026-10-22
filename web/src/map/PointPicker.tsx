import type { Map as MLMap } from "maplibre-gl";
import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { useTheme } from "@/lib/theme";

import { collapseAttribution, maplibregl, STYLE_URL, TALLINN } from "./setup";

interface Props {
  onPick: (lat: number, lon: number) => void;
  onCancel: () => void;
  disabled?: boolean;
}

/** A map with a fixed pin in the middle: pan the spot under it, then confirm. */
export default function PointPicker({ onPick, onCancel, disabled }: Props) {
  const { t } = useTranslation();
  const container = useRef<HTMLDivElement>(null);
  const mapRef = useRef<MLMap | null>(null);
  const theme = useTheme();

  useEffect(() => {
    if (!container.current) return;
    const map = new maplibregl.Map({
      container: container.current,
      style: STYLE_URL[theme],
      center: TALLINN,
      zoom: 13,
      attributionControl: { compact: true },
      dragRotate: false,
      pitchWithRotate: false,
    });
    map.touchZoomRotate.disableRotation();
    map.addControl(new maplibregl.NavigationControl({ showCompass: false }), "top-right");
    // Start at the user's position when the browser already allows it; never prompt for it here.
    void navigator.permissions?.query({ name: "geolocation" }).then((p) => {
      if (p.state === "granted") {
        navigator.geolocation.getCurrentPosition((pos) => map.jumpTo({ center: [pos.coords.longitude, pos.coords.latitude], zoom: 16 }));
      }
    });
    collapseAttribution(map);
    mapRef.current = map;
    return () => {
      map.remove();
      mapRef.current = null;
    };
  }, [theme]);

  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm text-muted-foreground">{t("walking.mapHint")}</p>
      <div className="relative h-72 overflow-hidden rounded-lg border">
        <div ref={container} className="h-full w-full" role="region" aria-label={t("walking.pickOnMap")} />
        {/* The pin's tip marks the map centre. */}
        <svg
          viewBox="0 0 24 32"
          className="pointer-events-none absolute left-1/2 top-1/2 h-9 w-7 -translate-x-1/2 -translate-y-full drop-shadow"
          aria-hidden
        >
          <path d="M12 0C5.4 0 0 5.3 0 11.9 0 20.8 12 32 12 32s12-11.2 12-20.1C24 5.3 18.6 0 12 0z" fill="#d02f2a" />
          <circle cx="12" cy="12" r="4.5" fill="#fff" />
        </svg>
      </div>
      <div className="flex gap-2">
        <Button
          type="button"
          disabled={disabled}
          onClick={() => {
            const c = mapRef.current?.getCenter();
            if (c) onPick(c.lat, c.lng);
          }}
        >
          {t("walking.useThisSpot")}
        </Button>
        <Button type="button" variant="ghost" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
      </div>
    </div>
  );
}

import type { ExpressionSpecification } from "@maplibre/maplibre-gl-style-spec";
import * as maplibregl from "maplibre-gl";
import type { GeoJSONSource, Map as MLMap, MapGeoJSONFeature, MapMouseEvent } from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
// MapLibre finds its worker next to its own module, which bundling breaks; hand it Vite's build.
import workerUrl from "maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url";
import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";

import { live } from "@/lib/liveClient";
import { MODE_COLORS } from "@/lib/modeColors";
import { prefersReducedMotion, useTheme, type Theme } from "@/lib/theme";
import { isMoving, nextTween, type Tween } from "@/lib/tween";
import { MODES, type Mode } from "@/lib/types";

import { pillImage, vehicleFeatures, wedgeImage, type ImageSpec } from "./vehicleFeatures";

maplibregl.setWorkerUrl(workerUrl);

const STYLE_URL: Record<Theme, string> = {
  light: "https://tiles.openfreemap.org/styles/positron",
  dark: "https://tiles.openfreemap.org/styles/dark",
};
const TALLINN: [number, number] = [24.7536, 59.437];
const PILL_MIN_ZOOM = 12.5;
/** Redraw rate while vehicles glide; 20 fps is smooth at city zoom and cheap on phones. */
const FRAME_MS = 50;

export interface VehicleMapProps {
  modes: ReadonlySet<Mode>;
  lineTokens: string[];
  selectedId?: string;
  onSelect: (id: string | undefined) => void;
}

function addImage(map: MLMap, name: string, img: ImageSpec) {
  if (map.hasImage(name)) map.removeImage(name);
  const { width, height, data, ...opts } = img;
  map.addImage(name, { width, height, data }, opts);
}

function addVehicleLayers(map: MLMap) {
  const ratio = Math.min(3, Math.max(1, Math.round(globalThis.devicePixelRatio || 1)));
  for (const m of MODES) {
    addImage(map, `pill-${m}`, pillImage(MODE_COLORS[m], ratio));
    addImage(map, `wedge-${m}`, wedgeImage(MODE_COLORS[m], ratio));
  }
  if (!map.getSource("vehicles")) {
    map.addSource("vehicles", { type: "geojson", data: { type: "FeatureCollection", features: [] }, promoteId: "id" });
  }
  const modeColor: ExpressionSpecification = [
    "match", ["get", "mode"],
    "tram", MODE_COLORS.tram, "bus", MODE_COLORS.bus, "trolley", MODE_COLORS.trolley, MODE_COLORS.train,
  ];
  // Dots are always drawn; where pills collide (busy stops) the dot still shows the vehicle.
  map.addLayer({
    id: "vehicle-dots",
    type: "circle",
    source: "vehicles",
    layout: { "circle-sort-key": ["-", 3, ["get", "sort"]] },
    paint: {
      "circle-color": modeColor,
      "circle-radius": ["interpolate", ["linear"], ["zoom"], 9, 2.5, PILL_MIN_ZOOM, 5.5],
      "circle-stroke-color": "#ffffff",
      "circle-stroke-width": ["interpolate", ["linear"], ["zoom"], 9, 0.5, PILL_MIN_ZOOM, 1.5],
    },
  });
  map.addLayer({
    id: "vehicle-selected",
    type: "circle",
    source: "vehicles",
    filter: ["==", ["get", "id"], ""],
    paint: { "circle-color": modeColor, "circle-opacity": 0.25, "circle-radius": 26, "circle-blur": 0.3 },
  });
  map.addLayer({
    id: "vehicle-heading",
    type: "symbol",
    source: "vehicles",
    minzoom: PILL_MIN_ZOOM,
    filter: [">=", ["get", "bearing"], 0],
    layout: {
      "icon-image": ["concat", "wedge-", ["get", "mode"]],
      "icon-rotate": ["get", "bearing"],
      "icon-rotation-alignment": "map",
      "icon-allow-overlap": true,
      "icon-ignore-placement": true,
      "symbol-sort-key": ["get", "sort"],
    },
  });
  const pillLayout = {
    "icon-image": ["concat", "pill-", ["get", "mode"]],
    "icon-text-fit": "both",
    "text-field": ["get", "text"],
    "text-font": ["Noto Sans Bold"],
    "text-size": 12,
    "symbol-sort-key": ["get", "sort"],
  } satisfies maplibregl.SymbolLayerSpecification["layout"];
  map.addLayer({
    id: "vehicle-pills",
    type: "symbol",
    source: "vehicles",
    minzoom: PILL_MIN_ZOOM,
    layout: { ...pillLayout, "icon-padding": 1 },
    paint: { "text-color": "#ffffff" },
  });
  // The selected vehicle always shows its pill, above everything else.
  map.addLayer({
    id: "vehicle-pill-selected",
    type: "symbol",
    source: "vehicles",
    filter: ["==", ["get", "id"], ""],
    layout: { ...pillLayout, "icon-allow-overlap": true, "icon-ignore-placement": true, "text-allow-overlap": true, "text-ignore-placement": true },
    paint: { "text-color": "#ffffff" },
  });
}

export default function VehicleMap({ modes, lineTokens, selectedId, onSelect }: VehicleMapProps) {
  const { t } = useTranslation();
  const container = useRef<HTMLDivElement>(null);
  const mapRef = useRef<MLMap | null>(null);
  const filterRef = useRef({ modes, lineTokens });
  const dirtyRef = useRef(true);
  const onSelectRef = useRef(onSelect);
  const theme = useTheme();
  const themeRef = useRef(theme);

  useEffect(() => {
    onSelectRef.current = onSelect;
  }, [onSelect]);

  useEffect(() => {
    filterRef.current = { modes, lineTokens };
    dirtyRef.current = true;
  }, [modes, lineTokens]);

  // Create the map once.
  useEffect(() => {
    if (!container.current) return;
    const map = new maplibregl.Map({
      container: container.current,
      style: STYLE_URL[themeRef.current],
      center: TALLINN,
      zoom: 13,
      attributionControl: { compact: true },
      dragRotate: false,
      pitchWithRotate: false,
      // Positions update 20×/s while vehicles glide; fading symbols in and out on every
      // collision change would flicker.
      fadeDuration: 0,
    });
    mapRef.current = map;
    // Hook for end-to-end tests (locating a vehicle on screen) and debugging.
    (globalThis as { __tramMap?: MLMap }).__tramMap = map;
    map.touchZoomRotate.disableRotation();
    map.addControl(new maplibregl.NavigationControl({ showCompass: false }), "top-right");
    map.addControl(
      new maplibregl.GeolocateControl({
        positionOptions: { enableHighAccuracy: true },
        trackUserLocation: true,
        showAccuracyCircle: true,
      }),
      "top-right",
    );

    const tweens = new Map<string, Tween>();
    const animate = !prefersReducedMotion();
    let ready = false;

    map.on("style.load", () => {
      addVehicleLayers(map);
      ready = true;
      dirtyRef.current = true;
    });

    const unsubscribe = live.subscribe((applied) => {
      const now = performance.now();
      if (applied.reset) {
        for (const id of tweens.keys()) if (!applied.state.vehicles.has(id)) tweens.delete(id);
      }
      for (const id of applied.removed) tweens.delete(id);
      for (const v of applied.changed) tweens.set(v.id, nextTween(tweens.get(v.id), v, now, animate));
      dirtyRef.current = true;
    });

    let raf = 0;
    let last = 0;
    const frame = (now: number) => {
      raf = requestAnimationFrame(frame);
      if (!ready || now - last < FRAME_MS || document.hidden) return;
      let moving = false;
      for (const tw of tweens.values()) {
        if (isMoving(tw, now)) {
          moving = true;
          break;
        }
      }
      if (!moving && !dirtyRef.current) return;
      last = now;
      dirtyRef.current = false;
      const { modes, lineTokens } = filterRef.current;
      const source = map.getSource<GeoJSONSource>("vehicles");
      source?.setData(vehicleFeatures(live.getState().vehicles.values(), tweens, now, modes, lineTokens));
    };
    raf = requestAnimationFrame(frame);

    const layers = ["vehicle-pills", "vehicle-dots"];
    const click = (e: MapMouseEvent & { features?: MapGeoJSONFeature[] }) => {
      const id = e.features?.[0]?.properties?.id as string | undefined;
      if (id) onSelectRef.current(id);
    };
    for (const l of layers) {
      map.on("click", l, click);
      map.on("mouseenter", l, () => (map.getCanvas().style.cursor = "pointer"));
      map.on("mouseleave", l, () => (map.getCanvas().style.cursor = ""));
    }
    map.on("click", (e: MapMouseEvent) => {
      if (map.queryRenderedFeatures(e.point, { layers: layers.filter((l) => map.getLayer(l)) }).length === 0) {
        onSelectRef.current(undefined);
      }
    });

    return () => {
      cancelAnimationFrame(raf);
      unsubscribe();
      map.remove();
      mapRef.current = null;
    };
  }, []);

  // Swap the basemap when the theme changes; our layers are re-added on style.load.
  useEffect(() => {
    if (themeRef.current === theme) return;
    themeRef.current = theme;
    mapRef.current?.setStyle(STYLE_URL[theme]);
  }, [theme]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const apply = () => {
      for (const l of ["vehicle-selected", "vehicle-pill-selected"]) {
        if (map.getLayer(l)) map.setFilter(l, ["==", ["get", "id"], selectedId ?? ""]);
      }
    };
    apply();
    map.on("style.load", apply);
    return () => {
      map.off("style.load", apply);
    };
  }, [selectedId]);

  // MapLibre's stylesheet forces position: relative on the map element, so size it from a wrapper.
  return (
    <div className="absolute inset-0">
      <div ref={container} className="h-full w-full" role="region" aria-label={t("nav.map")} />
    </div>
  );
}

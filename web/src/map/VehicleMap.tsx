import type { ExpressionSpecification } from "@maplibre/maplibre-gl-style-spec";
import type { GeoJSONSource, Map as MLMap, MapGeoJSONFeature, MapMouseEvent } from "maplibre-gl";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import type { MapStop } from "@/api";
import { pillText } from "@/lib/lines";
import { live } from "@/lib/liveClient";
import { useShowRail } from "@/lib/mapPrefs";
import { loadShapes, patternRoutes, shapes } from "@/lib/shapes";
import { MODE_COLORS } from "@/lib/modeColors";
import { prefersReducedMotion, useTheme } from "@/lib/theme";
import { isMoving, nextTween, type Tween } from "@/lib/tween";
import { MODES, type Mode } from "@/lib/types";

import { collapseAttribution, maplibregl, startView, styleUrl } from "./setup";
import { pillImage, vehicleFeatures, wedgeImage, type ImageSpec } from "./vehicleFeatures";
const PILL_MIN_ZOOM = 12.5;
/** Redraw rate while vehicles glide; 20 fps is smooth at city zoom and cheap on phones. */
const FRAME_MS = 50;

export type Selection = { kind: "vehicle"; id: string } | { kind: "stop"; id: string };

export interface VehicleMapProps {
  modes: ReadonlySet<Mode>;
  lineTokens: string[];
  stops: MapStop[];
  selected?: Selection;
  onSelect: (sel: Selection | undefined) => void;
  /** When set, the map flies here once (e.g. /map?stop=…). */
  focus?: [number, number];
  /** Pattern whose route is drawn under the vehicles (the selected vehicle's), in its mode's colour. */
  highlightPattern?: string;
  highlightMode?: Mode;
  /** Points ([lon, lat]) to frame when the map opens or they change, e.g. a line's route. */
  fit?: [number, number][];
  /** The user's saved places, marked on the map. */
  places?: { id: number; label: string; lat: number; lon: number }[];
}

const shapeOf = (p: string) => shapes.get(p);
const NO_FEATURES = { type: "FeatureCollection" as const, features: [] };

const STOPS_MIN_ZOOM = 14.5;

/** Under the selected route: a halo that sets it off from the streets in either theme. */
const ROUTE_CASING = { light: "#ffffff", dark: "#0b0f19" } as const;

/** Stops along the selected route, placed on its shape (fetched once per pattern). */
const routeStops = new Map<string, Promise<number[]>>();
function routeStopsAlong(pattern: string): Promise<number[]> {
  const cached = routeStops.get(pattern);
  if (cached) return cached;
  const p = fetch(`/api/v1/patterns/${pattern}`) // ids are URL-safe; the router does not decode %3A
    .then(async (r): Promise<number[]> => {
      if (!r.ok) throw new Error(`pattern: ${r.status}`);
      return ((await r.json()) as { stop_along_m?: number[] }).stop_along_m ?? [];
    })
    .catch((): number[] => {
      routeStops.delete(pattern); // retry next time
      return [];
    });
  routeStops.set(pattern, p);
  return p;
}

/** Railway lines from the train patterns, visible from the city-wide zoom out (base maps hide them early). */
function addRailLayers(map: MLMap) {
  if (!map.getSource("rail")) map.addSource("rail", { type: "geojson", data: NO_FEATURES });
  const width: ExpressionSpecification = ["interpolate", ["linear"], ["zoom"], 7, 1.5, 12, 3, 16, 5];
  map.addLayer(
    // Hidden until the person turns railway lines on (the effect below shows them).
    { id: "rail", type: "line", source: "rail", minzoom: 6, layout: { visibility: "none" }, paint: { "line-color": MODE_COLORS.train, "line-opacity": 0.55, "line-width": width } },
    "stops",
  );
  // White ties on the line, the usual way maps draw railways.
  map.addLayer(
    {
      id: "rail-ties",
      type: "line",
      source: "rail",
      minzoom: 11,
      layout: { visibility: "none" },
      paint: { "line-color": "#ffffff", "line-opacity": 0.8, "line-width": ["interpolate", ["linear"], ["zoom"], 11, 1, 16, 2], "line-dasharray": [2, 3] },
    },
    "stops",
  );
}

function railFeatures() {
  const seen = new Set<string>();
  const features = [];
  for (const [id, shape] of shapes) {
    if (!patternRoutes.get(id)?.startsWith("train:")) continue;
    const key = `${shape.coords[0]}|${shape.coords[shape.coords.length - 1]}|${shape.coords.length}`; // both directions share track
    if (seen.has(key)) continue;
    seen.add(key);
    features.push({ type: "Feature" as const, properties: {}, geometry: { type: "LineString" as const, coordinates: shape.coords } });
  }
  return { type: "FeatureCollection" as const, features };
}

function addPlaceLayers(map: MLMap) {
  if (!map.getSource("places")) map.addSource("places", { type: "geojson", data: NO_FEATURES });
  map.addLayer(
    {
      id: "places",
      type: "circle",
      source: "places",
      paint: {
        "circle-radius": ["interpolate", ["linear"], ["zoom"], 10, 5, 16, 8],
        "circle-color": "#111827",
        "circle-stroke-color": "#ffffff",
        "circle-stroke-width": 2.5,
      },
    },
    "vehicle-dots",
  );
  map.addLayer(
    {
      id: "place-labels",
      type: "symbol",
      source: "places",
      minzoom: 10,
      layout: {
        "text-field": ["get", "label"],
        "text-font": ["Noto Sans Bold"],
        "text-size": 12,
        "text-offset": [0, 1.2],
        "text-anchor": "top",
        "text-optional": true,
      },
      paint: { "text-color": "#111827", "text-halo-color": "#ffffff", "text-halo-width": 1.5 },
    },
    "vehicle-dots",
  );
}

function addRouteLayers(map: MLMap, theme: keyof typeof ROUTE_CASING) {
  for (const id of ["selected-route", "selected-route-stops"]) {
    if (!map.getSource(id)) map.addSource(id, { type: "geojson", data: NO_FEATURES });
  }
  const width: ExpressionSpecification = ["interpolate", ["linear"], ["zoom"], 10, 3, 13, 5, 17, 9];
  const layout = { "line-cap": "round", "line-join": "round" } as const;
  map.addLayer(
    {
      id: "selected-route-casing",
      type: "line",
      source: "selected-route",
      layout,
      // Zoom must be the top-level interpolate, so the halo repeats the stops 4 px wider.
      paint: { "line-color": ROUTE_CASING[theme], "line-opacity": 0.9, "line-width": ["interpolate", ["linear"], ["zoom"], 10, 7, 13, 9, 17, 13] },
    },
    "stops",
  );
  map.addLayer({ id: "selected-route", type: "line", source: "selected-route", layout, paint: { "line-color": "#111827", "line-width": width } }, "stops");
  // The route's stops show at every zoom, above the general stop layer.
  map.addLayer(
    {
      id: "selected-route-stops",
      type: "circle",
      source: "selected-route-stops",
      paint: {
        "circle-radius": ["interpolate", ["linear"], ["zoom"], 10, 2, 13, 3.5, 17, 6],
        "circle-color": "#ffffff",
        "circle-stroke-color": "#111827",
        "circle-stroke-width": ["interpolate", ["linear"], ["zoom"], 10, 1.5, 15, 2.5],
      },
    },
    "vehicle-dots",
  );
}

function addStopLayers(map: MLMap, before: string) {
  if (!map.getSource("stops")) {
    map.addSource("stops", { type: "geojson", data: { type: "FeatureCollection", features: [] }, promoteId: "id" });
  }
  map.addLayer(
    {
      id: "stops",
      type: "circle",
      source: "stops",
      minzoom: STOPS_MIN_ZOOM,
      paint: {
        "circle-radius": ["interpolate", ["linear"], ["zoom"], STOPS_MIN_ZOOM, 3, 17, 6],
        "circle-color": "#ffffff",
        "circle-stroke-color": "#6b7280",
        "circle-stroke-width": ["case", ["==", ["get", "id"], ""], 3, 1.5],
      },
    },
    before,
  );
  map.addLayer(
    {
      id: "stop-labels",
      type: "symbol",
      source: "stops",
      minzoom: 16,
      layout: {
        "text-field": ["case", ["==", ["get", "platform"], ""], ["get", "name"], ["concat", ["get", "name"], " ", ["get", "platform"]]],
        "text-font": ["Noto Sans Regular"],
        "text-size": 11,
        "text-offset": [0, 1.1],
        "text-anchor": "top",
        "text-optional": true,
      },
      paint: { "text-color": "#6b7280", "text-halo-color": "#ffffff", "text-halo-width": 1.2 },
    },
    before,
  );
}

function stopFeatures(stops: MapStop[]) {
  return {
    type: "FeatureCollection" as const,
    features: stops.map((s) => ({
      type: "Feature" as const,
      id: s.id,
      geometry: { type: "Point" as const, coordinates: [s.lon, s.lat] },
      properties: { id: s.id, name: s.name, platform: s.platform },
    })),
  };
}

function placeFeatures(places: VehicleMapProps["places"]) {
  return {
    type: "FeatureCollection" as const,
    features: (places ?? []).map((p) => ({
      type: "Feature" as const,
      geometry: { type: "Point" as const, coordinates: [p.lon, p.lat] },
      properties: { id: p.id, label: p.label },
    })),
  };
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

export default function VehicleMap({ modes, lineTokens, stops, selected, onSelect, focus, highlightPattern, highlightMode, places, fit }: VehicleMapProps) {
  const { t } = useTranslation();
  const container = useRef<HTMLDivElement>(null);
  const [failed, setFailed] = useState(false);
  const mapRef = useRef<MLMap | null>(null);
  const filterRef = useRef({ modes, lineTokens });
  const dirtyRef = useRef(true);
  const onSelectRef = useRef(onSelect);
  const theme = useTheme();
  const themeRef = useRef(theme);
  const stopsRef = useRef(stops);
  const placesRef = useRef(places);

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
      style: styleUrl(themeRef.current),
      ...startView(),
      attributionControl: { compact: true },
      dragRotate: false,
      pitchWithRotate: false,
      // Positions update 20×/s while vehicles glide; fading symbols in and out on every
      // collision change would flicker.
      fadeDuration: 0,
    });
    mapRef.current = map;
    collapseAttribution(map);
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
    // Route shapes let markers follow the street between fixes instead of cutting corners.
    let shapeRetry: ReturnType<typeof setTimeout> | undefined;
    const fetchShapes = () =>
      void loadShapes().then((ok) => {
        if (!ok) {
          shapeRetry = setTimeout(fetchShapes, 30_000);
          return;
        }
        dirtyRef.current = true;
        map.getSource<GeoJSONSource>("rail")?.setData(railFeatures());
      });
    fetchShapes();

    // Without a basemap (tile host down, blocked) nothing else draws; say so instead of a blank box.
    map.on("error", () => {
      if (!map.isStyleLoaded()) setFailed(true);
    });
    map.on("style.load", () => {
      setFailed(false);
      addVehicleLayers(map);
      addStopLayers(map, "vehicle-dots");
      addRailLayers(map);
      addPlaceLayers(map);
      addRouteLayers(map, themeRef.current);
      if (shapes.size > 0) map.getSource<GeoJSONSource>("rail")?.setData(railFeatures());
      map.getSource<GeoJSONSource>("places")?.setData(placeFeatures(placesRef.current));
      map.getSource<GeoJSONSource>("stops")?.setData(stopFeatures(stopsRef.current));
      ready = true;
      dirtyRef.current = true;
    });

    const unsubscribe = live.subscribe((applied) => {
      const now = performance.now();
      if (applied.reset) {
        for (const id of tweens.keys()) if (!applied.state.vehicles.has(id)) tweens.delete(id);
      }
      for (const id of applied.removed) tweens.delete(id);
      for (const v of applied.changed) tweens.set(v.id, nextTween(tweens.get(v.id), v, now, animate, shapeOf));
      dirtyRef.current = true;
    });
    // The snapshot may have arrived before this (lazy-loaded) map subscribed;
    // without a starting point the first diff would teleport every vehicle.
    const start = performance.now();
    for (const v of live.getState().vehicles.values()) {
      if (!tweens.has(v.id)) tweens.set(v.id, nextTween(undefined, v, start, animate, shapeOf));
    }

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
      last = now; // also when idle, so a still map checks only every FRAME_MS
      if (!moving && !dirtyRef.current) return;
      dirtyRef.current = false;
      const { modes, lineTokens } = filterRef.current;
      const source = map.getSource<GeoJSONSource>("vehicles");
      source?.setData(vehicleFeatures(live.getState().vehicles.values(), tweens, now, modes, lineTokens, shapeOf));
    };
    raf = requestAnimationFrame(frame);

    // One click handler decides what was hit: vehicles win over stops.
    const vehicleLayers = ["vehicle-pill-selected", "vehicle-pills", "vehicle-dots"];
    const hitLayers = [...vehicleLayers, "stops"];
    map.on("click", (e: MapMouseEvent) => {
      const box: [[number, number], [number, number]] = [[e.point.x - 6, e.point.y - 6], [e.point.x + 6, e.point.y + 6]];
      const hits: MapGeoJSONFeature[] = map.queryRenderedFeatures(box, { layers: hitLayers.filter((l) => map.getLayer(l)) });
      const v = hits.find((f) => vehicleLayers.includes(f.layer.id));
      const st = hits.find((f) => f.layer.id === "stops");
      const id = (v ?? st)?.properties?.id as string | undefined;
      onSelectRef.current(id ? { kind: v ? "vehicle" : "stop", id } : undefined);
    });
    for (const l of hitLayers) {
      map.on("mouseenter", l, () => (map.getCanvas().style.cursor = "pointer"));
      map.on("mouseleave", l, () => (map.getCanvas().style.cursor = ""));
    }

    return () => {
      cancelAnimationFrame(raf);
      clearTimeout(shapeRetry);
      unsubscribe();
      map.remove();
      mapRef.current = null;
      const g = globalThis as { __tramMap?: MLMap };
      if (g.__tramMap === map) delete g.__tramMap;
    };
  }, []);

  // Swap the basemap when the theme changes; our layers are re-added on style.load.
  useEffect(() => {
    if (themeRef.current === theme) return;
    themeRef.current = theme;
    mapRef.current?.setStyle(styleUrl(theme));
  }, [theme]);

  useEffect(() => {
    stopsRef.current = stops;
    mapRef.current?.getSource<GeoJSONSource>("stops")?.setData(stopFeatures(stops));
  }, [stops]);

  const fitKey = fit && fit.length > 0 ? JSON.stringify([fit[0], fit[fit.length - 1], fit.length]) : "";
  useEffect(() => {
    const map = mapRef.current;
    if (!map || !fit || fit.length === 0) return;
    const b = new maplibregl.LngLatBounds(fit[0], fit[0]);
    for (const p of fit) b.extend(p);
    const go = () => map.fitBounds(b, { padding: 48, maxZoom: 16, animate: false });
    if (map.loaded()) go();
    else map.once("load", go);
    // fitKey stands for fit: a new array with the same route must not re-frame the map.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fitKey]);

  useEffect(() => {
    placesRef.current = places;
    mapRef.current?.getSource<GeoJSONSource>("places")?.setData(placeFeatures(places));
  }, [places]);

  // Railway lines are opt-in (Settings → Map) and follow the train filter chip.
  const [railPref] = useShowRail();
  const showRail = railPref && modes.has("train");
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const apply = () => {
      for (const l of ["rail", "rail-ties"]) if (map.getLayer(l)) map.setLayoutProperty(l, "visibility", showRail ? "visible" : "none");
    };
    apply();
    map.on("style.load", apply);
    return () => {
      map.off("style.load", apply);
    };
  }, [showRail]);

  useEffect(() => {
    const map = mapRef.current;
    // Keep the focused point above the bottom sheet that opens with it.
    if (focus && map) {
      const h = map.getContainer().clientHeight;
      map.flyTo({ center: focus, zoom: Math.max(map.getZoom(), 16), padding: { top: 0, left: 0, right: 0, bottom: h * (globalThis.matchMedia?.("(min-width: 768px)").matches ? 0 : 0.2) }, essential: true });
    }
  }, [focus]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    let stale = false;
    const apply = async () => {
      const color = MODE_COLORS[highlightMode ?? "bus"];
      for (const [layer, prop] of [["selected-route", "line-color"], ["selected-route-stops", "circle-stroke-color"]] as const) {
        if (map.getLayer(layer)) map.setPaintProperty(layer, prop, color);
      }
      if (highlightPattern) await loadShapes();
      const shape = highlightPattern ? shapes.get(highlightPattern) : undefined;
      const along = shape && highlightPattern ? await routeStopsAlong(highlightPattern) : [];
      if (stale) return;
      map.getSource<GeoJSONSource>("selected-route")?.setData(
        shape ? { type: "Feature", properties: {}, geometry: { type: "LineString", coordinates: shape.coords } } : NO_FEATURES,
      );
      map.getSource<GeoJSONSource>("selected-route-stops")?.setData({
        type: "FeatureCollection",
        features: shape
          ? along.map((m) => {
              const [lat, lon] = shape.pointAt(m);
              return { type: "Feature" as const, properties: {}, geometry: { type: "Point" as const, coordinates: [lon, lat] } };
            })
          : [],
      });
    };
    void apply();
    const onStyle = () => void apply();
    map.on("style.load", onStyle);
    return () => {
      stale = true;
      map.off("style.load", onStyle);
    };
  }, [highlightPattern, highlightMode]);

  const selectedVehicle = selected?.kind === "vehicle" ? selected.id : "";
  const selectedStop = selected?.kind === "stop" ? selected.id : "";
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const apply = () => {
      for (const l of ["vehicle-selected", "vehicle-pill-selected"]) {
        if (map.getLayer(l)) map.setFilter(l, ["==", ["get", "id"], selectedVehicle]);
      }
      // With a vehicle selected, vehicles of other lines fade so its route stands out.
      const sel = selectedVehicle ? live.getState().vehicles.get(selectedVehicle) : undefined;
      const opacity: ExpressionSpecification | number = sel
        ? ["case", ["all", ["==", ["get", "mode"], sel.mode], ["==", ["get", "text"], pillText(sel)]], 1, 0.3]
        : 1;
      const faded = [
        ["vehicle-dots", ["circle-opacity", "circle-stroke-opacity"]],
        ["vehicle-heading", ["icon-opacity"]],
        ["vehicle-pills", ["icon-opacity", "text-opacity"]],
      ] as const;
      for (const [layer, props] of faded) {
        if (map.getLayer(layer)) for (const p of props) map.setPaintProperty(layer, p, opacity);
      }
      if (map.getLayer("stops")) {
        map.setPaintProperty("stops", "circle-stroke-width", ["case", ["==", ["get", "id"], selectedStop], 3, 1.5]);
        map.setPaintProperty("stops", "circle-stroke-color", ["case", ["==", ["get", "id"], selectedStop], "#111827", "#6b7280"]);
      }
    };
    apply();
    map.on("style.load", apply);
    return () => {
      map.off("style.load", apply);
    };
  }, [selectedVehicle, selectedStop]);

  // MapLibre's stylesheet forces position: relative on the map element, so size it from a wrapper.
  return (
    <div className="absolute inset-0">
      <div ref={container} className="h-full w-full" role="region" aria-label={t("nav.map")} />
      {failed && (
        <p role="status" className="absolute inset-x-4 top-4 rounded-lg border bg-card p-3 text-sm text-muted-foreground shadow">
          {t("map.unavailable")}
        </p>
      )}
    </div>
  );
}

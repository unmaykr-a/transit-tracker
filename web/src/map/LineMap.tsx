import type { GeoJSONSource, Map as MLMap } from "maplibre-gl";
import { useEffect, useRef } from "react";

import type { PatternStop } from "@/api";
import { useTheme } from "@/lib/theme";
import type { Vehicle } from "@/lib/types";

import { collapseAttribution, maplibregl, styleUrl } from "./setup";

interface Props {
  shape: [number, number][];
  stops: PatternStop[];
  vehicles: Vehicle[];
  color: string;
}

const empty = { type: "FeatureCollection" as const, features: [] };

/** A small map of one pattern: its shape, stops and the vehicles on it. */
export default function LineMap({ shape, stops, vehicles, color }: Props) {
  const el = useRef<HTMLDivElement>(null);
  const mapRef = useRef<MLMap | null>(null);
  const theme = useTheme();
  const data = useRef({ shape, stops, vehicles, color });
  useEffect(() => {
    data.current = { shape, stops, vehicles, color };
  });

  useEffect(() => {
    if (!el.current) return;
    const map = new maplibregl.Map({
      container: el.current,
      style: styleUrl(theme),
      bounds: bounds(data.current.shape),
      fitBoundsOptions: { padding: 24 },
      attributionControl: { compact: true },
      dragRotate: false,
      pitchWithRotate: false,
      cooperativeGestures: true,
    });
    mapRef.current = map;
    collapseAttribution(map);
    // The container may not have its final size when the map is constructed.
    map.once("load", () => map.fitBounds(bounds(data.current.shape), { padding: 24, animate: false }));
    map.on("style.load", () => {
      map.addSource("shape", { type: "geojson", data: empty });
      map.addSource("stops", { type: "geojson", data: empty });
      map.addSource("line-vehicles", { type: "geojson", data: empty });
      map.addLayer({ id: "shape", type: "line", source: "shape", layout: { "line-cap": "round", "line-join": "round" }, paint: { "line-width": 4 } });
      map.addLayer({ id: "line-stops", type: "circle", source: "stops", paint: { "circle-radius": 3.5, "circle-color": "#ffffff", "circle-stroke-width": 2 } });
      map.addLayer({ id: "line-vehicles", type: "circle", source: "line-vehicles", paint: { "circle-radius": 7, "circle-stroke-color": "#ffffff", "circle-stroke-width": 2 } });
      apply(map, data.current);
    });
    return () => {
      map.remove();
      mapRef.current = null;
    };
    // The map is created once per theme; data changes are applied below.
  }, [theme]);

  useEffect(() => {
    const map = mapRef.current;
    if (map?.isStyleLoaded()) apply(map, data.current);
  }, [shape, stops, color]);

  // Vehicles change every couple of seconds; the shape and stops need not be re-uploaded with them.
  useEffect(() => {
    const map = mapRef.current;
    if (map?.isStyleLoaded()) applyVehicles(map, vehicles);
  }, [vehicles]);

  useEffect(() => {
    const map = mapRef.current;
    if (map && shape.length > 1) map.fitBounds(bounds(shape), { padding: 24, animate: false });
  }, [shape]);

  return (
    <div className="absolute inset-0">
      <div ref={el} className="h-full w-full" />
    </div>
  );
}

function bounds(shape: [number, number][]): [[number, number], [number, number]] {
  if (shape.length === 0) return [[24.55, 59.35], [24.95, 59.5]];
  let [w, s] = shape[0]!;
  let [e, n] = shape[0]!;
  for (const [x, y] of shape) {
    w = Math.min(w, x);
    e = Math.max(e, x);
    s = Math.min(s, y);
    n = Math.max(n, y);
  }
  return [[w, s], [e, n]];
}

function applyVehicles(map: MLMap, vehicles: Vehicle[]) {
  map.getSource<GeoJSONSource>("line-vehicles")?.setData({
    type: "FeatureCollection",
    features: vehicles.map((v) => ({ type: "Feature", properties: {}, geometry: { type: "Point", coordinates: [v.lon, v.lat] } })),
  });
}

function apply(map: MLMap, { shape, stops, vehicles, color }: Props) {
  map.getSource<GeoJSONSource>("shape")?.setData({ type: "Feature", properties: {}, geometry: { type: "LineString", coordinates: shape } });
  map.getSource<GeoJSONSource>("stops")?.setData({
    type: "FeatureCollection",
    features: stops.map((s) => ({ type: "Feature", properties: {}, geometry: { type: "Point", coordinates: [s.lon, s.lat] } })),
  });
  applyVehicles(map, vehicles);
  if (map.getLayer("shape")) {
    map.setPaintProperty("shape", "line-color", color);
    map.setPaintProperty("line-stops", "circle-stroke-color", color);
    map.setPaintProperty("line-vehicles", "circle-color", color);
  }
}

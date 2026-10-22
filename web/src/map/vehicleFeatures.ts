import type { FeatureCollection, Point } from "geojson";

import { matchesFilter, pillText } from "@/lib/lines";
import { positionAt, type ShapeLookup, type Tween } from "@/lib/tween";
import { MODES, type Mode, type Vehicle } from "@/lib/types";

export interface VehicleFeatureProps {
  id: string;
  mode: Mode;
  text: string;
  bearing: number;
  sort: number;
}

export type VehicleFeatureCollection = FeatureCollection<Point, VehicleFeatureProps>;

// Symbol placement priority: lower sorts first, so rare modes win collisions over the bus swarm.
const SORT: Record<Mode, number> = { train: 0, tram: 1, trolley: 2, bus: 3 };

/** Builds the GeoJSON drawn by the vehicle layers at time now. */
export function vehicleFeatures(
  vehicles: Iterable<Vehicle>,
  tweens: ReadonlyMap<string, Tween>,
  now: number,
  modes: ReadonlySet<Mode>,
  lineTokens: string[],
  shapeOf?: ShapeLookup,
): VehicleFeatureCollection {
  const features: VehicleFeatureCollection["features"] = [];
  for (const v of vehicles) {
    if (!matchesFilter(v, modes, lineTokens)) continue;
    const tw = tweens.get(v.id);
    const [lat, lon] = tw ? positionAt(tw, now, shapeOf) : [v.lat, v.lon];
    features.push({
      type: "Feature",
      id: v.id,
      geometry: { type: "Point", coordinates: [lon, lat] },
      properties: { id: v.id, mode: v.mode, text: pillText(v), bearing: v.bearing, sort: SORT[v.mode] },
    });
  }
  return { type: "FeatureCollection", features };
}

export interface ImageSpec {
  width: number;
  height: number;
  data: Uint8ClampedArray;
  pixelRatio: number;
  stretchX?: [number, number][];
  stretchY?: [number, number][];
  content?: [number, number, number, number];
}

function canvas(w: number, h: number) {
  const c = document.createElement("canvas");
  c.width = w;
  c.height = h;
  const ctx = c.getContext("2d");
  if (!ctx) throw new Error("canvas 2d unavailable");
  return ctx;
}

/** A stretchable rounded pill used with icon-text-fit so it grows with the line number. */
export function pillImage(color: string, pixelRatio: number): ImageSpec {
  const r = 11 * pixelRatio;
  const border = 1.5 * pixelRatio;
  const w = r * 2 + 2;
  const h = r * 2;
  const ctx = canvas(w, h);
  ctx.fillStyle = "#ffffff";
  ctx.beginPath();
  ctx.roundRect(0, 0, w, h, r);
  ctx.fill();
  ctx.fillStyle = color;
  ctx.beginPath();
  ctx.roundRect(border, border, w - 2 * border, h - 2 * border, r - border);
  ctx.fill();
  return {
    width: w,
    height: h,
    data: ctx.getImageData(0, 0, w, h).data,
    pixelRatio,
    stretchX: [[r, r + 2]],
    stretchY: [[r - 1, r + 1]],
    content: [r * 0.6, 2, w - r * 0.6, h - 2],
  };
}

/** A small heading wedge at the top of a square; rotated by bearing it points the way the vehicle goes. */
export function wedgeImage(color: string, pixelRatio: number): ImageSpec {
  const s = 52 * pixelRatio;
  const ctx = canvas(s, s);
  const c = s / 2;
  ctx.fillStyle = color;
  ctx.strokeStyle = "#ffffff";
  ctx.lineWidth = 1.5 * pixelRatio;
  ctx.beginPath();
  ctx.moveTo(c, 1 * pixelRatio);
  ctx.lineTo(c - 6 * pixelRatio, 11 * pixelRatio);
  ctx.lineTo(c + 6 * pixelRatio, 11 * pixelRatio);
  ctx.closePath();
  ctx.fill();
  ctx.stroke();
  return { width: s, height: s, data: ctx.getImageData(0, 0, s, s).data, pixelRatio };
}

export const ALL_MODES: ReadonlySet<Mode> = new Set(MODES);

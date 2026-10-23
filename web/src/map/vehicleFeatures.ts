import type { FeatureCollection, Point } from "geojson";

import { matchesFilter, pillText } from "@/lib/lines";
import { positionAt, type ShapeLookup, type Tween } from "@/lib/tween";
import { KINDS, kindOf, type Kind, type Mode, type Vehicle } from "@/lib/types";

export interface VehicleFeatureProps {
  id: string;
  mode: Mode;
  kind: Kind;
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
  modes: ReadonlySet<Kind>,
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
      properties: { id: v.id, mode: v.mode, kind: kindOf(v), text: pillText(v), bearing: v.bearing, sort: SORT[v.mode] },
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

export const ALL_MODES: ReadonlySet<Kind> = new Set(KINDS);

/** Body proportions seen from above (width × length in px at 1×) per kind. */
const TOPDOWN_SIZE: Record<Kind, [number, number]> = {
  tram: [11, 46],
  bus: [11, 30],
  trolley: [11, 30],
  county: [11, 32],
  train: [12, 56],
};

/**
 * A vehicle seen from above, nose up (north at bearing 0), in its kind's colour:
 * articulated tram with pantographs, bus with a roof box, trolleybus with its
 * two poles, coach-like county bus, train with a pointed cab. Rotated by the
 * map with the vehicle's bearing.
 */
export function topdownImage(kind: Kind, color: string, pixelRatio: number): ImageSpec {
  const [bw, bl] = TOPDOWN_SIZE[kind];
  const pad = 3;
  const w = (bw + pad * 2) * pixelRatio;
  const h = (bl + pad * 2) * pixelRatio;
  const ctx = canvas(Math.ceil(w), Math.ceil(h));
  ctx.scale(pixelRatio, pixelRatio);
  const x = pad;
  const y = pad;
  const r = kind === "train" ? 5 : 3;
  // Body with a white outline so it reads on any basemap.
  ctx.lineWidth = 1.6;
  ctx.strokeStyle = "#ffffff";
  ctx.fillStyle = color;
  ctx.beginPath();
  if (kind === "train") {
    // Pointed cab at the front.
    ctx.moveTo(x + bw / 2, y);
    ctx.quadraticCurveTo(x + bw, y + 2, x + bw, y + 9);
    ctx.lineTo(x + bw, y + bl - r);
    ctx.quadraticCurveTo(x + bw, y + bl, x + bw - r, y + bl);
    ctx.lineTo(x + r, y + bl);
    ctx.quadraticCurveTo(x, y + bl, x, y + bl - r);
    ctx.lineTo(x, y + 9);
    ctx.quadraticCurveTo(x, y + 2, x + bw / 2, y);
  } else {
    ctx.roundRect(x, y, bw, bl, r);
  }
  ctx.fill();
  ctx.stroke();
  // Windscreen at the front.
  ctx.fillStyle = "rgba(15, 23, 42, 0.75)";
  ctx.beginPath();
  if (kind === "train") ctx.roundRect(x + 2.5, y + 3.5, bw - 5, 4, 1.5);
  else ctx.roundRect(x + 1.5, y + 1.5, bw - 3, 3.5, 1.5);
  ctx.fill();
  // Roof details.
  ctx.fillStyle = "rgba(255, 255, 255, 0.35)";
  ctx.strokeStyle = "rgba(15, 23, 42, 0.45)";
  ctx.lineWidth = 0.9;
  if (kind === "tram") {
    for (const f of [1 / 3, 2 / 3]) {
      ctx.beginPath(); // articulation joints
      ctx.moveTo(x + 0.8, y + bl * f);
      ctx.lineTo(x + bw - 0.8, y + bl * f);
      ctx.stroke();
    }
    ctx.fillRect(x + bw / 2 - 2.5, y + bl / 6 - 1.5, 5, 3); // pantographs
    ctx.fillRect(x + bw / 2 - 2.5, y + (bl * 5) / 6 - 1.5, 5, 3);
  } else if (kind === "trolley") {
    ctx.strokeStyle = "rgba(15, 23, 42, 0.8)";
    ctx.lineWidth = 1;
    for (const dx of [-2, 2]) {
      ctx.beginPath(); // the two poles, from the rear up to the wires
      ctx.moveTo(x + bw / 2 + dx, y + bl - 4);
      ctx.lineTo(x + bw / 2 + dx * 1.3, y + 8);
      ctx.stroke();
    }
  } else if (kind === "train") {
    for (const f of [0.38, 0.68]) {
      ctx.beginPath(); // car ends
      ctx.moveTo(x + 0.8, y + bl * f);
      ctx.lineTo(x + bw - 0.8, y + bl * f);
      ctx.stroke();
    }
    ctx.fillRect(x + bw / 2 - 3, y + 14, 6, 3);
  } else {
    ctx.fillRect(x + 2.5, y + bl / 2 - 3, bw - 5, 6); // roof unit
    if (kind === "county") ctx.fillRect(x + 3, y + bl - 7, bw - 6, 3); // rear hatch
  }
  return { width: Math.ceil(w), height: Math.ceil(h), data: ctx.getImageData(0, 0, Math.ceil(w), Math.ceil(h)).data, pixelRatio };
}

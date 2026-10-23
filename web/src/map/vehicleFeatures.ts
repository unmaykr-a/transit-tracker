import type { FeatureCollection, Point } from "geojson";

import { matchesFilter, pillText } from "@/lib/lines";
import { positionAt, type ShapeLookup, type Tween } from "@/lib/tween";
import { LIVERY_COLORS, liveryOf, type Livery } from "@/lib/livery";
import { KINDS, kindOf, type Kind, type Mode, type Vehicle } from "@/lib/types";

export interface VehicleFeatureProps {
  id: string;
  mode: Mode;
  kind: Kind;
  livery: Livery;
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
      properties: { id: v.id, mode: v.mode, kind: kindOf(v), livery: liveryOf(v), text: pillText(v), bearing: v.bearing, sort: SORT[v.mode] },
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

/** Body proportions seen from above (width × length in px at 1×) per livery. */
const TOPDOWN_SIZE: Record<Livery, [number, number]> = {
  "tlt-tram": [13, 46],
  "tlt-bus": [13, 32],
  "tlt-trolley": [13, 32],
  tartu: [13, 32],
  county: [13, 34],
  elron: [13, 56],
};

/**
 * A vehicle seen from above, nose up (north at bearing 0), in its livery (see
 * lib/livery): a white roof edged in the operator's colour — TLT's dashes, a
 * solid red edge for Tartu, Elron's orange cab — with a dark outline so it reads
 * on light and dark maps; pantographs and joints on trams, the two poles on
 * trolleybuses, gas tanks on buses — keeping the middle of the roof clear for
 * the line number. Rotated by the map with the bearing.
 */
export function topdownImage(livery: Livery, pixelRatio: number): ImageSpec {
  const c = LIVERY_COLORS[livery];
  const [bw, bl] = TOPDOWN_SIZE[livery];
  const train = livery === "elron";
  const pad = 3;
  const w = Math.ceil((bw + pad * 2) * pixelRatio);
  const h = Math.ceil((bl + pad * 2) * pixelRatio);
  const ctx = canvas(w, h);
  ctx.scale(pixelRatio, pixelRatio);
  const x = pad;
  const y = pad;
  const r = train ? 5 : 3;
  const body = () => {
    ctx.beginPath();
    if (train) {
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
  };
  // A light halo, then the roof with a dark outline.
  body();
  ctx.lineWidth = 2.6;
  ctx.strokeStyle = "rgba(255, 255, 255, 0.85)";
  ctx.stroke();
  ctx.fillStyle = c.body;
  ctx.fill();
  ctx.save();
  body();
  ctx.clip();
  // The operator's colour along the edges.
  ctx.fillStyle = c.accent;
  if (livery === "tartu" || livery === "county") {
    const e = livery === "tartu" ? 2 : 1.2;
    ctx.fillRect(x, y, e, bl);
    ctx.fillRect(x + bw - e, y, e, bl);
  } else if (train) {
    ctx.fillRect(x, y, bw, 11); // orange cab
    ctx.fillRect(x, y + bl - 3, bw, 3);
    ctx.fillStyle = c.line ?? c.accent;
    ctx.fillRect(x, y + 11, 1.2, bl - 14);
    ctx.fillRect(x + bw - 1.2, y + 11, 1.2, bl - 14);
  } else {
    for (let d = y + 6; d < y + bl - 2; d += 4) {
      ctx.fillRect(x, d, 1.6, 2.2);
      ctx.fillRect(x + bw - 1.6, d, 1.6, 2.2);
    }
    ctx.fillRect(x, y, bw, livery === "tlt-tram" ? 3 : 2); // nose
  }
  ctx.restore();
  body();
  ctx.lineWidth = 1;
  ctx.strokeStyle = "#1e293b";
  ctx.stroke();
  // Windscreen at the front.
  ctx.fillStyle = c.band;
  ctx.beginPath();
  if (train) ctx.roundRect(x + 2.5, y + 4, bw - 5, 3.5, 1.5);
  else ctx.roundRect(x + 1.8, y + (livery === "tlt-tram" ? 3.2 : 2.2), bw - 3.6, 3, 1.2);
  ctx.fill();
  // Roof details.
  ctx.fillStyle = "rgba(100, 116, 139, 0.55)";
  ctx.strokeStyle = "rgba(15, 23, 42, 0.5)";
  ctx.lineWidth = 0.9;
  const joint = (f: number) => {
    ctx.beginPath();
    ctx.moveTo(x + 0.8, y + bl * f);
    ctx.lineTo(x + bw - 0.8, y + bl * f);
    ctx.stroke();
  };
  if (livery === "tlt-tram") {
    joint(1 / 3);
    joint(2 / 3);
    ctx.fillRect(x + bw / 2 - 2.5, y + bl / 6 - 1, 5, 3); // pantograph
  } else if (livery === "tlt-trolley") {
    ctx.strokeStyle = "rgba(15, 23, 42, 0.85)";
    ctx.lineWidth = 1;
    for (const dx of [-2, 2]) {
      ctx.beginPath(); // the two poles, from the rear up to the wires
      ctx.moveTo(x + bw / 2 + dx * 1.6, y + bl - 3);
      ctx.lineTo(x + bw / 2 + dx * 2, y + 7);
      ctx.stroke();
    }
  } else if (train) {
    joint(0.5);
    ctx.fillRect(x + bw / 2 - 3, y + 14, 6, 3);
  } else if (livery === "county") {
    ctx.fillRect(x + 3, y + bl - 8, bw - 6, 4); // rear roof unit
  } else {
    ctx.fillRect(x + 2.5, y + 6, bw - 5, 5); // gas tanks
  }
  return { width: w, height: h, data: ctx.getImageData(0, 0, w, h).data, pixelRatio };
}

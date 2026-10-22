/** Decodes a Google encoded polyline (precision 5) into [lon, lat] pairs. */
export function decodePolyline(s: string): [number, number][] {
  const out: [number, number][] = [];
  let lat = 0;
  let lon = 0;
  let i = 0;
  const next = () => {
    let result = 0;
    let shift = 0;
    let b: number;
    do {
      b = s.charCodeAt(i++) - 63;
      result |= (b & 0x1f) << shift;
      shift += 5;
    } while (b >= 0x20 && i < s.length + 1);
    return result & 1 ? ~(result >> 1) : result >> 1;
  };
  while (i < s.length) {
    lat += next();
    lon += next();
    out.push([lon / 1e5, lat / 1e5]);
  }
  return out;
}

/** A pattern shape with cumulative distances, for positions along the route. */
export class Shape {
  readonly cum: number[];
  constructor(readonly coords: [number, number][]) {
    this.cum = [0];
    for (let i = 1; i < coords.length; i++) {
      const [x1, y1] = coords[i - 1]!;
      const [x2, y2] = coords[i]!;
      const kx = 111_320 * Math.cos(((y1 + y2) / 2) * (Math.PI / 180));
      this.cum.push(this.cum[i - 1]! + Math.hypot((x2 - x1) * kx, (y2 - y1) * 111_320));
    }
  }

  get length(): number {
    return this.cum[this.cum.length - 1] ?? 0;
  }

  /** [lat, lon] at `along` metres from the start (clamped). */
  pointAt(along: number): [number, number] {
    const n = this.coords.length;
    if (n === 0) return [0, 0];
    if (along <= 0) return [this.coords[0]![1], this.coords[0]![0]];
    if (along >= this.length) return [this.coords[n - 1]![1], this.coords[n - 1]![0]];
    let lo = 0;
    let hi = n - 1;
    while (hi - lo > 1) {
      const mid = (lo + hi) >> 1;
      if (this.cum[mid]! <= along) lo = mid;
      else hi = mid;
    }
    const span = this.cum[hi]! - this.cum[lo]!;
    const t = span > 0 ? (along - this.cum[lo]!) / span : 0;
    const [x1, y1] = this.coords[lo]!;
    const [x2, y2] = this.coords[hi]!;
    return [y1 + t * (y2 - y1), x1 + t * (x2 - x1)];
  }
}

/** Shapes by pattern id, filled once from /api/v1/patterns. */
export const shapes = new Map<string, Shape>();
/** Route id ("train:…") by pattern id. */
export const patternRoutes = new Map<string, string>();
let loading: Promise<void> | undefined;

export function loadShapes(): Promise<void> {
  loading ??= fetch("/api/v1/patterns")
    .then((r) => (r.ok ? (r.json() as Promise<{ patterns: { id: string; route_id: string; polyline: string }[] }>) : { patterns: [] }))
    .then((d) => {
      for (const p of d.patterns) {
        shapes.set(p.id, new Shape(decodePolyline(p.polyline)));
        patternRoutes.set(p.id, p.route_id);
      }
    })
    .catch(() => {
      loading = undefined; // retry on next call
    });
  return loading;
}

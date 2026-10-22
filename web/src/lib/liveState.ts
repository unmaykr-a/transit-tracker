import type { FeedStatus, LiveMessage, Vehicle } from "./types";

export interface LiveState {
  vehicles: Map<string, Vehicle>;
  feeds: FeedStatus[];
  version: number;
  /** Server clock minus local clock, ms. Used for "updated N s ago". */
  clockSkewMs: number;
}

export function emptyLiveState(): LiveState {
  return { vehicles: new Map(), feeds: [], version: 0, clockSkewMs: 0 };
}

export interface Applied {
  state: LiveState;
  /** Vehicles added or changed by this message. */
  changed: Vehicle[];
  /** Ids removed by this message. */
  removed: string[];
  /** True when this message replaced the whole set. */
  reset: boolean;
}

/** Applies one server message. The input state is not mutated. */
export function applyMessage(prev: LiveState, msg: LiveMessage, now = Date.now()): Applied {
  const clockSkewMs = Date.parse(msg.server_time) - now;
  switch (msg.type) {
    case "snapshot": {
      const vehicles = new Map((msg.vehicles ?? []).map((v) => [v.id, v]));
      const removed = [...prev.vehicles.keys()].filter((id) => !vehicles.has(id));
      return {
        state: { vehicles, feeds: msg.feeds ?? prev.feeds, version: msg.version, clockSkewMs },
        changed: [...vehicles.values()],
        removed,
        reset: true,
      };
    }
    case "diff": {
      const vehicles = new Map(prev.vehicles);
      const changed: Vehicle[] = [];
      for (const v of msg.upsert ?? []) {
        vehicles.set(v.id, v);
        changed.push(v);
      }
      for (const [id, lat, lon, bearing, speed_mps, t, along] of msg.move ?? []) {
        const old = vehicles.get(id);
        if (!old) continue; // move for a vehicle we never saw; the next snapshot fixes it
        const v = { ...old, lat, lon, bearing, speed_mps, updated_at: new Date(t).toISOString(), dist_along_m: along || undefined };
        vehicles.set(id, v);
        changed.push(v);
      }
      const removed = (msg.remove ?? []).filter((id) => vehicles.delete(id));
      return {
        state: { ...prev, vehicles, version: msg.version, clockSkewMs },
        changed,
        removed,
        reset: false,
      };
    }
    case "stale":
      return { state: { ...prev, feeds: msg.feeds ?? prev.feeds, clockSkewMs }, changed: [], removed: [], reset: false };
  }
}

export function staleFeeds(feeds: FeedStatus[]): FeedStatus[] {
  return feeds.filter((f) => f.stale);
}

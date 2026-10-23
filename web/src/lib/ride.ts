import type { TripStop } from "@/api";

import { load, save } from "./storage";

/** On-board mode: the vehicle you ride and the stop you get off at (index along its pattern). */
export interface Ride {
  vehicle: string;
  pattern: string;
  stop: string;
  index: number;
  name: string;
}

export type RidePhase =
  | { kind: "riding"; stopsLeft: number; eta: string } // stops before yours
  | { kind: "prepare"; eta: string } // yours is the stop after the next one
  | { kind: "next"; eta: string } // yours is the next stop
  | { kind: "arrived" } // yours is behind the vehicle
  | { kind: "unknown" }; // no prediction (lost the match, another pattern)

/** Where the ride stands, from the vehicle's stops ahead. */
export function ridePhase(ride: Ride, pattern: string | undefined, ahead: TripStop[] | undefined): RidePhase {
  if (!ahead || ahead.length === 0 || pattern !== ride.pattern) return { kind: "unknown" };
  const next = ahead[0]!;
  if (next.index > ride.index) return { kind: "arrived" };
  const mine = ahead.find((a) => a.index === ride.index);
  const stopsLeft = ride.index - next.index;
  // The prediction may not reach far enough ahead yet: still riding.
  const eta = mine?.eta ?? "";
  if (stopsLeft === 0) return { kind: "next", eta };
  if (stopsLeft === 1) return { kind: "prepare", eta };
  return { kind: "riding", stopsLeft, eta };
}

const KEY = "bus.ride";

/** The ride survives a reload of the tab (not a new device: it is only on this phone). */
export const loadRide = (): Ride | null => load<Ride | null>(KEY, null);
export const saveRide = (r: Ride | null) => save(KEY, r);

/** Buzz the phone for a ride alert, where the browser allows it (Android; not iOS). */
export function buzz(pattern: number[]) {
  try {
    globalThis.navigator?.vibrate?.(pattern);
  } catch {
    // vibration is a nicety
  }
}

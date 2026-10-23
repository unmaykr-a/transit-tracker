import { lazy, Suspense } from "react";

import { cn } from "@/lib/utils";

import { useMapPaneState } from "./mapPane";

const VehicleMap = lazy(() => import("./VehicleMap"));

/** The app's one live map, behind the page; see mapPane.ts. */
export function MapHost() {
  const { mode, props, kept } = useMapPaneState();
  if (!kept || !props) return null;
  return (
    <div
      aria-hidden={!mode || undefined}
      className={cn(
        "absolute inset-y-0 right-0",
        mode === "side" ? "left-[30rem]" : "left-0",
        !mode && "invisible", // kept for a quick way back; stays sized so nothing has to re-layout
      )}
    >
      <Suspense fallback={null}>
        <VehicleMap {...props} active={!!mode} />
      </Suspense>
    </div>
  );
}

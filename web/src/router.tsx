import { createRootRoute, createRoute, createRouter, lazyRouteComponent } from "@tanstack/react-router";

import { AppShell } from "@/components/AppShell";
import HomePage from "@/pages/HomePage";

const rootRoute = createRootRoute({ component: AppShell });

// The router parses search values as JSON, so ?stop=1234 arrives as a number.
const str = (v: unknown) => (typeof v === "string" && v !== "" ? v : typeof v === "number" ? String(v) : undefined);

const routeTree = rootRoute.addChildren([
  createRoute({ getParentRoute: () => rootRoute, path: "/", component: HomePage }),
  // The map pulls in MapLibre (~1 MB); load it only when visited.
  createRoute({
    getParentRoute: () => rootRoute,
    path: "/map",
    validateSearch: (s: Record<string, unknown>): { stop?: string; lines?: string } => ({ stop: str(s.stop), lines: str(s.lines) }),
    component: lazyRouteComponent(() => import("@/pages/MapPage")),
  }),
  createRoute({
    getParentRoute: () => rootRoute,
    path: "/search",
    validateSearch: (s: Record<string, unknown>): { q?: string } => ({ q: str(s.q) }),
    component: lazyRouteComponent(() => import("@/pages/SearchPage")),
  }),
  createRoute({
    getParentRoute: () => rootRoute,
    path: "/stops/$id",
    validateSearch: (s: Record<string, unknown>): { p?: string } => ({ p: str(s.p) }),
    component: lazyRouteComponent(() => import("@/pages/StopPage")),
  }),
  createRoute({ getParentRoute: () => rootRoute, path: "/lines/$mode/$line", component: lazyRouteComponent(() => import("@/pages/LinePage")) }),
  createRoute({ getParentRoute: () => rootRoute, path: "/settings", component: lazyRouteComponent(() => import("@/pages/SettingsPage")) }),
]);

export function makeRouter() {
  return createRouter({ routeTree, defaultPreload: "intent", scrollRestoration: true });
}

export const router = makeRouter();

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

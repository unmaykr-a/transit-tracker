import { createRootRoute, createRoute, createRouter, lazyRouteComponent } from "@tanstack/react-router";

import { AppShell } from "@/components/AppShell";
import HomePage from "@/pages/HomePage";

const rootRoute = createRootRoute({ component: AppShell });

const routeTree = rootRoute.addChildren([
  createRoute({ getParentRoute: () => rootRoute, path: "/", component: HomePage }),
  // The map pulls in MapLibre (~1 MB); load it only when visited.
  createRoute({ getParentRoute: () => rootRoute, path: "/map", component: lazyRouteComponent(() => import("@/pages/MapPage")) }),
  createRoute({ getParentRoute: () => rootRoute, path: "/search", component: lazyRouteComponent(() => import("@/pages/SearchPage")) }),
  createRoute({ getParentRoute: () => rootRoute, path: "/settings", component: lazyRouteComponent(() => import("@/pages/SettingsPage")) }),
]);

export function makeRouter() {
  return createRouter({ routeTree, defaultPreload: "intent" });
}

export const router = makeRouter();

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

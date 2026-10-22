import { createRootRoute, createRoute, createRouter, lazyRouteComponent, redirect } from "@tanstack/react-router";

import { AppShell } from "@/components/AppShell";
import HomePage from "@/pages/HomePage";

const rootRoute = createRootRoute({ component: AppShell });

// The router parses search values as JSON, so ?stop=1234 arrives as a number.
const str = (v: unknown) => (typeof v === "string" && v !== "" ? v : typeof v === "number" ? String(v) : undefined);

const adminRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin",
  component: lazyRouteComponent(() => import("@/pages/admin/AdminLayout")),
});

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
  createRoute({
    getParentRoute: () => rootRoute,
    path: "/trips/new",
    validateSearch: (s: Record<string, unknown>): { from?: string } => ({ from: str(s.from) }),
    component: lazyRouteComponent(() => import("@/pages/TripPage")),
  }),
  createRoute({
    getParentRoute: () => rootRoute,
    path: "/settings",
    // Set by the single sign-on link callback.
    validateSearch: (s: Record<string, unknown>): { sso?: string; warn?: string; error?: string } => ({
      sso: str(s.sso),
      warn: str(s.warn),
      error: str(s.error),
    }),
    component: lazyRouteComponent(() => import("@/pages/SettingsPage")),
  }),
  createRoute({
    getParentRoute: () => rootRoute,
    path: "/login",
    validateSearch: (s: Record<string, unknown>): { next?: string; error?: string } => ({ next: str(s.next), error: str(s.error) }),
    component: lazyRouteComponent(() => import("@/pages/AuthPages"), "LoginPage"),
  }),
  createRoute({ getParentRoute: () => rootRoute, path: "/setup", component: lazyRouteComponent(() => import("@/pages/AuthPages"), "SetupPage") }),
  createRoute({
    getParentRoute: () => rootRoute,
    path: "/signup",
    validateSearch: (s: Record<string, unknown>): { invite?: string } => ({ invite: str(s.invite) }),
    component: lazyRouteComponent(() => import("@/pages/AuthPages"), "SignupPage"),
  }),
  adminRoute.addChildren([
    createRoute({ getParentRoute: () => adminRoute, path: "/", beforeLoad: () => { throw redirect({ to: "/admin/users" }); } }),
    createRoute({ getParentRoute: () => adminRoute, path: "/users", component: lazyRouteComponent(() => import("@/pages/admin/UsersPage")) }),
    createRoute({ getParentRoute: () => adminRoute, path: "/invites", component: lazyRouteComponent(() => import("@/pages/admin/InvitesPage")) }),
    createRoute({ getParentRoute: () => adminRoute, path: "/tokens", component: lazyRouteComponent(() => import("@/pages/admin/TokensPage")) }),
    createRoute({ getParentRoute: () => adminRoute, path: "/settings", component: lazyRouteComponent(() => import("@/pages/admin/SettingsAdminPage")) }),
    createRoute({ getParentRoute: () => adminRoute, path: "/health", component: lazyRouteComponent(() => import("@/pages/admin/HealthPage")) }),
    createRoute({ getParentRoute: () => adminRoute, path: "/audit", component: lazyRouteComponent(() => import("@/pages/admin/AuditPage")) }),
    createRoute({ getParentRoute: () => adminRoute, path: "/about", component: lazyRouteComponent(() => import("@/pages/admin/AboutPage")) }),
  ]),
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

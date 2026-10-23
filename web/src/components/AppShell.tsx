import { Link, Navigate, Outlet, useLocation } from "@tanstack/react-router";
import { Home, Map, Search, Settings } from "lucide-react";
import { useEffect } from "react";
import { useTranslation } from "react-i18next";

import { applyUserPrefs, useSession } from "@/lib/session";
import { cn } from "@/lib/utils";
import { MapHost } from "@/map/MapHost";
import { useMapPaneMode } from "@/map/mapPane";

const OPEN_PATHS = ["/login", "/signup", "/setup"];

const tabs = [
  { to: "/", icon: Home, key: "nav.home" },
  { to: "/map", icon: Map, key: "nav.map" },
  { to: "/search", icon: Search, key: "nav.search" },
  { to: "/settings", icon: Settings, key: "nav.settings" },
] as const;

/** Bottom tab bar on phones, left rail on wider screens. */
export function AppShell() {
  const { t } = useTranslation();
  const session = useSession();
  const { pathname, href } = useLocation();
  const user = session.data?.user;
  const mapShown = !!useMapPaneMode();
  useEffect(() => applyUserPrefs(user), [user?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const s = session.data;
  if (s?.setup_required && pathname !== "/setup") return <Navigate to="/setup" replace />;
  if (s && !s.setup_required && pathname === "/setup") return <Navigate to="/" replace />;
  if (s && !s.user && !s.settings.public_map && !OPEN_PATHS.includes(pathname)) {
    return <Navigate to="/login" search={{ next: href }} replace />;
  }
  return (
    <div className="flex h-dvh flex-col md:flex-row-reverse">
      {/* The shared live map sits behind the page; pages that show it let pointer events through to it. */}
      <div className="relative min-h-0 flex-1">
        <MapHost />
        <main className={cn("relative h-full overflow-y-auto", mapShown && "pointer-events-none")}>
          <Outlet />
        </main>
      </div>
      <nav
        aria-label={t("nav.main")}
        className="z-10 border-t bg-background pb-[env(safe-area-inset-bottom)] md:w-20 md:border-t-0 md:border-r md:pb-0"
      >
        <ul className="flex md:flex-col md:gap-1 md:pt-4">
          {tabs.map(({ to, icon: Icon, key }) => (
            <li key={to} className="flex-1 md:flex-none">
              <Link
                to={to}
                activeOptions={{ exact: to === "/" }}
                className="flex flex-col items-center gap-1 py-2 text-xs text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring data-[status=active]:text-foreground"
              >
                <Icon className="size-5" aria-hidden />
                {t(key)}
              </Link>
            </li>
          ))}
        </ul>
      </nav>
    </div>
  );
}

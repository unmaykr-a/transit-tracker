import { Link, Outlet } from "@tanstack/react-router";
import { Home, Map, Search, Settings } from "lucide-react";
import { useTranslation } from "react-i18next";

const tabs = [
  { to: "/", icon: Home, key: "nav.home" },
  { to: "/map", icon: Map, key: "nav.map" },
  { to: "/search", icon: Search, key: "nav.search" },
  { to: "/settings", icon: Settings, key: "nav.settings" },
] as const;

/** Bottom tab bar on phones, left rail on wider screens. */
export function AppShell() {
  const { t } = useTranslation();
  return (
    <div className="flex h-dvh flex-col md:flex-row-reverse">
      <main className="relative min-h-0 flex-1 overflow-y-auto">
        <Outlet />
      </main>
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

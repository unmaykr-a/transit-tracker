import { Link, Outlet } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { useSession } from "@/lib/session";

const tabs = [
  { to: "/admin/users", key: "admin.users" },
  { to: "/admin/invites", key: "admin.invites" },
  { to: "/admin/settings", key: "admin.settings" },
  { to: "/admin/health", key: "admin.health" },
  { to: "/admin/audit", key: "admin.audit" },
  { to: "/admin/about", key: "admin.about" },
] as const;

export default function AdminLayout() {
  const { t } = useTranslation();
  const session = useSession();
  if (session.data && session.data.user?.role !== "admin") {
    return <p className="p-6 text-muted-foreground">{t("auth.loginRequired")}</p>;
  }
  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-6 px-4 py-6">
      <h1 className="text-2xl font-semibold tracking-tight">{t("admin.title")}</h1>
      <nav aria-label={t("admin.title")} className="-mx-4 overflow-x-auto px-4">
        <ul className="flex gap-1 border-b">
          {tabs.map((tab) => (
            <li key={tab.to}>
              <Link
                to={tab.to}
                className="block border-b-2 border-transparent px-3 py-2 text-sm whitespace-nowrap text-muted-foreground hover:text-foreground data-[status=active]:border-foreground data-[status=active]:font-medium data-[status=active]:text-foreground"
              >
                {t(tab.key)}
              </Link>
            </li>
          ))}
        </ul>
      </nav>
      <Outlet />
    </div>
  );
}

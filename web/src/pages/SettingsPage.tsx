import { useQuery } from "@tanstack/react-query";
import { Footprints, Info, Palette, Plug, UserRound } from "lucide-react";
import { Trans, useTranslation } from "react-i18next";

import { Link } from "@tanstack/react-router";

import { fetchHealth, updateMe } from "@/api";
import { AccountActions, AccountSection } from "@/components/AccountSection";
import { ApiTokensSection } from "@/components/ApiTokensSection";
import { WalkingSection } from "@/components/WalkingSection";
import { SettingRow, SettingsGroup } from "@/components/SettingsGroup";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { LANGUAGES, setLanguage } from "@/i18n";
import { useShowRail } from "@/lib/mapPrefs";
import { useSession } from "@/lib/session";
import { setThemePref, useThemePref, type ThemePref } from "@/lib/theme";
import { useLiveState } from "@/lib/useLive";
import { cn } from "@/lib/utils";
import { radioKeyDown, radioTabIndex } from "@/lib/radioGroup";

function Segmented<T extends string>({
  value,
  options,
  onChange,
  label,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
  label: string;
}) {
  return (
    <div role="radiogroup" aria-label={label} className="inline-flex w-fit rounded-lg border bg-muted p-1">
      {options.map((o, i) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={value === o.value}
          tabIndex={radioTabIndex(value === o.value, i, options.some((x) => x.value === value))}
          onKeyDown={radioKeyDown}
          onClick={() => onChange(o.value)}
          className={cn(
            "rounded-md px-3 py-1.5 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            value === o.value ? "bg-background font-medium shadow-sm" : "text-muted-foreground hover:text-foreground",
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

const GROUPS = [
  { id: "account", icon: UserRound, title: "account.title" },
  { id: "display", icon: Palette, title: "settings.display" },
  { id: "walking", icon: Footprints, title: "walking.title", signedIn: true },
  { id: "apps", icon: Plug, title: "settings.apps", signedIn: true },
  { id: "about", icon: Info, title: "settings.about" },
] as const;

export default function SettingsPage() {
  const { t, i18n } = useTranslation();
  const theme = useThemePref();
  const [showRail, setShowRail] = useShowRail();
  const live = useLiveState(5000);
  const health = useQuery({
    queryKey: ["health"],
    queryFn: ({ signal }) => fetchHealth(signal),
  });
  const session = useSession();
  const user = session.data?.user;
  // Logged-in users keep their language and theme across devices.
  const remember = (p: { locale?: string; theme?: string }) => {
    if (user) void updateMe(p);
  };
  const groups = GROUPS.filter((g) => !("signedIn" in g) || user);
  const icon = (id: string) => GROUPS.find((g) => g.id === id)!.icon;

  return (
    <div className="mx-auto flex max-w-2xl flex-col px-4 pt-8 pb-10">
      <h1 className="mb-2 text-2xl font-semibold tracking-tight">{t("settings.title")}</h1>
      {/* Jump to a group (the page is long on phones); stays at the top while scrolling. */}
      <div className="sticky top-0 z-10 -mx-4 mb-6 bg-background/95 px-4 py-2 backdrop-blur supports-[backdrop-filter]:bg-background/80">
        <nav aria-label={t("settings.sections")} className="-mx-4 overflow-x-auto px-4">
          <ul className="flex gap-2">
            {groups.map((g) => (
              <li key={g.id}>
                <button
                  type="button"
                  onClick={() => document.getElementById(g.id)?.scrollIntoView({ behavior: "smooth", block: "start" })}
                  className="inline-flex h-8 items-center gap-1.5 whitespace-nowrap rounded-full border bg-card px-3 text-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <g.icon className="size-4" aria-hidden />
                  {t(g.title)}
                </button>
              </li>
            ))}
          </ul>
        </nav>
      </div>

      <div className="flex flex-col gap-10">
        <SettingsGroup
          id="account"
          icon={icon("account")}
          title={t("account.title")}
          description={user ? t("account.signedInAs", { name: user.username }) : undefined}
          actions={user ? <AccountActions user={user} /> : undefined}
        >
          {user ? (
            <AccountSection user={user} />
          ) : (
            session.data && (
              <div className="flex items-center justify-between gap-4 rounded-xl border bg-card p-4">
                <p className="text-sm text-muted-foreground">{t("account.signedOut")}</p>
                <Button size="sm" asChild>
                  <Link to="/login">{t("auth.login")}</Link>
                </Button>
              </div>
            )
          )}
        </SettingsGroup>

        <SettingsGroup id="display" icon={icon("display")} title={t("settings.display")}>
          <div className="flex flex-col gap-5 rounded-xl border bg-card p-4">
            <SettingRow id="set-theme" label={t("settings.theme")}>
              <Segmented<ThemePref>
                label={t("settings.theme")}
                value={theme}
                onChange={(v) => {
                  setThemePref(v);
                  remember({ theme: v });
                }}
                options={[
                  { value: "system", label: t("settings.themeSystem") },
                  { value: "light", label: t("settings.themeLight") },
                  { value: "dark", label: t("settings.themeDark") },
                ]}
              />
            </SettingRow>
            <SettingRow id="set-language" label={t("settings.language")}>
              <Segmented
                label={t("settings.language")}
                value={i18n.resolvedLanguage ?? "en"}
                onChange={(lng) => {
                  setLanguage(lng);
                  remember({ locale: lng });
                }}
                options={LANGUAGES.map((l) => ({
                  value: l.code,
                  label: l.label,
                }))}
              />
            </SettingRow>
            <SettingRow id="set-rail" label={t("settings.railLines")} hint={t("settings.railLinesHint")}>
              <Switch checked={showRail} onChange={setShowRail} aria-labelledby="set-rail" />
            </SettingRow>
          </div>
        </SettingsGroup>

        {user && (
          <SettingsGroup id="walking" icon={icon("walking")} title={t("walking.title")} description={t("settings.walkingIntro")}>
            <WalkingSection />
          </SettingsGroup>
        )}

        {user && (
          <SettingsGroup id="apps" icon={icon("apps")} title={t("settings.apps")}>
            <ApiTokensSection />
          </SettingsGroup>
        )}

        <SettingsGroup id="about" icon={icon("about")} title={t("settings.about")}>
          <div className="flex flex-col gap-2">
            <h3 className="text-sm font-medium">{t("settings.feeds")}</h3>
            <ul className="divide-y rounded-xl border bg-card">
              {live.feeds.length === 0 && <li className="px-4 py-3 text-sm text-muted-foreground">{t("common.loading")}</li>}
              {live.feeds.map((f) => (
                <li key={f.name} className="flex items-center justify-between gap-4 px-4 py-3 text-sm">
                  <span className="first-letter:uppercase">{t(`live.feed.${f.name}`, { defaultValue: f.name })}</span>
                  <span className="text-muted-foreground tabular-nums">
                    {t("live.vehicles", { count: f.count })} ·{" "}
                    <span className={f.stale ? "text-amber-700 dark:text-amber-400" : undefined}>
                      {f.stale ? t("settings.feedStale") : t("settings.feedOk")}
                    </span>
                  </span>
                </li>
              ))}
            </ul>
          </div>
          <div className="flex flex-col gap-1 text-sm text-muted-foreground">
            {health.data && <p>{t("settings.version", { version: health.data.version })}</p>}
            <p>
              <Trans
                i18nKey="settings.credits"
                components={{
                  ofm: <a className="underline" href="https://openfreemap.org" />,
                  osm: <a className="underline" href="https://www.openstreetmap.org/copyright" />,
                }}
              />
            </p>
          </div>
        </SettingsGroup>
      </div>
    </div>
  );
}

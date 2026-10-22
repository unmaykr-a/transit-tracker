import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import { Link } from "@tanstack/react-router";

import { fetchHealth, updateMe } from "@/api";
import { AccountSection } from "@/components/AccountSection";
import { ApiTokensSection } from "@/components/ApiTokensSection";
import { WalkingSection } from "@/components/WalkingSection";
import { Button } from "@/components/ui/button";
import { LANGUAGES } from "@/i18n";
import { useSession } from "@/lib/session";
import { setThemePref, useThemePref, type ThemePref } from "@/lib/theme";
import { useLiveState } from "@/lib/useLive";
import { cn } from "@/lib/utils";

function Segmented<T extends string>({ value, options, onChange, label }: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
  label: string;
}) {
  return (
    <div role="radiogroup" aria-label={label} className="inline-flex rounded-lg border bg-muted p-1">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={value === o.value}
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

export default function SettingsPage() {
  const { t, i18n } = useTranslation();
  const theme = useThemePref();
  const live = useLiveState(5000);
  const health = useQuery({ queryKey: ["health"], queryFn: ({ signal }) => fetchHealth(signal) });
  const session = useSession();
  const user = session.data?.user;
  // Logged-in users keep their language and theme across devices.
  const remember = (p: { locale?: string; theme?: string }) => {
    if (user) void updateMe(p);
  };

  return (
    <div className="mx-auto flex max-w-xl flex-col gap-8 px-4 py-8">
      <h1 className="text-2xl font-semibold tracking-tight">{t("settings.title")}</h1>

      {user ? (
        <AccountSection user={user} />
      ) : (
        session.data && (
          <section className="flex items-center justify-between gap-4 rounded-xl border bg-card p-4">
            <p className="text-sm text-muted-foreground">{t("account.signedOut")}</p>
            <Button size="sm" asChild>
              <Link to="/login">{t("auth.login")}</Link>
            </Button>
          </section>
        )
      )}

      {user && <WalkingSection />}

      {user && <ApiTokensSection />}

      <section className="flex flex-col gap-2">
        <h2 className="font-medium">{t("settings.theme")}</h2>
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
      </section>

      <section className="flex flex-col gap-2">
        <h2 className="font-medium">{t("settings.language")}</h2>
        <Segmented
          label={t("settings.language")}
          value={i18n.resolvedLanguage ?? "en"}
          onChange={(lng) => {
            void i18n.changeLanguage(lng);
            remember({ locale: lng });
          }}
          options={LANGUAGES.map((l) => ({ value: l.code, label: l.label }))}
        />
      </section>

      <section className="flex flex-col gap-2">
        <h2 className="font-medium">{t("settings.feeds")}</h2>
        <ul className="divide-y rounded-xl border bg-card">
          {live.feeds.map((f) => (
            <li key={f.name} className="flex items-center justify-between gap-4 px-4 py-3 text-sm">
              <span>{t(`live.feed.${f.name}`, { defaultValue: f.name })}</span>
              <span className="text-muted-foreground tabular-nums">
                {t("live.vehicles", { count: f.count })} ·{" "}
                <span className={f.stale ? "text-amber-600 dark:text-amber-400" : undefined}>
                  {f.stale ? t("settings.feedStale") : t("settings.feedOk")}
                </span>
              </span>
            </li>
          ))}
        </ul>
      </section>

      <section className="flex flex-col gap-1 text-sm text-muted-foreground">
        <h2 className="font-medium text-foreground">{t("settings.about")}</h2>
        {health.data && <p>{t("settings.version", { version: health.data.version })}</p>}
        <p>
          Map © <a className="underline" href="https://openfreemap.org">OpenFreeMap</a>,{" "}
          <a className="underline" href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors. Data: Tallinna
          Transpordiamet, Elron.
        </p>
      </section>
    </div>
  );
}

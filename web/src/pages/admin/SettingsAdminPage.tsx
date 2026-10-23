import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { admin, ApiError, type AdminSettings } from "@/api";
import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/input";
import { SESSION_KEY } from "@/lib/session";

type Key = keyof AdminSettings;
type Draft = Partial<Record<Key, unknown>>;

const general: { key: Key; label: string; kind?: "text" | "number" | "url" | "center" }[] = [
  { key: "instance_name", label: "admin.instanceName" },
  { key: "map_style_light", label: "admin.mapStyleLight", kind: "url" },
  { key: "map_style_dark", label: "admin.mapStyleDark", kind: "url" },
  { key: "map_center", label: "admin.mapCenter", kind: "center" },
  { key: "map_zoom", label: "admin.mapZoom", kind: "number" },
];

const sources: { key: Key; label: string; kind?: "number" | "url" }[] = [
  { key: "gps_url", label: "admin.gpsUrl", kind: "url" },
  { key: "gps_interval_s", label: "admin.gpsInterval", kind: "number" },
  { key: "elron_url", label: "admin.elronUrl", kind: "url" },
  { key: "elron_interval_s", label: "admin.elronInterval", kind: "number" },
  { key: "county_vehicles_url", label: "admin.countyVehiclesUrl", kind: "url" },
  { key: "county_trips_url", label: "admin.countyTripsUrl", kind: "url" },
  { key: "siri_url", label: "admin.siriUrl", kind: "url" },
  { key: "otp_url", label: "admin.otpUrl", kind: "url" },
  { key: "stops_url", label: "admin.stopsUrl", kind: "url" },
  { key: "elron_gtfs_url", label: "admin.elronGtfsUrl", kind: "url" },
  { key: "geocoder_url", label: "admin.geocoderUrl", kind: "url" },
  { key: "user_agent_contact", label: "admin.uaContact", kind: "url" },
  { key: "transitous_url", label: "admin.transitousUrl", kind: "url" },
];

// The straight-line estimate is always the last resort, so it is not listed.
const WALK_PROVIDERS = ["otp", "transitous"] as const;

function show(v: unknown): string {
  return Array.isArray(v) ? v.join(", ") : String(v ?? "");
}

const numeric = new Set<Key>([...general, ...sources].filter((f) => f.kind === "number" || f.kind === "center").map((f) => f.key));

/** Number and centre fields are edited as text and parsed only when saved, so "59.4," can be typed. */
function parse(draft: Draft, invalid: string): Draft {
  const out: Draft = {};
  const num = (k: Key, x: string) => {
    const n = x.trim() === "" ? NaN : Number(x);
    if (!Number.isFinite(n)) throw new ApiError(400, invalid, k);
    return n;
  };
  for (const [k, v] of Object.entries(draft) as [Key, unknown][]) {
    if (!numeric.has(k) || typeof v !== "string") out[k] = v;
    else if (k === "map_center") out[k] = v.split(",").map((x) => num(k, x));
    else out[k] = num(k, v);
  }
  return out;
}

export default function SettingsAdminPage() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["admin", "settings"], queryFn: ({ signal }) => admin.settings(signal) });
  const [draft, setDraft] = useState<Draft>({});
  const save = useMutation({
    mutationFn: async (patch: Draft) => admin.updateSettings(parse(patch, t("admin.invalidNumber"))),
    onSuccess: (res, patch) => {
      qc.setQueryData(["admin", "settings"], res);
      // Keep unsaved edits of other fields (a reset saves just its own key).
      setDraft((d) => Object.fromEntries(Object.entries(d).filter(([k]) => !(k in patch))));
      void qc.invalidateQueries({ queryKey: SESSION_KEY });
    },
  });
  const refresh = useMutation({ mutationFn: admin.refreshStatic, onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "health"] }) });
  if (!q.data) return <p className="text-muted-foreground">{t("common.loading")}</p>;
  const { settings, defaults, overrides } = q.data;
  const value = <K extends Key>(k: K): AdminSettings[K] => (k in draft ? (draft[k] as AdminSettings[K]) : settings[k]);
  const set = (k: Key, v: unknown) => setDraft((d) => ({ ...d, [k]: v }));
  const err = (k: Key) => (save.error instanceof ApiError && save.error.field === k ? save.error.message : undefined);

  const input = (key: Key, label: string, kind?: string) => {
    const v = value(key);
    const control =
      kind === "center" || kind === "number" ? (
        <Input value={show(v)} onChange={(e) => set(key, e.target.value)} inputMode="decimal" aria-invalid={!!err(key) || undefined} />
      ) : (
        <Input type={kind === "url" ? "url" : "text"} value={show(v)} onChange={(e) => set(key, e.target.value)} aria-invalid={!!err(key) || undefined} />
      );
    return (
      <div key={key} className="flex flex-col gap-1">
        <Field label={t(label)} error={err(key)} hint={overrides[key] ? t("admin.default", { value: show(defaults[key]) }) : undefined}>
          {control}
        </Field>
        {overrides[key] && (
          <button type="button" className="self-start text-xs text-muted-foreground underline" onClick={() => save.mutate({ [key]: null })}>
            {t("admin.resetDefault")}
          </button>
        )}
      </div>
    );
  };

  const section = (id: string, title: string, intro: string | undefined, children: React.ReactNode) => (
    <section aria-labelledby={id} className="grid gap-4 rounded-xl border bg-card p-4">
      <div>
        <h2 id={id} className="font-medium">
          {title}
        </h2>
        {intro && <p className="text-sm text-muted-foreground">{intro}</p>}
      </div>
      {children}
    </section>
  );
  // A feed URL with its poll interval beside it.
  const feed = (url: Key, urlLabel: string, interval: Key, intervalLabel: string) => (
    <div className="grid gap-4 sm:grid-cols-[1fr_9rem]">
      {input(url, urlLabel, "url")}
      {input(interval, intervalLabel, "number")}
    </div>
  );
  const providers = value("walk_providers");
  const setProviders = (list: string[]) => set("walk_providers", list);

  return (
    <form
      className="flex flex-col gap-6"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate(draft);
      }}
    >
      {section(
        "set-general",
        t("admin.general"),
        undefined,
        <>
          {input("instance_name", "admin.instanceName")}
          <Field label={t("admin.signupMode")} error={err("signup_mode")}>
            <Select value={value("signup_mode")} onChange={(e) => set("signup_mode", e.target.value)}>
              <option value="off">{t("admin.signupOff")}</option>
              <option value="invite">{t("admin.signupInvite")}</option>
              <option value="open">{t("admin.signupOpen")}</option>
            </Select>
          </Field>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" className="size-4" checked={value("public_map")} onChange={(e) => set("public_map", e.target.checked)} />
            {t("admin.publicMap")}
          </label>
        </>,
      )}

      {section(
        "set-defaults",
        t("admin.visitorDefaults"),
        t("admin.visitorDefaultsIntro"),
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t("admin.defaultLocale")}>
            <Select value={value("default_locale")} onChange={(e) => set("default_locale", e.target.value)}>
              <option value="">{t("admin.browser")}</option>
              <option value="et">Eesti</option>
              <option value="en">English</option>
            </Select>
          </Field>
          <Field label={t("admin.defaultTheme")}>
            <Select value={value("default_theme")} onChange={(e) => set("default_theme", e.target.value)}>
              <option value="system">{t("settings.themeSystem")}</option>
              <option value="light">{t("settings.themeLight")}</option>
              <option value="dark">{t("settings.themeDark")}</option>
            </Select>
          </Field>
        </div>,
      )}

      {section(
        "set-map",
        t("admin.mapSection"),
        undefined,
        <>
          {input("map_style_light", "admin.mapStyleLight", "url")}
          {input("map_style_dark", "admin.mapStyleDark", "url")}
          <div className="grid gap-4 sm:grid-cols-[1fr_9rem]">
            {input("map_center", "admin.mapCenter", "center")}
            {input("map_zoom", "admin.mapZoom", "number")}
          </div>
        </>,
      )}

      {section(
        "set-live",
        t("admin.liveSection"),
        t("admin.liveIntro"),
        <>
          {feed("gps_url", "admin.gpsUrl", "gps_interval_s", "admin.gpsInterval")}
          {feed("elron_url", "admin.elronUrl", "elron_interval_s", "admin.elronInterval")}
          {input("county_vehicles_url", "admin.countyVehiclesUrl", "url")}
          {input("county_trips_url", "admin.countyTripsUrl", "url")}
          <p className="text-xs text-muted-foreground">{t("admin.countyHint")}</p>
        </>,
      )}

      {section(
        "set-static",
        t("admin.timetableSection"),
        t("admin.timetableIntro"),
        <>
          {input("siri_url", "admin.siriUrl", "url")}
          {input("otp_url", "admin.otpUrl", "url")}
          {input("stops_url", "admin.stopsUrl", "url")}
          {input("elron_gtfs_url", "admin.elronGtfsUrl", "url")}
          <div className="flex flex-wrap items-center gap-3">
            <Button type="button" variant="outline" disabled={refresh.isPending} onClick={() => refresh.mutate()}>
              {refresh.isPending ? t("admin.refreshing") : t("admin.refreshStatic")}
            </Button>
            {refresh.error && <span className="text-sm text-red-600 dark:text-red-400">{(refresh.error as Error).message}</span>}
            {refresh.data?.static && (
              <span className="text-sm text-muted-foreground">
                {t("admin.stops")}: {refresh.data.static.stops} · {t("admin.routes")}: {refresh.data.static.routes}
              </span>
            )}
          </div>
        </>,
      )}

      {section(
        "set-walk",
        t("admin.walkSection"),
        undefined,
        <>
          <fieldset className="flex flex-col gap-2">
            <legend className="mb-1 text-sm font-medium">{t("admin.walkProviders")}</legend>
            <p className="text-sm text-muted-foreground">{t("admin.walkProvidersHint")}</p>
            <ol className="divide-y rounded-lg border">
              {WALK_PROVIDERS.map((name) => {
                const at = providers.indexOf(name);
                return (
                  <li key={name} className="flex items-center gap-3 px-3 py-2 text-sm">
                    <input
                      id={`walk-${name}`}
                      type="checkbox"
                      className="size-4"
                      checked={at >= 0}
                      onChange={(e) => setProviders(e.target.checked ? [...providers, name] : providers.filter((p) => p !== name))}
                    />
                    <label htmlFor={`walk-${name}`} className="flex-1">
                      {t(`admin.walk.${name}`)}
                    </label>
                    {at >= 0 && <span className="text-xs text-muted-foreground tabular-nums">#{at + 1}</span>}
                    {at > 0 && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => setProviders(providers.with(at - 1, name).with(at, providers[at - 1]!))}
                        aria-label={t("admin.walkFirst", { name: t(`admin.walk.${name}`) })}
                      >
                        ↑
                      </Button>
                    )}
                  </li>
                );
              })}
            </ol>
            {err("walk_providers") && <p className="text-sm text-red-600 dark:text-red-400">{err("walk_providers")}</p>}
          </fieldset>
          {input("transitous_url", "admin.transitousUrl", "url")}
          {input("geocoder_url", "admin.geocoderUrl", "url")}
        </>,
      )}

      {section("set-polite", t("admin.politeSection"), t("admin.politeIntro"), input("user_agent_contact", "admin.uaContact", "url"))}

      {save.error && !(save.error instanceof ApiError && save.error.field) && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {(save.error as Error).message}
        </p>
      )}
      <div className="sticky bottom-0 flex items-center gap-3 border-t bg-background py-3">
        <Button type="submit" disabled={Object.keys(draft).length === 0 || save.isPending}>
          {t("account.save")}
        </Button>
        {Object.keys(draft).length > 0 && <span className="text-sm text-muted-foreground">{t("admin.unsaved", { count: Object.keys(draft).length })}</span>}
        {save.isSuccess && Object.keys(draft).length === 0 && <span className="text-sm text-muted-foreground">{t("account.saved")}</span>}
      </div>
    </form>
  );
}

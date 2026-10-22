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
  { key: "siri_url", label: "admin.siriUrl", kind: "url" },
  { key: "otp_url", label: "admin.otpUrl", kind: "url" },
  { key: "stops_url", label: "admin.stopsUrl", kind: "url" },
  { key: "elron_gtfs_url", label: "admin.elronGtfsUrl", kind: "url" },
  { key: "geocoder_url", label: "admin.geocoderUrl", kind: "url" },
  { key: "user_agent_contact", label: "admin.uaContact", kind: "url" },
];

function show(v: unknown): string {
  return Array.isArray(v) ? v.join(", ") : String(v ?? "");
}

export default function SettingsAdminPage() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["admin", "settings"], queryFn: ({ signal }) => admin.settings(signal) });
  const [draft, setDraft] = useState<Draft>({});
  const save = useMutation({
    mutationFn: (patch: Draft) => admin.updateSettings(patch),
    onSuccess: (res) => {
      qc.setQueryData(["admin", "settings"], res);
      setDraft({});
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
      kind === "center" ? (
        <Input
          value={show(v)}
          onChange={(e) => set(key, e.target.value.split(",").map((x) => Number(x.trim())))}
          inputMode="decimal"
        />
      ) : (
        <Input
          type={kind === "number" ? "number" : kind === "url" ? "url" : "text"}
          value={show(v)}
          onChange={(e) => set(key, kind === "number" ? Number(e.target.value) : e.target.value)}
        />
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

  return (
    <form
      className="flex flex-col gap-6"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate(draft);
      }}
    >
      <section className="grid gap-4 rounded-xl border bg-card p-4">
        <h2 className="font-medium">{t("admin.general")}</h2>
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
        </div>
        {general.slice(1).map((f) => input(f.key, f.label, f.kind))}
      </section>

      <section className="grid gap-4 rounded-xl border bg-card p-4">
        <h2 className="font-medium">{t("admin.dataSources")}</h2>
        {sources.map((f) => input(f.key, f.label, f.kind))}
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
      </section>

      {save.error && !(save.error instanceof ApiError && save.error.field) && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {(save.error as Error).message}
        </p>
      )}
      <div className="sticky bottom-0 flex items-center gap-3 bg-background py-3">
        <Button type="submit" disabled={Object.keys(draft).length === 0 || save.isPending}>
          {t("account.save")}
        </Button>
        {save.isSuccess && Object.keys(draft).length === 0 && <span className="text-sm text-muted-foreground">{t("account.saved")}</span>}
      </div>
    </form>
  );
}

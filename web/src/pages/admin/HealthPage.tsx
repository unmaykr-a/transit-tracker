import { useQuery } from "@tanstack/react-query";
import { CircleAlert, CircleCheck } from "lucide-react";
import { useTranslation } from "react-i18next";

import { admin } from "@/api";
import { relative } from "@/lib/format";
import { useNow } from "@/lib/useLive";

import { Sparkline } from "./Sparkline";

export default function HealthPage() {
  const { t, i18n } = useTranslation();
  const now = useNow(30_000);
  const lang = i18n.resolvedLanguage;
  const q = useQuery({ queryKey: ["admin", "health"], queryFn: ({ signal }) => admin.health(signal), refetchInterval: 10_000 });
  if (!q.data) return <p className="text-muted-foreground">{t("common.loading")}</p>;
  const st = q.data.static;
  return (
    <div className="flex flex-col gap-4">
      {q.data.feeds.map((f) => (
        <section key={f.name} className="flex flex-wrap items-start justify-between gap-4 rounded-xl border bg-card p-4" aria-label={f.name}>
          <div className="flex min-w-48 flex-col gap-1 text-sm">
            <h2 className="flex items-center gap-2 font-medium">
              {f.stale ? <CircleAlert className="size-4 text-amber-700" aria-hidden /> : <CircleCheck className="size-4 text-emerald-600" aria-hidden />}
              <span className="first-letter:uppercase">{t(`live.feed.${f.name}`, { defaultValue: f.name })}</span>
              <span className="font-normal text-muted-foreground">· {f.stale ? t("admin.stale") : t("admin.ok")}</span>
            </h2>
            <p className="text-muted-foreground">
              {t("admin.lastSuccess")}: {f.last_success ? relative(f.last_success, lang, now) : t("admin.never")} · {t("admin.latency")}: {f.latency_ms} ms ·{" "}
              {t("admin.items")}: {f.count}
            </p>
            {f.last_error && (
              <p className="text-red-600 dark:text-red-400">
                {t("admin.lastError")}: {f.last_error} {f.last_error_at && `(${relative(f.last_error_at, lang, now)})`}
              </p>
            )}
          </div>
          <Sparkline points={f.history} label={t("admin.vehiclesLastHour")} />
        </section>
      ))}
      {st && (
        <section className="flex flex-col gap-1 rounded-xl border bg-card p-4 text-sm">
          <h2 className="font-medium">{t("admin.staticData")}</h2>
          <p className="text-muted-foreground">
            {t("admin.importedAt")}: {st.imported_at ? relative(st.imported_at, lang, now) : t("admin.never")} ({st.duration_ms} ms) · {t("admin.stops")}: {st.stops} ·{" "}
            {t("admin.routes")}: {st.routes} · {t("admin.patterns")}: {st.patterns}
          </p>
          {st.last_error && (
            <p className="text-red-600 dark:text-red-400">
              {t("admin.lastError")}: {st.last_error}
            </p>
          )}
        </section>
      )}
    </div>
  );
}

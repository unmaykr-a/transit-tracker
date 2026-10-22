import { useQuery } from "@tanstack/react-query";
import { Download } from "lucide-react";
import { useTranslation } from "react-i18next";

import { admin } from "@/api";
import { Button } from "@/components/ui/button";
import { bytes } from "@/lib/format";

function uptime(s: number): string {
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  return d > 0 ? `${d}d ${h}h` : h > 0 ? `${h}h ${m}m` : `${m}m`;
}

export default function AboutPage() {
  const { t } = useTranslation();
  const q = useQuery({ queryKey: ["admin", "about"], queryFn: ({ signal }) => admin.about(signal) });
  if (!q.data) return <p className="text-muted-foreground">{t("common.loading")}</p>;
  const a = q.data;
  const rows: [string, string][] = [
    [t("admin.version"), a.version],
    [t("admin.commit"), a.commit],
    [t("admin.platform"), `${a.os}/${a.arch} · ${a.go}`],
    [t("admin.uptime"), uptime(a.uptime_s)],
    [t("admin.dbSize"), bytes(a.db_bytes)],
    [t("admin.users"), String(a.users)],
    [t("admin.dataDir"), a.data_dir],
  ];
  return (
    <div className="flex flex-col gap-6">
      <dl className="divide-y rounded-xl border bg-card text-sm">
        {rows.map(([k, v]) => (
          <div key={k} className="flex justify-between gap-4 px-4 py-2">
            <dt className="text-muted-foreground">{k}</dt>
            <dd className="truncate font-medium">{v}</dd>
          </div>
        ))}
      </dl>
      <section className="flex flex-col items-start gap-2">
        <Button asChild>
          <a href="/api/v1/admin/backup" download>
            <Download className="size-4" aria-hidden />
            {t("admin.backup")}
          </a>
        </Button>
        <p className="text-sm text-muted-foreground">{t("admin.backupHint")}</p>
      </section>
    </div>
  );
}

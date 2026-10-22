import { useInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { admin } from "@/api";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/input";
import { clock } from "@/lib/format";

const ACTIONS = [
  "setup.complete", "user.signup", "user.create", "user.role", "user.disable", "user.enable", "user.password_reset",
  "user.password_change", "user.sessions_revoke", "user.delete", "invite.create", "invite.revoke", "settings.update",
  "static.refresh", "backup.download",
];

export default function AuditPage() {
  const { t, i18n } = useTranslation();
  const [action, setAction] = useState("");
  const q = useInfiniteQuery({
    queryKey: ["admin", "audit", action],
    queryFn: ({ pageParam, signal }) => admin.audit(action, pageParam, signal),
    initialPageParam: 0,
    getNextPageParam: (last) => (last.entries.length === 50 ? last.entries[last.entries.length - 1]!.id : undefined),
  });
  const entries = q.data?.pages.flatMap((p) => p.entries) ?? [];
  return (
    <div className="flex flex-col gap-4">
      <Select value={action} onChange={(e) => setAction(e.target.value)} aria-label={t("admin.allActions")} className="self-start">
        <option value="">{t("admin.allActions")}</option>
        {ACTIONS.map((a) => (
          <option key={a} value={a}>
            {a}
          </option>
        ))}
      </Select>
      <ol className="divide-y rounded-xl border bg-card text-sm">
        {entries.length === 0 && !q.isPending && <li className="px-4 py-3 text-muted-foreground">{t("admin.noEntries")}</li>}
        {entries.map((e) => (
          <li key={e.id} className="flex flex-wrap gap-x-3 gap-y-1 px-4 py-2">
            <span className="text-muted-foreground tabular-nums">
              {new Date(e.at).toLocaleDateString(i18n.resolvedLanguage)} {clock(e.at, i18n.resolvedLanguage)}
            </span>
            <span className="font-medium">{e.actor || "—"}</span>
            <span className="font-mono text-xs leading-5">{e.action}</span>
            {e.target && <span>{e.target}</span>}
            {e.detail && <span className="text-muted-foreground">{e.detail}</span>}
          </li>
        ))}
      </ol>
      {q.hasNextPage && (
        <Button variant="outline" className="self-start" onClick={() => void q.fetchNextPage()} disabled={q.isFetchingNextPage}>
          {t("admin.loadMore")}
        </Button>
      )}
    </div>
  );
}

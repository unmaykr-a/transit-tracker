import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import { admin } from "@/api";
import { relative } from "@/lib/format";
import { ConfirmButton } from "./ConfirmButton";

export default function TokensPage() {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ["admin", "tokens"], queryFn: ({ signal }) => admin.tokens(signal) });
  const revoke = useMutation({ mutationFn: admin.revokeToken, onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "tokens"] }) });

  return (
    <ul className="divide-y rounded-xl border bg-card" aria-label={t("admin.tokens")}>
      {list.data?.tokens.length === 0 && <li className="px-4 py-3 text-sm text-muted-foreground">{t("tokens.none")}</li>}
      {list.data?.tokens.map((tok) => (
        <li key={tok.id} className="flex flex-wrap items-center gap-3 px-4 py-3 text-sm">
          <div className="min-w-0 flex-1">
            <p className="font-medium">
              {tok.username} · {tok.name}
            </p>
            <p className="text-muted-foreground">
              <span className="font-mono">{tok.prefix}…</span> · {t("tokens.created", { when: relative(tok.created_at, i18n.resolvedLanguage) })} ·{" "}
              {tok.last_used_at
                ? t("tokens.lastUsed", { when: relative(tok.last_used_at, i18n.resolvedLanguage) }) + (tok.last_used_ip ? ` (${tok.last_used_ip})` : "")
                : t("tokens.neverUsed")}
            </p>
          </div>
          <ConfirmButton size="sm" variant="ghost" question={t("tokens.confirmRevoke", { name: tok.name, user: tok.username })} onConfirm={() => revoke.mutate(tok.id)}>
            {t("tokens.revoke")}
          </ConfirmButton>
        </li>
      ))}
    </ul>
  );
}

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { admin, type Role } from "@/api";
import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/input";
import { clock, relative } from "@/lib/format";
import { useNow } from "@/lib/useLive";

export default function InvitesPage() {
  const { t, i18n } = useTranslation();
  const now = useNow(30_000);
  const qc = useQueryClient();
  const invites = useQuery({ queryKey: ["admin", "invites"], queryFn: ({ signal }) => admin.invites(signal) });
  const [role, setRole] = useState<Role>("user");
  const [days, setDays] = useState(7);
  const [link, setLink] = useState<string>();
  const [copied, setCopied] = useState(false);
  const create = useMutation({
    mutationFn: () => admin.createInvite(role, days * 24),
    onSuccess: (res) => {
      setLink(`${location.origin}${res.path}`);
      setCopied(false);
      void qc.invalidateQueries({ queryKey: ["admin", "invites"] });
    },
  });
  const revoke = useMutation({ mutationFn: admin.deleteInvite, onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "invites"] }) });

  return (
    <div className="flex flex-col gap-6">
      <form
        className="flex flex-wrap items-end gap-3 rounded-xl border bg-card p-4"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <Field label={t("admin.role")}>
          <Select value={role} onChange={(e) => setRole(e.target.value as Role)}>
            <option value="user">{t("auth.roleNames.user")}</option>
            <option value="admin">{t("auth.roleNames.admin")}</option>
          </Select>
        </Field>
        <Field label={t("admin.expiresIn")}>
          <Select value={days} onChange={(e) => setDays(Number(e.target.value))}>
            {[1, 7, 30].map((d) => (
              <option key={d} value={d}>
                {t("admin.days", { count: d })}
              </option>
            ))}
          </Select>
        </Field>
        <Button type="submit" disabled={create.isPending}>
          {t("admin.createInvite")}
        </Button>
      </form>

      {link && (
        <div className="flex flex-col gap-2 rounded-xl border border-amber-300 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-950">
          <p className="text-sm font-medium">{t("admin.inviteLink")}</p>
          <div className="flex gap-2">
            <Input readOnly value={link} onFocus={(e) => e.target.select()} aria-label={t("admin.inviteLink")} />
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                void navigator.clipboard?.writeText(link).then(() => setCopied(true));
              }}
            >
              {copied ? t("admin.copied") : t("admin.copy")}
            </Button>
          </div>
        </div>
      )}

      <ul className="divide-y rounded-xl border bg-card">
        {invites.data?.invites.length === 0 && <li className="px-4 py-3 text-sm text-muted-foreground">{t("admin.noInvites")}</li>}
        {invites.data?.invites.map((inv) => {
          const state = inv.used_by ? t("admin.inviteUsed", { name: inv.used_by }) : inv.expired ? t("admin.inviteExpired") : t("admin.inviteActive");
          return (
            <li key={inv.id} className="flex flex-wrap items-center gap-3 px-4 py-3 text-sm">
              <div className="min-w-0 flex-1">
                <p className="font-medium">
                  {t(`auth.roleNames.${inv.role}`)} · {state}
                </p>
                <p className="text-muted-foreground">
                  {inv.created_by} · {relative(inv.created_at, i18n.resolvedLanguage, now)} · {t("admin.expires")}{" "}
                  {new Date(inv.expires_at).toLocaleDateString(i18n.resolvedLanguage)} {clock(inv.expires_at, i18n.resolvedLanguage)}
                </p>
              </div>
              {!inv.used_by && !inv.expired && (
                <Button size="sm" variant="ghost" onClick={() => revoke.mutate(inv.id)}>
                  {t("admin.revoke")}
                </Button>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}

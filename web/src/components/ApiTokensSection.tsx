import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { ApiError, tokens } from "@/api";
import { ConfirmButton } from "@/components/ConfirmButton";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { relative } from "@/lib/format";
import { useNow } from "@/lib/useLive";

const KEY = ["my-tokens"];

/** Personal API tokens, for the Home Assistant integration and scripts. */
export function ApiTokensSection() {
  const { t, i18n } = useTranslation();
  const now = useNow(30_000);
  const qc = useQueryClient();
  const list = useQuery({ queryKey: KEY, queryFn: ({ signal }) => tokens.list(signal) });
  const [name, setName] = useState("Home Assistant");
  const [created, setCreated] = useState<string>();
  const [copied, setCopied] = useState(false);
  const create = useMutation({
    mutationFn: () => tokens.create(name),
    onSuccess: (res) => {
      setCreated(res.token);
      setCopied(false);
      void qc.invalidateQueries({ queryKey: KEY });
    },
  });
  const revoke = useMutation({ mutationFn: tokens.revoke, onSuccess: () => qc.invalidateQueries({ queryKey: KEY }) });
  const err = create.error instanceof ApiError ? create.error.message : undefined;

  return (
    <section className="flex flex-col gap-3" aria-labelledby="api-tokens">
      <div>
        <h2 id="api-tokens" className="font-medium">
          {t("tokens.title")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("tokens.intro")}</p>
      </div>
      <form
        className="flex flex-wrap items-end gap-3 rounded-xl border bg-card p-4"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <div className="min-w-48 flex-1">
          <Field label={t("tokens.name")} error={err}>
            <Input required maxLength={64} value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
        </div>
        <Button type="submit" disabled={create.isPending}>
          {t("tokens.create")}
        </Button>
      </form>

      {created && (
        <div className="flex flex-col gap-2 rounded-xl border border-amber-300 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-950">
          <p className="text-sm font-medium">{t("tokens.copyNow")}</p>
          <div className="flex gap-2">
            <Input readOnly value={created} onFocus={(e) => e.target.select()} aria-label={t("tokens.newToken")} className="font-mono" />
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                void navigator.clipboard?.writeText(created).then(() => setCopied(true));
              }}
            >
              {copied ? t("admin.copied") : t("admin.copy")}
            </Button>
          </div>
        </div>
      )}

      {list.data && list.data.tokens.length > 0 && (
        <ul className="divide-y rounded-xl border bg-card" aria-label={t("tokens.title")}>
          {list.data.tokens.map((tok) => (
            <li key={tok.id} className="flex items-center justify-between gap-3 px-4 py-3 text-sm">
              <div className="min-w-0">
                <p className="truncate font-medium">{tok.name}</p>
                <p className="truncate text-muted-foreground">
                  <span className="font-mono">{tok.prefix}…</span> ·{" "}
                  {tok.last_used_at ? t("tokens.lastUsed", { when: relative(tok.last_used_at, i18n.resolvedLanguage, now) }) : t("tokens.neverUsed")}
                </p>
              </div>
              <ConfirmButton
                variant="ghost"
                size="sm"
                onConfirm={() => revoke.mutate(tok.id)}
                question={t("tokens.revokeQuestion")}
                aria-label={t("tokens.revokeNamed", { name: tok.name })}
              >
                {t("tokens.revoke")}
              </ConfirmButton>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

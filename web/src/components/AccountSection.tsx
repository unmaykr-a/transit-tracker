import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Shield } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { ApiError, changePassword, fetchMySessions, logout, revokeMySession, updateMe, type User } from "@/api";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { relative } from "@/lib/format";
import { SESSION_KEY } from "@/lib/session";


/** Short device description from a User-Agent string. */
function device(ua: string): string {
  const browser = /Firefox\//.test(ua) ? "Firefox" : /Edg\//.test(ua) ? "Edge" : /Chrome\//.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : "";
  const os = /iPhone|iPad/.test(ua) ? "iOS" : /Android/.test(ua) ? "Android" : /Mac OS X/.test(ua) ? "macOS" : /Windows/.test(ua) ? "Windows" : /Linux/.test(ua) ? "Linux" : "";
  return [browser, os].filter(Boolean).join(" · ") || ua.slice(0, 40) || "—";
}

export function AccountSection({ user }: { user: User }) {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const [email, setEmail] = useState(user.email);
  const [pw, setPw] = useState({ current: "", next: "" });
  const profile = useMutation({ mutationFn: () => updateMe({ email }), onSuccess: () => qc.invalidateQueries({ queryKey: SESSION_KEY }) });
  const password = useMutation({
    mutationFn: () => changePassword(pw.current, pw.next),
    onSuccess: () => {
      setPw({ current: "", next: "" });
      void qc.invalidateQueries({ queryKey: ["my-sessions"] });
    },
  });
  const sessions = useQuery({ queryKey: ["my-sessions"], queryFn: ({ signal }) => fetchMySessions(signal) });
  const revoke = useMutation({ mutationFn: revokeMySession, onSuccess: () => qc.invalidateQueries({ queryKey: ["my-sessions"] }) });
  const out = useMutation({ mutationFn: logout, onSuccess: () => qc.invalidateQueries() });
  const pwErr = (f: string) => (password.error instanceof ApiError && password.error.field === f ? password.error.message : undefined);

  return (
    <section className="flex flex-col gap-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h2 className="font-medium">{t("account.title")}</h2>
          <p className="text-sm text-muted-foreground">{t("account.signedInAs", { name: user.username })}</p>
        </div>
        <div className="flex gap-2">
          {user.role === "admin" && (
            <Button variant="outline" size="sm" asChild>
              <Link to="/admin/users">
                <Shield className="size-4" aria-hidden />
                {t("account.admin")}
              </Link>
            </Button>
          )}
          <Button variant="outline" size="sm" onClick={() => out.mutate()}>
            {t("auth.logout")}
          </Button>
        </div>
      </div>

      <form
        className="flex flex-col gap-3 rounded-xl border bg-card p-4"
        onSubmit={(e) => {
          e.preventDefault();
          profile.mutate();
        }}
      >
        <h3 className="text-sm font-medium">{t("account.profile")}</h3>
        <Field label={t("auth.email")}>
          <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
        </Field>
        <div className="flex items-center gap-3">
          <Button type="submit" size="sm" disabled={profile.isPending}>
            {t("account.save")}
          </Button>
          {profile.isSuccess && <span className="text-sm text-muted-foreground">{t("account.saved")}</span>}
        </div>
      </form>

      <form
        className="flex flex-col gap-3 rounded-xl border bg-card p-4"
        onSubmit={(e) => {
          e.preventDefault();
          password.mutate();
        }}
      >
        <h3 className="text-sm font-medium">{user.has_password ? t("account.password") : t("account.setPassword")}</h3>
        {user.has_password ? (
          <Field label={t("auth.currentPassword")} error={pwErr("current_password")}>
            <Input type="password" autoComplete="current-password" required value={pw.current} onChange={(e) => setPw({ ...pw, current: e.target.value })} />
          </Field>
        ) : (
          <p className="text-sm text-muted-foreground">{t("account.setPasswordHint")}</p>
        )}
        <Field label={t("auth.newPassword")} hint={t("auth.passwordHint")} error={pwErr("password")}>
          <Input type="password" autoComplete="new-password" required value={pw.next} onChange={(e) => setPw({ ...pw, next: e.target.value })} />
        </Field>
        <div className="flex items-center gap-3">
          <Button type="submit" size="sm" disabled={password.isPending}>
            {user.has_password ? t("account.password") : t("account.setPassword")}
          </Button>
          {password.isSuccess && <span className="text-sm text-muted-foreground">{t("account.passwordChanged")}</span>}
        </div>
      </form>

      <div className="flex flex-col gap-2">
        <h3 className="text-sm font-medium">{t("account.sessions")}</h3>
        <ul className="divide-y rounded-xl border bg-card">
          {sessions.data?.sessions.map((s) => (
            <li key={s.id} className="flex items-center justify-between gap-3 px-4 py-3 text-sm">
              <div className="min-w-0">
                <p className="truncate font-medium">{s.current ? t("account.thisDevice") : device(s.user_agent)}</p>
                <p className="truncate text-muted-foreground">
                  {s.ip} · {t("account.lastSeen", { when: relative(s.last_seen_at, i18n.resolvedLanguage) })}
                </p>
              </div>
              {!s.current && (
                <Button variant="ghost" size="sm" onClick={() => revoke.mutate(s.id)}>
                  {t("account.revoke")}
                </Button>
              )}
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}

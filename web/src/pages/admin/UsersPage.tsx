import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { admin, ApiError, type AdminUser, type Role } from "@/api";
import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/input";
import { useSession } from "@/lib/session";
import { cn } from "@/lib/utils";

import { ConfirmButton } from "@/components/ConfirmButton";

function errorText(e: unknown) {
  return e instanceof Error ? e.message : undefined;
}

function UserRow({ u, me }: { u: AdminUser; me: boolean }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [resetting, setResetting] = useState(false);
  const [pw, setPw] = useState("");
  const done = () => qc.invalidateQueries({ queryKey: ["admin", "users"] });
  const update = useMutation({ mutationFn: (p: { role?: Role; disabled?: boolean }) => admin.updateUser(u.id, p), onSuccess: done });
  const reset = useMutation({
    mutationFn: () => admin.resetPassword(u.id, pw),
    onSuccess: () => {
      setResetting(false);
      setPw("");
    },
  });
  const revoke = useMutation({ mutationFn: () => admin.revokeSessions(u.id), onSuccess: done });
  const del = useMutation({ mutationFn: () => admin.deleteUser(u.id), onSuccess: done });
  const err = errorText(update.error ?? reset.error ?? revoke.error ?? del.error);

  return (
    <li className="flex flex-col gap-3 px-4 py-3" aria-label={u.username}>
      <div className="flex flex-wrap items-center gap-3">
        <div className="min-w-0 flex-1">
          <p className="font-medium">
            {u.username}
            {me && <span className="text-muted-foreground"> ({t("admin.you")})</span>}
          </p>
          <p className="text-sm text-muted-foreground">
            {u.email || "—"} · {t("admin.sessions", { count: u.sessions })}
          </p>
        </div>
        <span
          className={cn(
            "rounded-full px-2 py-0.5 text-xs font-medium",
            u.disabled ? "bg-muted text-muted-foreground" : "bg-emerald-100 text-emerald-900 dark:bg-emerald-950 dark:text-emerald-100",
          )}
        >
          {u.disabled ? t("admin.disabled") : t("admin.active")}
        </span>
        <label className="sr-only" htmlFor={`role-${u.id}`}>
          {t("admin.role")}
        </label>
        {/* Your own role is changed by another admin, so nobody locks themselves out of this page by accident. */}
        <Select
          id={`role-${u.id}`}
          value={u.role}
          disabled={me}
          onChange={(e) => update.mutate({ role: e.target.value as Role })}
          className="h-8"
        >
          <option value="user">{t("auth.roles.user")}</option>
          <option value="admin">{t("auth.roles.admin")}</option>
        </Select>
      </div>
      <div className="flex flex-wrap gap-2">
        {!me && (
          <Button size="sm" variant="outline" onClick={() => update.mutate({ disabled: !u.disabled })}>
            {u.disabled ? t("admin.enable") : t("admin.disable")}
          </Button>
        )}
        <Button size="sm" variant="outline" onClick={() => setResetting((v) => !v)}>
          {t("admin.resetPassword")}
        </Button>
        {u.sessions > 0 && (
          <Button size="sm" variant="outline" onClick={() => revoke.mutate()}>
            {t("admin.revokeSessions")}
          </Button>
        )}
        {!me && (
          <ConfirmButton size="sm" variant="ghost" question={t("admin.confirmDelete", { name: u.username })} onConfirm={() => del.mutate()}>
            {t("admin.delete")}
          </ConfirmButton>
        )}
      </div>
      {resetting && (
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            reset.mutate();
          }}
        >
          <div className="min-w-48 flex-1">
            <Field label={t("auth.newPassword")} hint={`${t("auth.passwordHint")} ${t("admin.resetRevokes")}`}>
              <Input type="password" autoComplete="new-password" value={pw} onChange={(e) => setPw(e.target.value)} required />
            </Field>
          </div>
          <Button type="submit" size="sm">
            {t("admin.setPassword")}
          </Button>
        </form>
      )}
      {err && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {err}
        </p>
      )}
    </li>
  );
}

export default function UsersPage() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const me = useSession().data?.user;
  const users = useQuery({ queryKey: ["admin", "users"], queryFn: ({ signal }) => admin.users(signal) });
  const [form, setForm] = useState({ username: "", email: "", password: "", role: "user" as Role });
  const create = useMutation({
    mutationFn: () => admin.createUser(form),
    onSuccess: () => {
      setForm({ username: "", email: "", password: "", role: "user" });
      void qc.invalidateQueries({ queryKey: ["admin", "users"] });
    },
  });
  const fieldErr = (f: string) => (create.error instanceof ApiError && create.error.field === f ? create.error.message : undefined);

  return (
    <div className="flex flex-col gap-6">
      <ul className="divide-y rounded-xl border bg-card">
        {users.data?.users.map((u) => <UserRow key={u.id} u={u} me={u.id === me?.id} />)}
      </ul>
      <form
        className="grid gap-3 rounded-xl border bg-card p-4 sm:grid-cols-2"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <h2 className="font-medium sm:col-span-2">{t("admin.createUser")}</h2>
        <Field label={t("auth.username")} error={fieldErr("username")}>
          <Input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} required autoComplete="off" />
        </Field>
        <Field label={t("auth.email")}>
          <Input type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} autoComplete="off" />
        </Field>
        <Field label={t("auth.password")} hint={t("auth.passwordHint")} error={fieldErr("password")}>
          <Input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required autoComplete="new-password" />
        </Field>
        <Field label={t("admin.role")}>
          <Select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value as Role })}>
            <option value="user">{t("auth.roles.user")}</option>
            <option value="admin">{t("auth.roles.admin")}</option>
          </Select>
        </Field>
        <div className="sm:col-span-2">
          <Button type="submit" disabled={create.isPending}>
            {t("admin.createUser")}
          </Button>
        </div>
      </form>
    </div>
  );
}

import { useMutation, useQuery } from "@tanstack/react-query";
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { ApiError, checkInvite, login, setup, signup, type Credentials } from "@/api";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { useOnAuthenticated, useSession } from "@/lib/session";

function errorFor(err: unknown, field: string): string | undefined {
  return err instanceof ApiError && err.field === field ? err.message : undefined;
}

function generalError(err: unknown): string | undefined {
  if (!err) return undefined;
  if (err instanceof ApiError && err.field && err.field !== "invite") return undefined; // shown next to the field
  return err instanceof Error ? err.message : String(err);
}

function AuthCard({ title, intro, children }: { title: string; intro?: string; children: React.ReactNode }) {
  return (
    <div className="mx-auto flex max-w-sm flex-col gap-6 px-4 py-12">
      <header className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        {intro && <p className="text-muted-foreground">{intro}</p>}
      </header>
      {children}
    </div>
  );
}

function CredentialsForm({
  submit,
  button,
  withEmail,
  invite,
}: {
  submit: (c: Credentials) => Promise<{ user: unknown; csrf: string }>;
  button: string;
  withEmail?: boolean;
  invite?: string;
}) {
  const { t } = useTranslation();
  const onAuthenticated = useOnAuthenticated();
  const navigate = useNavigate();
  const next = new URLSearchParams(globalThis.location?.search).get("next");
  const [c, setC] = useState<Credentials>({ username: "", password: "", email: "" });
  const m = useMutation({
    mutationFn: () => submit({ ...c, invite }),
    onSuccess: (res) => {
      onAuthenticated(res as never);
      void navigate({ to: next && next.startsWith("/") ? next : "/" });
    },
  });
  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault();
        m.mutate();
      }}
    >
      <Field label={t("auth.username")} error={errorFor(m.error, "username")}>
        <Input autoComplete="username" required value={c.username} onChange={(e) => setC({ ...c, username: e.target.value })} />
      </Field>
      {withEmail && (
        <Field label={t("auth.email")}>
          <Input type="email" autoComplete="email" value={c.email} onChange={(e) => setC({ ...c, email: e.target.value })} />
        </Field>
      )}
      <Field label={t("auth.password")} hint={withEmail ? t("auth.passwordHint") : undefined} error={errorFor(m.error, "password")}>
        <Input
          type="password"
          autoComplete={withEmail ? "new-password" : "current-password"}
          required
          value={c.password}
          onChange={(e) => setC({ ...c, password: e.target.value })}
        />
      </Field>
      {generalError(m.error) && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {generalError(m.error)}
        </p>
      )}
      <Button type="submit" disabled={m.isPending}>
        {button}
      </Button>
    </form>
  );
}

export function LoginPage() {
  const { t } = useTranslation();
  const session = useSession();
  const name = session.data?.settings.instance_name ?? "Tram";
  const canSignup = session.data?.settings.signup_mode === "open";
  return (
    <AuthCard title={t("auth.loginTitle", { name })}>
      <CredentialsForm submit={login} button={t("auth.login")} />
      {canSignup && (
        <p className="text-sm text-muted-foreground">
          {t("auth.noAccount")}{" "}
          <Link to="/signup" className="font-medium text-foreground underline">
            {t("auth.signup")}
          </Link>
        </p>
      )}
    </AuthCard>
  );
}

export function SetupPage() {
  const { t } = useTranslation();
  const session = useSession();
  const name = session.data?.settings.instance_name ?? "Tram";
  return (
    <AuthCard title={t("auth.setupTitle", { name })} intro={t("auth.setupIntro")}>
      <CredentialsForm submit={setup} button={t("auth.setupButton")} withEmail />
    </AuthCard>
  );
}

const signupRoute = getRouteApi("/signup");

export function SignupPage() {
  const { t } = useTranslation();
  const session = useSession();
  const { invite } = signupRoute.useSearch();
  const name = session.data?.settings.instance_name ?? "Tram";
  const check = useQuery({ queryKey: ["invite", invite], queryFn: () => checkInvite(invite!), enabled: !!invite, retry: false });
  const open = session.data?.settings.signup_mode === "open";

  let body: React.ReactNode;
  if (invite && check.isError) body = <p className="text-muted-foreground">{t("auth.inviteInvalid")}</p>;
  else if (!invite && !open) body = <p className="text-muted-foreground">{t("auth.signupClosed")}</p>;
  else if (invite && !check.data) body = <p className="text-muted-foreground">{t("common.loading")}</p>;
  else body = <CredentialsForm submit={signup} button={t("auth.signup")} withEmail invite={invite} />;

  return (
    <AuthCard title={t("auth.signupTitle", { name })} intro={check.data ? t("auth.inviteFor", { role: t(`auth.roles.${check.data.role}`) }) : undefined}>
      {body}
      <p className="text-sm text-muted-foreground">
        {t("auth.haveAccount")}{" "}
        <Link to="/login" className="font-medium text-foreground underline">
          {t("auth.login")}
        </Link>
      </p>
    </AuthCard>
  );
}

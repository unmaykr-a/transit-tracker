import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";

import { fetchSession, setCsrfToken, type SessionInfo, type User } from "@/api";
import i18n from "@/i18n";

import { setThemePref, type ThemePref } from "./theme";

export const SESSION_KEY = ["session"] as const;

/** The bootstrap query: public settings, setup state and the logged-in user. */
export function useSession() {
  const q = useQuery({ queryKey: SESSION_KEY, queryFn: ({ signal }) => fetchSession(signal), staleTime: 5 * 60_000 });
  useEffect(() => {
    if (q.data) setCsrfToken(q.data.csrf);
  }, [q.data]);
  useEffect(() => {
    if (q.data?.settings.instance_name) document.title = q.data.settings.instance_name;
  }, [q.data?.settings.instance_name]);
  return q;
}

/** Applies a user's saved language and theme after login. */
export function applyUserPrefs(u: User | null | undefined) {
  if (!u) return;
  if (u.locale && u.locale !== i18n.resolvedLanguage) void i18n.changeLanguage(u.locale);
  if (u.theme) setThemePref(u.theme as ThemePref);
}

/** After login/signup/setup: store CSRF, apply prefs, refresh everything user-specific. */
export function useOnAuthenticated() {
  const qc = useQueryClient();
  return (res: { user: User; csrf: string }) => {
    setCsrfToken(res.csrf);
    applyUserPrefs(res.user);
    qc.setQueryData<SessionInfo>(SESSION_KEY, (old) => (old ? { ...old, user: res.user, csrf: res.csrf, setup_required: false } : old));
    void qc.invalidateQueries();
  };
}

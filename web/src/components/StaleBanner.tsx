import { useTranslation } from "react-i18next";

import { staleFeeds } from "@/lib/liveState";
import { useConnectionStatus, useLiveState } from "@/lib/useLive";
import { cn } from "@/lib/utils";

/** Shown when a feed has had no fresh data for 60 s or the live connection is down. */
export function StaleBanner({ className }: { className?: string }) {
  const { t } = useTranslation();
  const state = useLiveState(2000);
  const status = useConnectionStatus();
  const stale = staleFeeds(state.feeds);
  let text: string | undefined;
  if (status === "reconnecting") text = t("live.reconnecting");
  else if (stale.length > 0 && state.version > 0)
    text = t("live.delayed", { feeds: stale.map((f) => t(`live.feed.${f.name}`, { defaultValue: f.name })).join(", ") });
  if (!text) return null;
  return (
    <div
      role="status"
      className={cn(
        "rounded-full bg-amber-100 px-3 py-1.5 text-sm font-medium text-amber-900 shadow-sm dark:bg-amber-950 dark:text-amber-100",
        className,
      )}
    >
      {text}
    </div>
  );
}

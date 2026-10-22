import { useTranslation } from "react-i18next";

import type { VerdictLevel } from "@/api";
import { cn } from "@/lib/utils";

const styles: Record<Exclude<VerdictLevel, "none">, string> = {
  neutral: "bg-muted text-foreground",
  green: "bg-emerald-100 text-emerald-900 dark:bg-emerald-950 dark:text-emerald-100",
  amber: "bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-100",
  red: "bg-red-100 text-red-900 dark:bg-red-950 dark:text-red-100",
};

/** The can-I-make-it chip (PLAN.md verdict table). nextIn: minutes until the next catchable departure. */
export function VerdictChip({ level, slack, nextIn }: { level: VerdictLevel; slack: number; nextIn?: number }) {
  const { t } = useTranslation();
  if (level === "none") return null;
  const text =
    level === "red"
      ? nextIn !== undefined
        ? t("verdict.tooTight", { min: nextIn })
        : t("verdict.tooTightNone")
      : level === "amber"
        ? t("verdict.leaveNow")
        : t("verdict.leaveIn", { min: slack });
  return <span className={cn("inline-flex items-center rounded-full px-2.5 py-1 text-sm font-medium whitespace-nowrap", styles[level])}>{text}</span>;
}

import { MODE_BG } from "@/lib/modeColors";
import type { Mode } from "@/lib/types";
import { cn } from "@/lib/utils";

export function ModePill({ mode, children, className }: { mode: Mode; children: React.ReactNode; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex h-7 min-w-9 items-center justify-center rounded-full px-2.5 text-sm font-semibold text-white tabular-nums",
        MODE_BG[mode],
        className,
      )}
    >
      {children}
    </span>
  );
}

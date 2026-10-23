import { KIND_BG } from "@/lib/modeColors";
import type { Kind } from "@/lib/types";
import { cn } from "@/lib/utils";

export function ModePill({ mode, children, className }: { mode: Kind; children: React.ReactNode; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex h-7 min-w-9 items-center justify-center rounded-full px-2.5 text-sm font-semibold text-white tabular-nums",
        KIND_BG[mode],
        className,
      )}
    >
      {children}
    </span>
  );
}

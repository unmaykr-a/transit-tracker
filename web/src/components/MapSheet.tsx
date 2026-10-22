import { ChevronUp } from "lucide-react";
import { useId, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";

const wide = () => typeof window !== "undefined" && window.matchMedia?.("(min-width: 768px)").matches;

/**
 * Bottom sheet over the map. On phones it opens compact (header and a one-line
 * summary) so the map stays visible, and expands on demand; on wider screens it
 * is a side card and always open.
 */
export function MapSheet({ label, header, summary, children }: { label?: string; header: ReactNode; summary?: ReactNode; children: ReactNode }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(wide);
  const bodyId = useId();
  return (
    <section aria-label={label} className="pointer-events-auto mx-auto w-full max-w-md rounded-t-2xl border bg-card shadow-lg md:rounded-2xl">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        aria-controls={bodyId}
        aria-label={open ? t("map.sheetLess") : t("map.sheetMore")}
        className="flex w-full justify-center pt-2 pb-1 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring md:hidden"
      >
        <span className="h-1 w-10 rounded-full bg-muted-foreground/40" aria-hidden />
      </button>
      <div className="px-4 pb-4 md:pt-4">
        {header}
        {!open && summary && (
          <button
            type="button"
            onClick={() => setOpen(true)}
            className="mt-2 flex w-full items-center gap-2 text-left text-sm"
            aria-controls={bodyId}
          >
            <span className="min-w-0 flex-1 truncate">{summary}</span>
            <ChevronUp className="size-4 shrink-0 text-muted-foreground" aria-hidden />
          </button>
        )}
        <div id={bodyId} hidden={!open} className={cn("max-h-[45vh] overflow-y-auto md:max-h-none")}>
          {children}
        </div>
      </div>
    </section>
  );
}

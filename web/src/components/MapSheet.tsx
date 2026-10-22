import { ChevronUp } from "lucide-react";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";

const wide = () => typeof window !== "undefined" && window.matchMedia?.("(min-width: 768px)").matches;

/**
 * Bottom sheet over the map. On phones it opens compact (header and a one-line
 * summary) so the map stays visible, and expands on demand; on wider screens it
 * is a side card and always open.
 */
export function MapSheet({
  label,
  header,
  summary,
  children,
  onClose,
}: {
  label?: string;
  header: ReactNode;
  summary?: ReactNode;
  children: ReactNode;
  onClose?: () => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(wide);
  const bodyId = useId();
  // Escape closes the sheet (wherever focus is, except while typing in a field).
  const close = useRef(onClose);
  useEffect(() => {
    close.current = onClose;
  });
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented) return;
      if (e.target instanceof HTMLElement && e.target.closest("input, textarea, select")) return;
      close.current?.();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);
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
            className="mt-2 flex w-full items-center gap-2 text-left text-sm md:hidden"
            aria-controls={bodyId}
          >
            <span className="min-w-0 flex-1 truncate">{summary}</span>
            <ChevronUp className="size-4 shrink-0 text-muted-foreground" aria-hidden />
          </button>
        )}
        {/* Always shown from md up, also when a narrow window that collapsed it is widened. */}
        <div id={bodyId} className={cn("max-h-[45vh] overflow-y-auto md:block md:max-h-none", !open && "hidden")}>
          {children}
        </div>
      </div>
    </section>
  );
}

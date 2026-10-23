import { ChevronUp } from "lucide-react";
import { useEffect, useId, useRef, useState, type PointerEvent as ReactPointerEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";

const wide = () => typeof window !== "undefined" && window.matchMedia?.("(min-width: 768px)").matches;

/** Drags shorter than this are taps; longer ones decide (px). */
const DRAG_SLOP = 8;
const DRAG_DECIDE = 56;

/**
 * Bottom sheet over the map. On phones it opens compact (header and a one-line
 * summary) so the map stays visible, and expands on demand; on wider screens it
 * is a side card and always open. On phones the handle and header can be
 * dragged: up opens it, down makes an open sheet compact and closes a compact one.
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
  // Dragging moves the sheet directly (no re-render per move); release decides.
  const sheet = useRef<HTMLElement>(null);
  const drag = useRef<{ id: number; y0: number; dy: number } | null>(null);
  const onPointerDown = (e: ReactPointerEvent) => {
    if (wide() || e.button !== 0) return;
    if (e.target instanceof HTMLElement && e.target.closest("a, input, select, textarea, [data-no-drag]")) return;
    drag.current = { id: e.pointerId, y0: e.clientY, dy: 0 };
  };
  const onPointerMove = (e: ReactPointerEvent) => {
    const d = drag.current;
    if (!d || d.id !== e.pointerId) return;
    d.dy = e.clientY - d.y0;
    if (Math.abs(d.dy) < DRAG_SLOP) return;
    if (!(e.currentTarget as HTMLElement).hasPointerCapture(e.pointerId)) (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    // Follows the finger downwards; upwards only a little (it opens on release).
    const y = d.dy > 0 ? d.dy : Math.max(d.dy, -24);
    if (sheet.current) Object.assign(sheet.current.style, { transition: "none", transform: `translateY(${y}px)` });
  };
  const endDrag = (e: ReactPointerEvent, cancelled = false) => {
    const d = drag.current;
    if (!d || d.id !== e.pointerId) return;
    drag.current = null;
    // Where the finger left, not the last move seen: moves are coalesced, a quick flick may show few.
    if (!cancelled) d.dy = e.clientY - d.y0;
    const el = sheet.current;
    const decided = !cancelled && (d.dy <= -DRAG_DECIDE / 2 || d.dy >= DRAG_DECIDE);
    if (el) {
      // A drag that changes the sheet lands at once (its size changes too); one that does not springs back.
      Object.assign(el.style, { transition: decided ? "none" : "", transform: "" });
      if (decided) requestAnimationFrame(() => requestAnimationFrame(() => (el.style.transition = "")));
    }
    if (!decided) return;
    // A real drag is not also a tap on whatever it started on.
    const swallow = (ev: Event) => ev.stopPropagation();
    window.addEventListener("click", swallow, { capture: true, once: true });
    setTimeout(() => window.removeEventListener("click", swallow, { capture: true }), 0);
    if (d.dy < 0) setOpen(true);
    else if (open) setOpen(false);
    else onClose?.();
  };
  const dragProps = {
    onPointerDown,
    onPointerMove,
    onPointerUp: (e: ReactPointerEvent) => endDrag(e),
    onPointerCancel: (e: ReactPointerEvent) => endDrag(e, true),
  };
  return (
    <section
      ref={sheet}
      data-map-cover
      aria-label={label}
      className="pointer-events-auto mx-auto w-full max-w-md rounded-t-2xl border bg-card shadow-lg transition-transform duration-150 md:rounded-2xl"
    >
      {/* The handle and header are the drag area on phones (touch-none: the drag, not the page, moves). */}
      <div {...dragProps} className="touch-none md:touch-auto">
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
        <div className="px-4 md:pt-4">{header}</div>
      </div>
      <div className="px-4 pb-4">
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

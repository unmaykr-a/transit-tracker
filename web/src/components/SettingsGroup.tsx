import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

/** One topic on the Settings page: a heading with an icon, optional actions, then its parts. */
export function SettingsGroup({ id, icon: Icon, title, description, actions, children }: {
  id: string;
  icon: LucideIcon;
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section id={id} aria-labelledby={`${id}-title`} className="flex scroll-mt-16 flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b pb-3">
        <div className="min-w-0">
          <h2 id={`${id}-title`} className="flex items-center gap-2 text-lg font-semibold tracking-tight">
            <Icon className="size-5 text-muted-foreground" aria-hidden />
            {title}
          </h2>
          {description && <p className="mt-0.5 text-sm text-muted-foreground">{description}</p>}
        </div>
        {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
      </div>
      {children}
    </section>
  );
}

/** A labelled row inside a group: the label and hint on the left, the control on the right (stacked on phones). */
export function SettingRow({ id, label, hint, children }: { id: string; label: string; hint?: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-6">
      <div className="min-w-0">
        <h3 id={id} className="text-sm font-medium">
          {label}
        </h3>
        {hint && <p className="text-sm text-muted-foreground">{hint}</p>}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  );
}

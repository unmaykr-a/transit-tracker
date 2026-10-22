import * as React from "react";

import { cn } from "@/lib/utils";

export function Input({ className, ...props }: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={cn(
        "h-10 w-full rounded-md border bg-background px-3 text-base shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 aria-invalid:border-red-500 md:text-sm",
        className,
      )}
      {...props}
    />
  );
}

export function Select({ className, ...props }: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      className={cn(
        "h-10 rounded-md border bg-background px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        className,
      )}
      {...props}
    />
  );
}

/** A labelled form control with an optional hint and error message. */
export function Field({ label, hint, error, children }: { label: string; hint?: string; error?: string; children: React.ReactNode }) {
  const noteId = React.useId();
  const note = error || hint;
  // The control announces its hint or error, and is marked invalid while the error shows.
  const control =
    note && React.isValidElement<React.InputHTMLAttributes<HTMLElement>>(children)
      ? React.cloneElement(children, { "aria-describedby": noteId, "aria-invalid": error ? true : children.props["aria-invalid"] })
      : children;
  return (
    <label className="flex flex-col gap-1.5 text-sm">
      <span className="font-medium">{label}</span>
      {control}
      {error ? (
        <span id={noteId} role="alert" className="text-red-600 dark:text-red-400">
          {error}
        </span>
      ) : (
        hint && (
          <span id={noteId} className="text-muted-foreground">
            {hint}
          </span>
        )
      )}
    </label>
  );
}

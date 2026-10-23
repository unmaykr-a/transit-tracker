import { cn } from "@/lib/utils";

/** An on/off switch; label it with aria-labelledby or aria-label. */
export function Switch({ checked, onChange, className, ...props }: Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, "onChange"> & {
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className={cn(
        "relative inline-flex h-6 w-11 shrink-0 items-center rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
        checked ? "bg-foreground" : "bg-muted-foreground/40",
        className,
      )}
      {...props}
    >
      <span
        aria-hidden
        className={cn("block size-5 rounded-full bg-background shadow transition-transform", checked ? "translate-x-5" : "translate-x-0")}
      />
    </button>
  );
}

import { Link } from "@tanstack/react-router";

import type { Mode } from "@/lib/types";
import { cn } from "@/lib/utils";

import { ModePill } from "./ModePill";

/** A line number in its mode colour, optionally linking to the line page. */
export function LinePill({ mode, line, link = false, className }: { mode: Mode; line: string; link?: boolean; className?: string }) {
  const pill = <ModePill mode={mode} className={className}>{line}</ModePill>;
  if (!link) return pill;
  return (
    <Link
      to="/lines/$mode/$line"
      params={{ mode, line }}
      className={cn("rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring")}
    >
      {pill}
    </Link>
  );
}

import { Link } from "@tanstack/react-router";

import { displayLine } from "@/lib/lines";
import type { Kind, Mode } from "@/lib/types";
import { cn } from "@/lib/utils";

import { ModePill } from "./ModePill";

/** A line number in its mode colour, optionally linking to the line page. */
export function LinePill({ mode, kind, line, link = false, className }: { mode: Mode; kind?: Kind; line: string; link?: boolean; className?: string }) {
  const pill = <ModePill mode={kind ?? mode} className={className}>{displayLine(line)}</ModePill>;
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

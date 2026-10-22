import type { ReactNode } from "react";

import { useWide } from "@/lib/useWide";

/**
 * Details beside a large live map on wide screens; just the details (which
 * bring their own small map) on phones and tablets.
 */
export function SplitLayout({ children, map }: { children: ReactNode; map: ReactNode }) {
  const wide = useWide();
  if (!wide) return <>{children}</>;
  return (
    <div className="flex h-full">
      <div className="w-[30rem] shrink-0 overflow-y-auto border-r">{children}</div>
      <div className="relative min-w-0 flex-1">{map}</div>
    </div>
  );
}

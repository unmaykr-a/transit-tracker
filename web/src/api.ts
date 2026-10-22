export interface Health {
  status: string;
  version: string;
  commit: string;
  uptime_s: number;
}

export async function fetchHealth(signal?: AbortSignal): Promise<Health> {
  const res = await fetch("/healthz", { signal });
  if (!res.ok) throw new Error(`healthz: ${res.status}`);
  return (await res.json()) as Health;
}

import { useEffect, useState } from "react";

import { fetchHealth, type Health } from "@/api";
import { Button } from "@/components/ui/button";

const modes = [
  { name: "Tram", className: "bg-tram" },
  { name: "Bus", className: "bg-bus" },
  { name: "Trolley", className: "bg-trolley" },
  { name: "Train", className: "bg-train" },
] as const;

type State = { kind: "loading" } | { kind: "ok"; health: Health } | { kind: "error" };

export default function App() {
  const [state, setState] = useState<State>({ kind: "loading" });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const ctrl = new AbortController();
    fetchHealth(ctrl.signal)
      .then((health) => setState({ kind: "ok", health }))
      .catch((err: unknown) => {
        if (!ctrl.signal.aborted) {
          console.error(err);
          setState({ kind: "error" });
        }
      });
    return () => ctrl.abort();
  }, [attempt]);

  return (
    <main className="mx-auto flex min-h-dvh max-w-md flex-col justify-center gap-8 px-4 py-12">
      <header className="flex flex-col gap-2">
        <h1 className="text-3xl font-semibold tracking-tight">Tram</h1>
        <p className="text-muted-foreground">Live Tallinn buses, trams, trolleybuses and Elron trains.</p>
      </header>

      <ul className="flex flex-wrap gap-2" aria-label="Transport modes">
        {modes.map((m) => (
          <li key={m.name} className={`${m.className} rounded-full px-3 py-1 text-sm font-medium text-white`}>
            {m.name}
          </li>
        ))}
      </ul>

      <section className="rounded-xl border bg-card p-4" aria-live="polite">
        {state.kind === "loading" && <p className="text-muted-foreground">Checking server…</p>}
        {state.kind === "ok" && (
          <p>
            Server <span className="font-medium">{state.health.status}</span>
            <span className="text-muted-foreground"> · {state.health.version}</span>
          </p>
        )}
        {state.kind === "error" && (
          <div className="flex items-center justify-between gap-4">
            <p>Server unreachable.</p>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setState({ kind: "loading" });
                setAttempt((n) => n + 1);
              }}
            >
              Retry
            </Button>
          </div>
        )}
      </section>
    </main>
  );
}

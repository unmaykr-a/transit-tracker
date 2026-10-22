import { describe, expect, it } from "vitest";

import { LiveClient } from "./liveClient";

class FakeWS {
  static instances: FakeWS[] = [];
  onopen?: () => void;
  onmessage?: (e: { data: string }) => void;
  onclose?: () => void;
  closed = false;
  constructor(public url: string) {
    FakeWS.instances.push(this);
  }
  close() {
    this.closed = true;
  }
}

class FakeES {
  static instances: FakeES[] = [];
  onopen?: () => void;
  onerror?: () => void;
  handlers = new Map<string, (e: { data: string }) => void>();
  constructor(public url: string) {
    FakeES.instances.push(this);
  }
  addEventListener(type: string, fn: (e: { data: string }) => void) {
    this.handlers.set(type, fn);
  }
  close() {}
}

function setup() {
  FakeWS.instances = [];
  FakeES.instances = [];
  const timers: (() => void)[] = [];
  const c = new LiveClient({
    wsUrl: "ws://x/ws",
    sseUrl: "/sse",
    WebSocketImpl: FakeWS as unknown as typeof WebSocket,
    EventSourceImpl: FakeES as unknown as typeof EventSource,
    setTimeout: (fn) => timers.push(fn),
    clearTimeout: () => {},
  });
  return { c, timers };
}

const snapshot = JSON.stringify({
  type: "snapshot", version: 1, server_time: "2026-10-22T09:00:00Z",
  vehicles: [{ id: "bus-1", mode: "bus", line: "17", label: "17", lat: 59.4, lon: 24.7, bearing: 0, speed_mps: 0, destination: "X", low_floor: false, updated_at: "2026-10-22T09:00:00Z" }],
});

describe("LiveClient", () => {
  it("connects lazily, applies messages and reconnects after a drop", () => {
    const { c, timers } = setup();
    expect(FakeWS.instances).toHaveLength(0);
    const seen: number[] = [];
    c.subscribe((a) => seen.push(a.state.vehicles.size));
    const ws = FakeWS.instances[0]!;
    ws.onopen!();
    expect(c.status).toBe("open");
    ws.onmessage!({ data: snapshot });
    ws.onmessage!({ data: "not json" });
    expect(seen).toEqual([1]);

    ws.onclose!();
    expect(c.status).toBe("reconnecting");
    timers.shift()!();
    expect(FakeWS.instances).toHaveLength(2);
    expect(c.transport).toBe("ws");
  });

  it("falls back to SSE when WebSockets never open", () => {
    const { c, timers } = setup();
    c.start();
    for (let i = 0; i < 3; i++) {
      FakeWS.instances.at(-1)!.onclose!();
      timers.shift()!();
    }
    expect(FakeWS.instances).toHaveLength(3);
    expect(FakeES.instances).toHaveLength(1);
    expect(c.transport).toBe("sse");
    let size = 0;
    c.subscribe((a) => (size = a.state.vehicles.size));
    FakeES.instances[0]!.handlers.get("snapshot")!({ data: snapshot });
    expect(size).toBe(1);
  });

  it("stops cleanly", () => {
    const { c } = setup();
    c.start();
    c.stop();
    expect(FakeWS.instances[0]!.closed).toBe(true);
    expect(c.transport).toBeUndefined();
  });

  it("closes when nobody listens or the tab is hidden, and reconnects on return", () => {
    const { c, timers } = setup();
    const doc = new EventTarget() as EventTarget & { visibilityState: string };
    doc.visibilityState = "visible";
    const win = new EventTarget();
    c.watchPage(doc as unknown as Document, win as unknown as Window);

    const off = c.subscribe(() => {});
    FakeWS.instances[0]!.onopen!();
    off();
    expect(timers).toHaveLength(1);
    const again = c.subscribe(() => {}); // back within the grace period: same connection
    timers.shift()!();
    expect(FakeWS.instances[0]!.closed).toBe(false);

    doc.visibilityState = "hidden";
    doc.dispatchEvent(new Event("visibilitychange"));
    timers.shift()!();
    expect(FakeWS.instances[0]!.closed).toBe(true);
    expect(c.transport).toBeUndefined();

    doc.visibilityState = "visible";
    doc.dispatchEvent(new Event("visibilitychange"));
    expect(FakeWS.instances).toHaveLength(2);
    again();
  });
});

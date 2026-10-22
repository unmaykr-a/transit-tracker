import { applyMessage, emptyLiveState, type Applied, type LiveState } from "./liveState";
import type { LiveMessage } from "./types";

export type ConnectionStatus = "connecting" | "open" | "reconnecting";

type Listener = (applied: Applied) => void;

export interface LiveClientOptions {
  wsUrl: string;
  sseUrl: string;
  /** Injected for tests. */
  WebSocketImpl?: typeof WebSocket;
  EventSourceImpl?: typeof EventSource;
  setTimeout?: (fn: () => void, ms: number) => unknown;
  clearTimeout?: (h: unknown) => void;
}

/** WebSocket attempts that never open before falling back to SSE (proxies without WS support). */
const WS_FAILURES_BEFORE_SSE = 3;
/** How long the stream stays open with nobody listening, or in a background tab. */
const IDLE_MS = 30_000;

/**
 * One shared connection to the server's live vehicle stream. Reconnects with
 * capped exponential backoff and falls back to Server-Sent Events when
 * WebSockets never manage to open.
 */
export class LiveClient {
  private state: LiveState = emptyLiveState();
  private listeners = new Set<Listener>();
  private statusListeners = new Set<() => void>();
  private ws?: WebSocket;
  private es?: EventSource;
  private retry = 0;
  private wsFailures = 0;
  private timer?: unknown;
  private started = false;
  private hidden = false;
  private idleTimer?: unknown;
  private _status: ConnectionStatus = "connecting";
  private readonly opts: Required<Omit<LiveClientOptions, "WebSocketImpl" | "EventSourceImpl">> &
    Pick<LiveClientOptions, "WebSocketImpl" | "EventSourceImpl">;

  constructor(opts: LiveClientOptions) {
    this.opts = {
      setTimeout: (fn, ms) => globalThis.setTimeout(fn, ms),
      clearTimeout: (h) => globalThis.clearTimeout(h as ReturnType<typeof setTimeout>),
      ...opts,
    };
  }

  get status(): ConnectionStatus {
    return this._status;
  }

  get transport(): "ws" | "sse" | undefined {
    return this.ws ? "ws" : this.es ? "sse" : undefined;
  }

  getState(): LiveState {
    return this.state;
  }

  /** Subscribes to applied messages; starts the connection on first use. */
  subscribe(fn: Listener): () => void {
    this.listeners.add(fn);
    this.wake();
    return () => {
      this.listeners.delete(fn);
      if (this.listeners.size === 0) this.idle();
    };
  }

  /** Follows the page: closes the stream a while after the tab is hidden, reconnects at once when it is back. */
  watchPage(doc: Document = document, win: Window = window) {
    doc.addEventListener("visibilitychange", () => {
      this.hidden = doc.visibilityState === "hidden";
      if (this.hidden) this.idle();
      else this.resume();
    });
    win.addEventListener("online", () => this.resume());
  }

  /** Connect now instead of waiting out the backoff, and give WebSockets another chance. */
  private resume() {
    this.wsFailures = 0;
    this.retry = 0;
    if (this._status === "reconnecting" && this.started) {
      if (this.timer !== undefined) this.opts.clearTimeout(this.timer);
      this.connect();
    }
    this.wake();
  }

  private wake() {
    if (this.listeners.size === 0 || this.hidden) return;
    if (this.idleTimer !== undefined) this.opts.clearTimeout(this.idleTimer);
    this.idleTimer = undefined;
    this.start();
  }

  private idle() {
    if (!this.started || this.idleTimer !== undefined) return;
    this.idleTimer = this.opts.setTimeout(() => {
      this.idleTimer = undefined;
      if (this.listeners.size === 0 || this.hidden) this.stop();
    }, IDLE_MS);
  }

  onStatus(fn: () => void): () => void {
    this.statusListeners.add(fn);
    return () => this.statusListeners.delete(fn);
  }

  start() {
    if (this.started) return;
    this.started = true;
    this.connect();
  }

  stop() {
    this.started = false;
    if (this.timer !== undefined) this.opts.clearTimeout(this.timer);
    this.ws?.close();
    this.es?.close();
    this.ws = this.es = undefined;
    this.setStatus("connecting");
  }

  private setStatus(s: ConnectionStatus) {
    if (s === this._status) return;
    this._status = s;
    this.statusListeners.forEach((fn) => fn());
  }

  private handle(data: string) {
    let msg: LiveMessage;
    try {
      msg = JSON.parse(data) as LiveMessage;
    } catch {
      return;
    }
    const applied = applyMessage(this.state, msg);
    this.state = applied.state;
    this.listeners.forEach((fn) => fn(applied));
  }

  private connect() {
    if (!this.started) return;
    const WS = this.opts.WebSocketImpl ?? globalThis.WebSocket;
    if (WS && this.wsFailures < WS_FAILURES_BEFORE_SSE) this.connectWs(WS);
    else this.connectSse();
  }

  private connectWs(WS: typeof WebSocket) {
    let opened = false;
    const ws = new WS(this.opts.wsUrl);
    this.ws = ws;
    ws.onopen = () => {
      opened = true;
      this.retry = 0;
      this.wsFailures = 0;
      this.setStatus("open");
    };
    ws.onmessage = (e: MessageEvent) => this.handle(String(e.data));
    ws.onclose = () => {
      if (this.ws !== ws) return;
      this.ws = undefined;
      if (!opened) this.wsFailures++;
      this.scheduleReconnect();
    };
  }

  private connectSse() {
    const ES = this.opts.EventSourceImpl ?? globalThis.EventSource;
    if (!ES) return;
    const es = new ES(this.opts.sseUrl);
    this.es = es;
    es.onopen = () => {
      this.retry = 0;
      this.setStatus("open");
    };
    for (const type of ["snapshot", "diff", "stale"]) {
      es.addEventListener(type, (e) => this.handle(String((e as MessageEvent).data)));
    }
    es.onerror = () => {
      // EventSource retries on its own, but we want our backoff and a fresh snapshot.
      es.close();
      if (this.es !== es) return;
      this.es = undefined;
      this.scheduleReconnect();
    };
  }

  private scheduleReconnect() {
    if (!this.started) return;
    this.setStatus("reconnecting");
    const delay = Math.min(30_000, 1_000 * 2 ** this.retry) * (0.8 + Math.random() * 0.4);
    this.retry++;
    this.timer = this.opts.setTimeout(() => this.connect(), delay);
  }
}

function defaultUrls() {
  const loc = globalThis.location;
  const proto = loc?.protocol === "https:" ? "wss:" : "ws:";
  return { wsUrl: `${proto}//${loc?.host}/ws`, sseUrl: "/api/v1/vehicles/stream" };
}

export const live = new LiveClient(defaultUrls());
if (typeof document !== "undefined") live.watchPage();

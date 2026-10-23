import { css, html, LitElement, nothing, type PropertyValues, type TemplateResult } from "lit";

import { translator, type Key } from "./i18n";
import { allBusEntities, clock, MODE_COLORS, view, type Level, type Mode, type NextAttributes } from "./model";
import type { HomeAssistant, BusCardConfig } from "./types";
import { BusCardEditor } from "./bus-card-editor";

// Replaced at build time with the release version (rollup.config.mjs).
const VERSION = "__BUS_VERSION__";

const DEFAULTS = { departures: 3, show_walk: true, show_verdict: true, layout: "full" as const };

export class BusCard extends LitElement {
  static properties = {
    hass: { attribute: false },
    _config: { state: true },
    _now: { state: true },
  };

  hass?: HomeAssistant;
  private _config?: BusCardConfig;
  private _now = new Date();
  private _timer?: ReturnType<typeof setInterval>;

  static getConfigElement(): HTMLElement {
    return document.createElement("bus-card-editor");
  }

  /** Added from the card picker: every saved line, nothing to fill in. */
  static getStubConfig(): Partial<BusCardConfig> {
    return { departures: DEFAULTS.departures, show_walk: true, show_verdict: true };
  }

  setConfig(config: BusCardConfig): void {
    if (!config) throw new Error("bus-card: missing configuration");
    if (config.entities !== undefined && !Array.isArray(config.entities)) {
      throw new Error("bus-card: `entities` must be a list of sensor.bus_*_next entities (or left out for all lines)");
    }
    const departures = Math.min(5, Math.max(1, Math.round(config.departures ?? DEFAULTS.departures)));
    this._config = { ...DEFAULTS, ...config, departures };
  }

  /** The configured lines, or every saved line when none are listed. */
  private get entities(): string[] {
    const listed = this._config?.entities ?? [];
    return listed.length > 0 ? listed : this.hass ? allBusEntities(this.hass.states, this.hass.entities) : [];
  }

  getCardSize(): number {
    const n = Math.max(1, this.entities.length);
    return this._config?.layout === "compact" ? n + (this._config.title ? 1 : 0) : 2 * n + (this._config?.title ? 1 : 0);
  }

  /** Sections dashboards: a sensible default footprint. */
  getGridOptions(): { columns: number; rows: number | "auto"; min_columns: number } {
    return { columns: 12, rows: "auto", min_columns: 6 };
  }

  connectedCallback(): void {
    super.connectedCallback();
    // Countdowns tick between the integration's polls.
    this._timer = setInterval(() => (this._now = new Date()), 15_000);
  }

  disconnectedCallback(): void {
    super.disconnectedCallback();
    clearInterval(this._timer);
  }

  /** Home Assistant sets hass on every state change anywhere; re-render only for this card's entities. */
  protected shouldUpdate(changed: PropertyValues): boolean {
    if (changed.size > 1 || !changed.has("hass")) return true;
    const old = changed.get("hass") as HomeAssistant | undefined;
    if (!old || !this.hass || old.locale !== this.hass.locale || old.themes?.darkMode !== this.hass.themes?.darkMode) return true;
    const ids = this.entities;
    const listed = (this._config?.entities ?? []).length > 0;
    if (!listed && ids.join() !== allBusEntities(old.states, old.entities).join()) return true;
    return ids.some((id) => old.states[id] !== this.hass!.states[id]);
  }

  protected willUpdate(changed: PropertyValues): void {
    if (changed.has("hass")) this._now = new Date();
  }

  private get t() {
    return translator(this.hass?.locale?.language ?? this.hass?.language);
  }

  protected render(): TemplateResult | typeof nothing {
    if (!this._config || !this.hass) return nothing;
    const c = this._config;
    const ids = this.entities;
    return html`
      <ha-card .header=${c.title || undefined} class=${this.hass.themes?.darkMode ? "dark" : ""}>
        <div class="lines ${c.layout}">
          ${ids.length === 0 ? html`<div class="muted empty">${this.t("noLines")}</div>` : ids.map((id) => this.renderLine(id))}
        </div>
      </ha-card>
    `;
  }

  private pill(line: string, mode: Mode, small = false): TemplateResult {
    return html`<span class="pill ${small ? "small" : ""}" style="background:${MODE_COLORS[mode] ?? MODE_COLORS.bus}">${line}</span>`;
  }

  private renderLine(entityId: string): TemplateResult {
    const t = this.t;
    const c = this._config!;
    const lang = this.hass!.locale?.language ?? this.hass!.language;
    const entity = this.hass!.states[entityId];
    if (!entity) return html`<div class="warning">${t("notFound", { id: entityId })}</div>`;
    const attrs = entity.attributes as NextAttributes;
    if (entity.state === "unavailable") {
      // Home Assistant drops the attributes of unavailable entities: show the name only.
      const name = String(entity.attributes.friendly_name ?? entityId).replace(/ (Next|Järgmine)$/, "");
      return html`<div class="line">
        <div class="row">
          <span class="pill" style="background:var(--disabled-color, #9e9e9e)">–</span>
          <div class="what"><div class="title">${name}</div></div>
          <span class="muted">${t("unavailable")}</span>
        </div>
      </div>`;
    }
    const v = view(attrs, this._now, c.departures!);
    const unavailable = false;
    const [first, ...rest] = v.departures;
    const open = () => {
      if (v.url) window.open(v.url, "_blank", "noopener");
    };
    const walk =
      c.show_walk && v.walkMin !== null
        ? `${t(v.walkApprox ? "walkApprox" : "walk", { n: v.walkMin })}${v.walkFrom ? " " + t("walkFrom", { place: v.walkFrom }) : ""}`
        : "";
    const meta = [v.stop ? t("from", { stop: v.stop }) : "", walk, v.via.length ? t("via", { stops: v.via.join(", ") }) : ""]
      .filter(Boolean)
      .join(" · ");
    const label = attrs.label && attrs.label !== attrs.line ? attrs.label : "";
    const countdown = unavailable
      ? html`<span class="muted">${t("unavailable")}</span>`
      : first
        ? html`<span class="big">${this.minutes(first.minutes)}</span>
            <span class="clock">${clock(first.time, lang)}${first.realtime ? nothing : html` · ${t("scheduled")}`}</span>`
        : html`<span class="muted">${attrs.error ? t("feedError") : t("noDepartures")}</span>`;

    if (c.layout === "compact") {
      return html`<div class="line" role=${v.url ? "link" : nothing} tabindex=${v.url ? "0" : nothing} @click=${open}
        @keydown=${(e: KeyboardEvent) => (e.key === "Enter" ? open() : undefined)}>
        <div class="row">
          ${this.pill(v.line, v.mode)}
          <div class="what"><div class="title">${label ? html`${label} · ` : nothing}${v.title}</div></div>
          <div class="countdown">${countdown}</div>
        </div>
      </div>`;
    }
    return html`
      <div
        class="line"
        role=${v.url ? "link" : nothing}
        tabindex=${v.url ? "0" : nothing}
        @click=${open}
        @keydown=${(e: KeyboardEvent) => (e.key === "Enter" ? open() : undefined)}
      >
        <div class="row">
          ${v.trip ? html`<div class="pills">${v.pills.map((p) => this.pill(p.line, p.mode, true))}</div>` : this.pill(v.line, v.mode)}
          <div class="what">
            <div class="title">${label ? html`${label} · ` : nothing}${v.title}</div>
            ${meta ? html`<div class="meta">${meta}</div>` : nothing}
          </div>
        </div>
        <div class="row second">
          <div class="countdown">${v.trip && first ? this.pill(v.line, v.mode, true) : nothing}${countdown}</div>
          ${c.show_verdict && first && first.slack !== null && !unavailable ? this.chip(first.level, first.slack) : nothing}
        </div>
        ${first && !unavailable && (first.changes.length > 0 || first.arriveAt)
          ? html`<div class="journey">
              ${first.changes.map(
                (ch) => html`<div class="change">
                  ↳ ${this.pill(ch.line, ch.mode, true)} <span class="clock">${clock(ch.time, lang)}</span>
                  <span class="muted">${t("changeAt", { stop: ch.at })} ·
                    <span class=${ch.wait_min === 0 ? "tight" : ""}>${ch.wait_min === 0 ? t("tight") : t("wait", { n: ch.wait_min })}</span></span>
                </div>`,
              )}
              ${first.arriveAt ? html`<div class="muted">${t("arrive", { time: clock(first.arriveAt, lang) })}</div>` : nothing}
            </div>`
          : nothing}
        ${rest.length > 0 && !unavailable
          ? html`<div class="later">
              ${t("then")}
              ${rest.map((d) => html`<span>${v.trip && d.line ? html`<b>${d.line}</b> ` : nothing}${this.minutes(d.minutes)}${d.realtime ? "" : "*"}</span>`)}
            </div>`
          : nothing}
      </div>
    `;
  }

  private minutes(n: number): string {
    return n <= 0 ? this.t("now") : this.t("min", { n });
  }

  private chip(level: Level, slack: number): TemplateResult {
    const key: Key = level === "red" ? "tooTight" : level === "amber" ? "leaveNow" : "leaveIn";
    return html`<span class="chip ${level}">${this.t(key, { n: slack })}</span>`;
  }

  static styles = css`
    .journey {
      display: flex;
      flex-direction: column;
      gap: 4px;
      margin-top: 8px;
      font-size: 0.9em;
    }
    .change {
      display: flex;
      flex-wrap: wrap;
      align-items: center;
      gap: 6px;
    }
    .tight {
      color: var(--warning-color, #b45309);
      font-weight: 600;
    }
    .lines {
      display: flex;
      flex-direction: column;
      gap: 8px;
      padding: 12px;
    }
    .line {
      padding: 14px 16px;
      border-radius: 16px;
      border: 1px solid var(--divider-color, rgba(127, 127, 127, 0.2));
      background: var(--card-background-color, var(--ha-card-background));
      cursor: default;
      outline: none;
    }
    .line[role="link"] {
      cursor: pointer;
    }
    .line[role="link"]:focus-visible {
      box-shadow: 0 0 0 2px var(--primary-color);
    }
    .row {
      display: flex;
      align-items: center;
      gap: 12px;
    }
    .second {
      margin-top: 10px;
      justify-content: space-between;
      flex-wrap: wrap;
      row-gap: 6px;
    }
    .pills {
      display: flex;
      flex-wrap: wrap;
      gap: 4px;
      max-width: 7.5em;
      flex: none;
    }
    .pill {
      flex: none;
      min-width: 2.4em;
      padding: 4px 10px;
      border-radius: 999px;
      color: #fff;
      font-weight: 700;
      text-align: center;
      font-size: 1.05em;
      line-height: 1.3;
    }
    .pill.small {
      min-width: 2em;
      padding: 2px 8px;
      font-size: 0.85em;
    }
    .what {
      flex: 1;
      min-width: 0;
    }
    .title {
      color: var(--primary-text-color);
      font-weight: 600;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .meta,
    .muted,
    .later,
    .clock {
      color: var(--secondary-text-color);
      font-size: 0.9em;
    }
    .meta {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .countdown {
      display: flex;
      align-items: baseline;
      gap: 10px;
      color: var(--primary-text-color);
      font-variant-numeric: tabular-nums;
    }
    .countdown .pill {
      align-self: center;
    }
    .big {
      font-size: 2.1em;
      font-weight: 650;
      letter-spacing: -0.02em;
      line-height: 1.05;
    }
    .later {
      margin-top: 8px;
      display: flex;
      flex-wrap: wrap;
      gap: 4px 12px;
      font-variant-numeric: tabular-nums;
    }
    .later b {
      color: var(--primary-text-color);
      font-weight: 600;
    }
    .chip {
      border-radius: 999px;
      padding: 4px 12px;
      font-size: 0.9em;
      font-weight: 600;
      white-space: nowrap;
      background: var(--secondary-background-color, rgba(127, 127, 127, 0.15));
      color: var(--primary-text-color);
    }
    /* Fixed colours, not the theme's status colours: those are too light for text (WCAG AA). */
    .chip.green {
      background: rgba(16, 185, 129, 0.16);
      color: #047857;
    }
    .chip.amber {
      background: rgba(245, 158, 11, 0.18);
      color: #92400e;
    }
    .chip.red {
      background: rgba(239, 68, 68, 0.16);
      color: #b91c1c;
    }
    .dark .chip.green {
      color: #6ee7b7;
    }
    .dark .chip.amber {
      color: #fcd34d;
    }
    .dark .chip.red {
      color: #fca5a5;
    }
    .compact .line {
      padding: 10px 14px;
    }
    .compact .countdown {
      flex-direction: column;
      align-items: flex-end;
      gap: 0;
    }
    .compact .big {
      font-size: 1.4em;
    }
    .compact .clock {
      font-size: 0.8em;
    }
    .empty,
    .warning {
      padding: 10px 16px;
    }
    .warning {
      color: var(--error-color, #b91c1c);
    }
  `;
}

/**
 * Defines the card (and its editor). The integration loads this module early;
 * while Home Assistant starts it can replace window.customElements with its own
 * registry, dropping definitions made before that. So define again if they
 * disappear, for the first half minute (the dashboard rebuilds the card once
 * it is defined).
 */
function register(): void {
  if (!customElements.get("bus-card-editor")) customElements.define("bus-card-editor", BusCardEditor);
  if (!customElements.get("bus-card")) customElements.define("bus-card", BusCard);
}
register();
const started = Date.now();
const watch = setInterval(() => {
  register();
  if (Date.now() - started > 30_000) clearInterval(watch);
}, 250);

declare global {
  interface Window {
    customCards?: { type: string; name: string; description: string; preview?: boolean; documentationURL?: string }[];
  }
}

window.customCards = window.customCards || [];
if (!window.customCards.some((c) => c.type === "bus-card")) {
  window.customCards.push({
    type: "bus-card",
    name: "Bus",
    description: "Next departures of your saved Tallinn lines, with walking time and the can-I-make-it verdict.",
    preview: true,
    documentationURL: "https://github.com/unmaykr-a/transit-tracker/tree/main/ha",
  });
}

console.info(`%c BUS-CARD %c ${VERSION} `, "color:#fff;background:#d02f2a;font-weight:600", "color:#d02f2a");

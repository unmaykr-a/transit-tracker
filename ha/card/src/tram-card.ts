import { css, html, LitElement, nothing, type PropertyValues, type TemplateResult } from "lit";

import { translator, type Key } from "./i18n";
import { MODE_COLORS, view, type Level, type NextAttributes } from "./model";
import type { HassEntity, HomeAssistant, TramCardConfig } from "./types";
import "./tram-card-editor";

const VERSION = "0.6.0";

const DEFAULTS = { departures: 3, show_walk: true, show_verdict: true, layout: "full" as const };

/** Entities created by the Tram integration for a line's next departure. */
export function isTramNext(e: HassEntity | undefined): boolean {
  return Boolean(e && e.entity_id.startsWith("sensor.") && e.entity_id.endsWith("_next") && Array.isArray(e.attributes.next_departures));
}

export class TramCard extends LitElement {
  static properties = {
    hass: { attribute: false },
    _config: { state: true },
    _now: { state: true },
  };

  hass?: HomeAssistant;
  private _config?: TramCardConfig;
  private _now = new Date();
  private _timer?: ReturnType<typeof setInterval>;

  static getConfigElement(): HTMLElement {
    return document.createElement("tram-card-editor");
  }

  static getStubConfig(hass: HomeAssistant): Partial<TramCardConfig> {
    const first = Object.values(hass.states).find(isTramNext);
    return { entities: first ? [first.entity_id] : [], departures: DEFAULTS.departures, show_walk: true, show_verdict: true };
  }

  setConfig(config: TramCardConfig): void {
    if (!config || !Array.isArray(config.entities) || config.entities.length === 0) {
      throw new Error("tram-card: set `entities` to one or more sensor.tram_*_next entities");
    }
    const departures = Math.min(5, Math.max(1, Math.round(config.departures ?? DEFAULTS.departures)));
    this._config = { ...DEFAULTS, ...config, departures };
  }

  getCardSize(): number {
    const n = this._config?.entities.length ?? 1;
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

  protected willUpdate(changed: PropertyValues): void {
    if (changed.has("hass")) this._now = new Date();
  }

  private get t() {
    return translator(this.hass?.locale?.language ?? this.hass?.language);
  }

  protected render(): TemplateResult | typeof nothing {
    if (!this._config || !this.hass) return nothing;
    const c = this._config;
    return html`
      <ha-card .header=${c.title || undefined}>
        <div class="lines ${c.layout}">${c.entities.map((id) => this.renderLine(id))}</div>
      </ha-card>
    `;
  }

  private renderLine(entityId: string): TemplateResult {
    const t = this.t;
    const c = this._config!;
    const entity = this.hass!.states[entityId];
    if (!entity) return html`<div class="warning">${t("notFound", { id: entityId })}</div>`;
    const v = view(entity.attributes as NextAttributes, this._now, c.departures!);
    const unavailable = entity.state === "unavailable";
    const [first, ...rest] = v.departures;
    const open = () => {
      if (v.url) window.open(v.url, "_blank", "noopener");
    };
    const meta = [v.stop, c.show_walk && v.walkMin !== null ? t(v.walkApprox ? "walkApprox" : "walk", { n: v.walkMin }) : ""].filter(Boolean).join(" · ");
    return html`
      <div
        class="line"
        role=${v.url ? "link" : nothing}
        tabindex=${v.url ? "0" : nothing}
        @click=${open}
        @keydown=${(e: KeyboardEvent) => (e.key === "Enter" ? open() : undefined)}
      >
        <div class="row">
          <span class="pill" style="background:${MODE_COLORS[v.mode] ?? MODE_COLORS.bus}">${v.line}</span>
          <div class="what">
            <div class="headsign">${v.headsign}</div>
            ${c.layout === "full" && meta ? html`<div class="meta">${meta}</div>` : nothing}
          </div>
          <div class="countdown" aria-live="polite">
            ${unavailable
              ? html`<span class="muted">${t("unavailable")}</span>`
              : first
                ? html`${this.minutes(first.minutes)}${first.realtime ? nothing : html`<span class="tag">${t("scheduled")}</span>`}`
                : html`<span class="muted">${t("noDepartures")}</span>`}
          </div>
        </div>
        ${c.layout === "full" && !unavailable && first
          ? html`<div class="row second">
              ${c.show_verdict && first.slack !== null ? this.chip(first.level, first.slack) : html`<span></span>`}
              <span class="later">${rest.map((d) => html`<span>${this.minutes(d.minutes)}</span>`)}</span>
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
    .lines {
      display: flex;
      flex-direction: column;
      padding: 8px 0;
    }
    .line {
      padding: 10px 16px;
      cursor: default;
      outline: none;
    }
    .line[role="link"] {
      cursor: pointer;
    }
    .line[role="link"]:focus-visible {
      box-shadow: inset 0 0 0 2px var(--primary-color);
      border-radius: 8px;
    }
    .line + .line {
      border-top: 1px solid var(--divider-color, rgba(127, 127, 127, 0.2));
    }
    .row {
      display: flex;
      align-items: center;
      gap: 12px;
    }
    .second {
      margin-top: 8px;
      justify-content: space-between;
    }
    .pill {
      flex: none;
      min-width: 2.5em;
      padding: 4px 10px;
      border-radius: 999px;
      color: #fff;
      font-weight: 600;
      text-align: center;
      font-size: 1.05em;
    }
    .what {
      flex: 1;
      min-width: 0;
    }
    .headsign {
      color: var(--primary-text-color);
      font-weight: 500;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .meta,
    .muted,
    .later,
    .tag {
      color: var(--secondary-text-color);
      font-size: 0.9em;
    }
    .countdown {
      flex: none;
      color: var(--primary-text-color);
      font-size: 1.6em;
      font-weight: 600;
      font-variant-numeric: tabular-nums;
      display: flex;
      flex-direction: column;
      align-items: flex-end;
      line-height: 1.1;
    }
    .countdown .muted {
      font-size: 0.55em;
      font-weight: 400;
    }
    .tag {
      font-size: 0.45em;
      font-weight: 400;
    }
    .later {
      display: flex;
      gap: 12px;
      font-variant-numeric: tabular-nums;
    }
    .chip {
      border-radius: 999px;
      padding: 3px 10px;
      font-size: 0.9em;
      font-weight: 500;
      white-space: nowrap;
      background: var(--secondary-background-color, rgba(127, 127, 127, 0.15));
      color: var(--primary-text-color);
    }
    .chip.green {
      background: rgba(16, 185, 129, 0.18);
      color: var(--success-color, #047857);
    }
    .chip.amber {
      background: rgba(245, 158, 11, 0.2);
      color: var(--warning-color, #b45309);
    }
    .chip.red {
      background: rgba(239, 68, 68, 0.18);
      color: var(--error-color, #b91c1c);
    }
    .compact .line {
      padding: 6px 16px;
    }
    .compact .countdown {
      font-size: 1.25em;
    }
    .warning {
      padding: 10px 16px;
      color: var(--error-color, #b91c1c);
    }
  `;
}

if (!customElements.get("tram-card")) customElements.define("tram-card", TramCard);

declare global {
  interface Window {
    customCards?: { type: string; name: string; description: string; preview?: boolean; documentationURL?: string }[];
  }
}

window.customCards = window.customCards || [];
if (!window.customCards.some((c) => c.type === "tram-card")) {
  window.customCards.push({
    type: "tram-card",
    name: "Tram",
    description: "Next departures of your saved Tallinn lines, with walking time and the can-I-make-it verdict.",
    preview: true,
    documentationURL: "https://github.com/unmaykr-a/transit-tracker/tree/main/ha",
  });
}

console.info(`%c TRAM-CARD %c ${VERSION} `, "color:#fff;background:#d02f2a;font-weight:600", "color:#d02f2a");

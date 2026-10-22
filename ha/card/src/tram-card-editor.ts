import { html, LitElement, nothing, type TemplateResult } from "lit";

import { translator } from "./i18n";
import type { HomeAssistant, TramCardConfig } from "./types";

/**
 * Visual editor. Uses Home Assistant's own <ha-form>, so the line picker is an
 * entity selector limited to the Tram integration's "next" sensors: users pick
 * saved lines from a list instead of typing entity ids.
 */
export class TramCardEditor extends LitElement {
  static properties = { hass: { attribute: false }, _config: { state: true } };

  hass?: HomeAssistant;
  private _config?: TramCardConfig;

  setConfig(config: TramCardConfig): void {
    this._config = config;
  }

  private schema() {
    const t = translator(this.hass?.locale?.language ?? this.hass?.language);
    return [
      {
        name: "entities",
        label: t("pickEntities"),
        required: true,
        selector: { entity: { multiple: true, filter: { integration: "tram", domain: "sensor", device_class: "timestamp" } } },
      },
      { name: "title", label: t("title"), selector: { text: {} } },
      { name: "departures", label: t("departures"), selector: { number: { min: 1, max: 5, mode: "slider" } } },
      {
        type: "grid",
        name: "",
        schema: [
          { name: "show_walk", label: t("showWalk"), selector: { boolean: {} } },
          { name: "show_verdict", label: t("showVerdict"), selector: { boolean: {} } },
        ],
      },
      {
        name: "layout",
        label: t("layout"),
        required: true,
        selector: {
          select: {
            mode: "dropdown",
            options: [
              { value: "full", label: t("full") },
              { value: "compact", label: t("compact") },
            ],
          },
        },
      },
    ];
  }

  protected render(): TemplateResult | typeof nothing {
    if (!this.hass || !this._config) return nothing;
    const data = { departures: 3, show_walk: true, show_verdict: true, layout: "full", ...this._config };
    return html`<ha-form
      .hass=${this.hass}
      .data=${data}
      .schema=${this.schema()}
      .computeLabel=${(s: { label?: string; name: string }) => s.label ?? s.name}
      @value-changed=${this.changed}
    ></ha-form>`;
  }

  private changed(ev: CustomEvent<{ value: TramCardConfig }>): void {
    ev.stopPropagation();
    const config = { ...ev.detail.value };
    if (!config.title) delete config.title;
    this._config = config;
    this.dispatchEvent(new CustomEvent("config-changed", { detail: { config }, bubbles: true, composed: true }));
  }
}

if (!customElements.get("tram-card-editor")) customElements.define("tram-card-editor", TramCardEditor);

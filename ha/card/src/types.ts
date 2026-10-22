/** The slice of Home Assistant's frontend API the card uses. */
export interface HassEntity {
  entity_id: string;
  state: string;
  attributes: Record<string, unknown>;
  last_updated?: string;
}

export interface HomeAssistant {
  states: Record<string, HassEntity>;
  locale?: { language: string };
  language?: string;
}

export interface BusCardConfig {
  type: string;
  /** Empty or missing: every saved line. */
  entities?: string[];
  departures?: number;
  show_walk?: boolean;
  show_verdict?: boolean;
  layout?: "full" | "compact";
  title?: string;
}

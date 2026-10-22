// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

import { BusCard } from "./bus-card";
import type { HomeAssistant } from "./types";

function hass(minutes: number[], leave: (number | null)[], lang = "en"): HomeAssistant {
  const now = Date.now();
  const deps = minutes.map((m, i) => ({
    time: new Date(now + m * 60000 + 30000).toISOString(),
    minutes: m,
    delay_s: 0,
    realtime: i !== 2,
    headsign: "Kopli",
    verdict: "",
    leave_in_min: leave[i] ?? null,
  }));
  return {
    locale: { language: lang },
    states: {
      "sensor.bus_work_next": {
        entity_id: "sensor.bus_work_next",
        state: deps[0]?.time ?? "unknown",
        attributes: {
          line: "T4",
          mode: "tram",
          headsign: "Kopli",
          stop_name: "Hobujaama",
          platform: "A",
          walk_min: 5,
          next_departures: deps,
          url: "https://bus.example.com/lines/tram/T4?stop=1",
        },
      },
      "sensor.other": { entity_id: "sensor.other", state: "1", attributes: {} },
    },
  };
}

async function mount(config: object, h: HomeAssistant): Promise<ShadowRoot> {
  const card = document.createElement("bus-card") as BusCard;
  card.setConfig({ type: "custom:bus-card", ...config } as never);
  card.hass = h;
  document.body.append(card);
  await card.updateComplete;
  return card.shadowRoot!;
}

const text = (root: ShadowRoot) => root.textContent!.replace(/\s+/g, " ").trim();

afterEach(() => {
  document.body.innerHTML = "";
});

describe("bus-card", () => {
  it("rejects a config without entities", () => {
    const card = document.createElement("bus-card") as BusCard;
    expect(() => card.setConfig({ type: "custom:bus-card", entities: [] })).toThrow(/entities/);
  });

  it("renders the line, countdown, verdict, walk and later departures", async () => {
    const root = await mount({ entities: ["sensor.bus_work_next"], departures: 3 }, hass([9, 17, 25, 33], [4, 12, 20, 28]));
    const s = text(root);
    expect(root.querySelector(".pill")!.textContent).toBe("T4");
    expect(s).toContain("Kopli");
    expect(s).toContain("Hobujaama · A · 5 min walk");
    expect(root.querySelector(".countdown")!.textContent).toContain("9 min");
    expect(root.querySelector(".chip.green")!.textContent).toBe("Leave in 4 min");
    expect([...root.querySelectorAll(".later span")].map((e) => e.textContent)).toEqual(["17 min", "25 min"]);
  });

  it("follows the toggles, the compact layout and Estonian", async () => {
    const root = await mount(
      { entities: ["sensor.bus_work_next"], show_walk: false, show_verdict: false },
      hass([0, 8], [-5, 3], "et"),
    );
    // The first departure cannot be caught any more, so the card starts at the next one.
    expect(root.querySelector(".countdown")!.textContent).toContain("8 min");
    expect(text(root)).not.toContain("jalgsi");
    expect(root.querySelector(".chip")).toBeNull();

    const compact = await mount({ entities: ["sensor.bus_work_next"], layout: "compact" }, hass([2], [0], "et"));
    expect(compact.querySelector(".second")).toBeNull();
    expect(compact.querySelector(".lines.compact")).not.toBeNull();
  });

  it("warns about missing entities and opens the line page on tap", async () => {
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    const root = await mount({ entities: ["sensor.bus_work_next", "sensor.bus_gone_next"] }, hass([9], [4]));
    expect(text(root)).toContain("Entity not found: sensor.bus_gone_next");
    (root.querySelector(".line") as HTMLElement).click();
    expect(open).toHaveBeenCalledWith("https://bus.example.com/lines/tram/T4?stop=1", "_blank", "noopener");
  });

  it("offers a stub config with the first Bus entity and registers in the card picker", () => {
    expect(BusCard.getStubConfig(hass([9], [4]))).toMatchObject({ entities: ["sensor.bus_work_next"] });
    expect(window.customCards?.some((c) => c.type === "bus-card")).toBe(true);
  });
});

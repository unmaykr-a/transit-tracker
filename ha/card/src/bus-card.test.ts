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
  it("shows every saved line, in Bus order, when no entities are listed", async () => {
    const h = hass([9], [4]);
    h.states["sensor.bus_gym_next"] = {
      entity_id: "sensor.bus_gym_next",
      state: "unknown",
      attributes: { ...h.states["sensor.bus_work_next"]!.attributes, line: "T1", order: 0 },
    };
    h.states["sensor.bus_work_next"]!.attributes.order = 1;
    const root = await mount({}, h);
    expect([...root.querySelectorAll(".pill")].map((e) => e.textContent)).toEqual(["T1", "T4"]);
    const card = document.createElement("bus-card") as BusCard;
    expect(() => card.setConfig({ type: "custom:bus-card", entities: "sensor.x" as never })).toThrow(/entities/);
  });

  it("renders a trip with its lines, the departure's own line and where the walk starts", async () => {
    const h = hass([9, 12], [4, 7]);
    const a = h.states["sensor.bus_work_next"]!.attributes as Record<string, unknown>;
    Object.assign(a, { kind: "trip", lines: ["T4", "3"], stop_name: "Viru", headsign: "Kopli", destination: "Balti jaam", walk_from: "Home", label: "Work" });
    (a.next_departures as Record<string, unknown>[])[0]!.line = "3";
    (a.next_departures as Record<string, unknown>[])[0]!.mode = "bus";
    (a.next_departures as Record<string, unknown>[])[1]!.line = "T4";
    const root = await mount({ entities: ["sensor.bus_work_next"] }, h);
    const s = text(root);
    expect(s).toContain("Work · Viru → Balti jaam");
    expect(s).toContain("5 min walk from Home");
    expect(root.querySelector(".countdown .pill")!.textContent).toBe("3");
    expect(root.querySelector(".later")!.textContent).toContain("T4");
  });

  it("renders the line, countdown, verdict, walk and later departures", async () => {
    const root = await mount({ entities: ["sensor.bus_work_next"], departures: 3 }, hass([9, 17, 25, 33], [4, 12, 20, 28]));
    const s = text(root);
    expect(root.querySelector(".pill")!.textContent).toBe("T4");
    expect(s).toContain("→ Kopli");
    expect(s).toContain("from Hobujaama · A · 5 min walk");
    expect(root.querySelector(".countdown .big")!.textContent).toBe("9 min");
    expect(root.querySelector(".clock")!.textContent).toMatch(/^\d{2}:\d{2}$/);
    expect(root.querySelector(".chip.green")!.textContent).toBe("Leave in 4 min");
    expect([...root.querySelectorAll(".later span")].map((e) => e.textContent)).toEqual(["17 min", "25 min*"]);
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
    expect(compact.querySelector(".big")!.textContent).toBe("2 min");
    expect(compact.querySelector(".lines.compact")).not.toBeNull();
  });

  it("warns about missing entities and opens the line page on tap", async () => {
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    const root = await mount({ entities: ["sensor.bus_work_next", "sensor.bus_gone_next"] }, hass([9], [4]));
    expect(text(root)).toContain("Entity not found: sensor.bus_gone_next");
    (root.querySelector(".line") as HTMLElement).click();
    expect(open).toHaveBeenCalledWith("https://bus.example.com/lines/tram/T4?stop=1", "_blank", "noopener");
  });

  it("offers a stub config (all lines) and registers in the card picker", () => {
    expect(BusCard.getStubConfig()).not.toHaveProperty("entities");
    expect(window.customCards?.some((c) => c.type === "bus-card")).toBe(true);
  });
});

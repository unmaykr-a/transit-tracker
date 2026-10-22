import { render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import App from "./App";


function mockFetch(impl: () => Promise<Response>) {
  vi.stubGlobal("fetch", vi.fn(impl));
}

afterEach(() => vi.unstubAllGlobals());

describe("App", () => {
  it("shows server status from /healthz", async () => {
    mockFetch(async () => Response.json({ status: "ok", version: "v0.1.0", commit: "abc", uptime_s: 1 }));
    render(<App />);
    expect(screen.getByText("Checking server…")).toBeInTheDocument();
    expect(await screen.findByText("ok")).toBeInTheDocument();
    expect(screen.getByText(/v0\.1\.0/)).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledWith("/healthz", expect.anything());
  });

  it("offers retry when the server is down", async () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    mockFetch(async () => new Response("down", { status: 503 }));
    render(<App />);
    const retry = await screen.findByRole("button", { name: "Retry" });
    mockFetch(async () => Response.json({ status: "ok", version: "dev", commit: "x", uptime_s: 0 }));
    retry.click();
    expect(await screen.findByText("ok")).toBeInTheDocument();
  });

  it("lists all four modes", () => {
    mockFetch(() => new Promise(() => {}));
    render(<App />);
    const list = screen.getByRole("list", { name: "Transport modes" });
    expect(within(list).getAllByRole("listitem").map((li) => li.textContent)).toEqual(["Tram", "Bus", "Trolley", "Train"]);
  });
});

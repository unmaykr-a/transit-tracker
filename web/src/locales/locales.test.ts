import { describe, expect, it } from "vitest";

import en from "./en.json";
import et from "./et.json";

type Tree = { [k: string]: string | Tree };

function flatten(t: Tree, prefix = ""): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(t)) {
    const key = prefix ? `${prefix}.${k}` : k;
    if (typeof v === "string") out[key] = v;
    else Object.assign(out, flatten(v, key));
  }
  return out;
}

// Plural forms (_one/_other) may differ between languages; compare the base key.
const base = (k: string) => k.replace(/_(zero|one|two|few|many|other)$/, "");
const vars = (s: string) => [...s.matchAll(/\{\{\s*(\w+)\s*\}\}/g)].map((m) => m[1]).sort();

describe("locales", () => {
  const e = flatten(en as Tree);
  const t = flatten(et as Tree);

  it("have the same keys", () => {
    const ek = new Set(Object.keys(e).map(base));
    const tk = new Set(Object.keys(t).map(base));
    expect([...ek].filter((k) => !tk.has(k)), "missing in et").toEqual([]);
    expect([...tk].filter((k) => !ek.has(k)), "missing in en").toEqual([]);
  });

  it("use the same interpolation variables", () => {
    const mismatched = Object.keys(e)
      .filter((k) => k in t)
      .filter((k) => vars(e[k] ?? "").join() !== vars(t[k] ?? "").join());
    expect(mismatched).toEqual([]);
  });

  it("have no empty strings", () => {
    expect(Object.entries({ ...e, ...t }).filter(([, v]) => v.trim() === "")).toEqual([]);
  });
});

import resolve from "@rollup/plugin-node-resolve";
import terser from "@rollup/plugin-terser";
import typescript from "@rollup/plugin-typescript";

import { readFileSync } from "node:fs";

// The release passes BUS_VERSION; local builds use package.json's.
const version = process.env.BUS_VERSION || JSON.parse(readFileSync(new URL("./package.json", import.meta.url), "utf8")).version;

/** Puts the version into the bundle (the card logs it to the browser console). */
const injectVersion = () => ({
  name: "inject-version",
  transform: (code) => (code.includes("__BUS_VERSION__") ? code.replaceAll("__BUS_VERSION__", version) : null),
});

// One self-contained ES module, as Home Assistant loads dashboard resources.
export default {
  input: "src/bus-card.ts",
  output: { file: "dist/bus-card.js", format: "es", sourcemap: false },
  plugins: [injectVersion(), resolve(), typescript({ exclude: ["src/**/*.test.ts"] }), terser({ format: { comments: false } })],
};

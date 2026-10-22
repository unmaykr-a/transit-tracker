import resolve from "@rollup/plugin-node-resolve";
import terser from "@rollup/plugin-terser";
import typescript from "@rollup/plugin-typescript";

// One self-contained ES module, as Home Assistant loads dashboard resources.
export default {
  input: "src/bus-card.ts",
  output: { file: "dist/bus-card.js", format: "es", sourcemap: false },
  plugins: [resolve(), typescript({ exclude: ["src/**/*.test.ts"] }), terser({ format: { comments: false } })],
};

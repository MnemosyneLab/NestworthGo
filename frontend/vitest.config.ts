import path from "node:path";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import { CITestSequencer } from "./scripts/ci-test-sequencer.ts";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  test: {
    // Bound expensive jsdom accessibility/focus work on CPU-limited runners.
    // Two-CPU paired runs reproduce the historical-view race at four workers;
    // two workers lower individual interaction times without relaxing deadlines.
    maxWorkers: 2,
    sequence: { sequencer: CITestSequencer },
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    css: true,
    globals: false,
  },
});

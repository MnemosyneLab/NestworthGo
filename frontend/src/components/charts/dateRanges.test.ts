import { expect, it } from "vitest";
import { presetRange, shiftRange } from "./dateRanges";
it("moves weeks and days without overlap", () => {
  const week = presetRange("1w", "2026-09-27");
  expect(week).toEqual({ from: "2026-09-21", to: "2026-09-27" });
  const previous = shiftRange(week, -1, "1w");
  expect(previous).toEqual({ from: "2026-09-14", to: "2026-09-20" });
  expect(shiftRange(previous, 1, "1w")).toEqual(week);
  expect(shiftRange({ from: "2026-09-21", to: "2026-09-21" }, 1, "1d")).toEqual({ from: "2026-09-22", to: "2026-09-22" });
});
it("handles leap months and clamps to available history", () => {
  expect(presetRange("1m", "2024-03-31")).toEqual({ from: "2024-03-01", to: "2024-03-31" });
  expect(shiftRange({ from: "2024-03-01", to: "2024-03-31" }, -1, "1m")).toEqual({ from: "2024-02-01", to: "2024-02-29" });
  expect(presetRange("1w", "2026-09-27", "2026-09-25")).toEqual({ from: "2026-09-25", to: "2026-09-27" });
});

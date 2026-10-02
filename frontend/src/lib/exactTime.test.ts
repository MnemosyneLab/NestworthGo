import { expect, it } from "vitest";
import { formatExactTimestamp, hasSubMinuteTime } from "./time";

it("checks local seconds and milliseconds in the origin timezone", () => {
  expect(hasSubMinuteTime("2026-10-02T12:34:00.000Z", "Asia/Shanghai")).toBe(false);
  expect(hasSubMinuteTime("2026-10-02T12:34:00.123Z", "Asia/Shanghai")).toBe(true);
  expect(hasSubMinuteTime("2026-10-02T12:34:45.000Z", "UTC")).toBe(true);
  // Paris used +00:09:21: UTC seconds alone are not the local minute boundary.
  expect(hasSubMinuteTime("1900-01-01T00:00:39.000Z", "Europe/Paris")).toBe(false);
  expect(hasSubMinuteTime("1900-01-01T00:00:00.000Z", "Europe/Paris")).toBe(true);
});

it("shows seconds and milliseconds in each supported UI language", () => {
  for (const language of ["en", "zh-CN", "zh-TW"]) {
    expect(formatExactTimestamp("2026-10-02T12:34:50.789Z", "Asia/Shanghai", language)).toContain("20:34:50.789");
  }
});

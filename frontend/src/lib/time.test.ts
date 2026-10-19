import { describe, expect, it } from "vitest";
import { formatCivilDate, formatCivilDateRange as format, formatQuoteAsOf, timeZoneOptions } from "./time";

const formatCivilDateRange = (...args: Parameters<typeof format>) => format(...args).replace(/\s/g, " ");

describe("formatCivilDateRange", () => {
  it("formats civil dates without shifting them across timezones and collapses shared parts", () => {
    expect(formatCivilDateRange("2026-10-05", "2026-10-14", "en")).toBe("Oct 5 – 14, 2026");
    expect(formatCivilDateRange("2026-09-15", "2026-10-14", "en")).toBe("Sep 15 – Oct 14, 2026");
    expect(formatCivilDateRange("2026-10-14", "2026-10-14", "en")).toBe("Oct 14, 2026");
    expect(formatCivilDateRange("bad", "2026-10-14", "en")).toBe("bad – 2026-10-14");
  });
});

describe.each([
  ["en", "Oct 15, 2026", /5:00\s*PM/],
  ["zh-CN", "2026年10月15日", /17:00/],
  ["zh-TW", "2026年10月15日", /下午5:00/],
])("localized quote dates in %s", (language, expected, time) => {
  it("keeps a source date unchanged across timezones and never adds a time", () => {
    expect(formatCivilDate("2026-10-15", language)).toBe(expected);
    for (const timezone of ["Pacific/Auckland", "America/Los_Angeles", "UTC"]) {
      expect(formatQuoteAsOf({ quotedAt: "2026-10-14T16:00:00Z", effectiveDate: "2026-10-15", timestampBasis: "date_label" }, timezone, language)).toBe(expected);
      expect(formatQuoteAsOf({ quotedAt: "2026-10-15" }, timezone, language)).toBe(expected);
    }
  });

  it("retains actual quote times in the selected timezone using the same date style", () => {
    const result = formatQuoteAsOf({ quotedAt: "2026-10-15T09:00:00Z", timestampBasis: "session_close" }, "Asia/Singapore", language);
    expect(result).toContain(expected);
    expect(result).toMatch(time);
  });
});

it("does not normalize invalid civil dates or convert timestamps into dates", () => {
  for (const value of ["", "bad", "2026-02-30", "2026-00-15", "2026-13-15", "0000-10-15", "2026-10-15T09:00:00Z"]) {
    expect(formatCivilDate(value, "zh-CN")).toBe(value);
  }
  expect(formatCivilDate("2024-02-29", "en")).toBe("Feb 29, 2024");
});

it("falls back to UTC when Intl.supportedValuesOf is unavailable", () => {
  const descriptor = Object.getOwnPropertyDescriptor(Intl, "supportedValuesOf");
  Object.defineProperty(Intl, "supportedValuesOf", { configurable: true, value: undefined });
  try {
    expect(timeZoneOptions()).toEqual(["UTC"]);
  } finally {
    if (descriptor) Object.defineProperty(Intl, "supportedValuesOf", descriptor);
    else Reflect.deleteProperty(Intl, "supportedValuesOf");
  }
});

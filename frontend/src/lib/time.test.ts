import { afterEach, describe, expect, it } from "vitest";
import { resetDisplayPreferences, setDisplayPreferences } from "./displayPreferences";
import { formatCivilDate, formatCivilDateRange as format, formatQuoteAsOf, formatTimestamp, timeZoneOptions } from "./time";

const formatCivilDateRange = (...args: Parameters<typeof format>) => format(...args).replace(/\s/g, " ");

afterEach(() => {
  resetDisplayPreferences();
});

describe("formatCivilDateRange", () => {
  it("formats civil dates in the selected date format without shifting them across timezones", () => {
    expect(formatCivilDateRange("2026-10-05", "2026-10-14", "en")).toBe("2026-10-05 – 2026-10-14");
    expect(formatCivilDateRange("2026-10-14", "2026-10-14", "en")).toBe("2026-10-14");
    expect(formatCivilDateRange("bad", "2026-10-14", "en")).toBe("bad – 2026-10-14");
    setDisplayPreferences({ dateFormat: "day-first" });
    expect(formatCivilDateRange("2026-10-05", "2026-10-14", "en")).toBe("05/10/2026 – 14/10/2026");
    setDisplayPreferences({ dateFormat: "localized" });
    expect(formatCivilDateRange("2026-10-05", "2026-10-14", "en")).toBe("Oct 5 – 14, 2026");
    expect(formatCivilDateRange("2026-09-15", "2026-10-14", "en")).toBe("Sep 15 – Oct 14, 2026");
  });
});

describe.each([
  ["en", "2026-10-15", /17:00/],
  ["zh-CN", "2026-10-15", /17:00/],
  ["zh-TW", "2026-10-15", /17:00/],
])("quote dates in %s", (language, expected, time) => {
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

it("honors 24-hour time and switches to 12-hour when that setting is selected", () => {
  expect(formatTimestamp("2026-10-20T15:04:00Z", "Asia/Singapore", "en")).toBe("2026-10-20 23:04");
  expect(formatTimestamp("2026-10-20T15:04:00Z", "Asia/Singapore", "zh-CN")).toBe("2026-10-20 23:04");
  expect(formatTimestamp("2026-10-20T15:04:00Z", "Asia/Singapore", "zh-TW")).toBe("2026-10-20 23:04");
  expect(formatTimestamp("2026-10-20T15:04:00Z", "UTC", "en")).toBe("2026-10-20 15:04");
  setDisplayPreferences({ timeFormat: "12h", dateFormat: "localized" });
  expect(formatTimestamp("2026-10-20T15:04:00Z", "Asia/Singapore", "en")).toMatch(/11:04\s*PM/);
  expect(formatTimestamp("2026-10-20T15:04:00Z", "UTC", "zh-TW")).toMatch(/下午3:04|3:04/);
  setDisplayPreferences({ dateFormat: "month-first", timeFormat: "24h" });
  expect(formatTimestamp("2026-10-20T15:04:00Z", "UTC", "zh-CN")).toBe("10/20/2026 15:04");
  expect(formatCivilDate("2026-10-20", "zh-TW")).toBe("10/20/2026");
});

it("does not normalize invalid civil dates or convert timestamps into dates", () => {
  for (const value of ["", "bad", "2026-02-30", "2026-00-15", "2026-13-15", "0000-10-15", "2026-10-15T09:00:00Z"]) {
    expect(formatCivilDate(value, "zh-CN")).toBe(value);
  }
  expect(formatCivilDate("2024-02-29", "en")).toBe("2024-02-29");
  setDisplayPreferences({ dateFormat: "localized" });
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

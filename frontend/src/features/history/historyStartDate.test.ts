import { describe, expect, it } from "vitest";
import { localDateInTimeZone } from "./historyStartDate";

describe("localDateInTimeZone", () => {
  it("returns the calendar date in the confirmed timezone", () => {
    const now = new Date("2026-08-27T11:00:00.000Z");
    expect(localDateInTimeZone("UTC", now)).toBe("2026-08-27");
    expect(localDateInTimeZone("Asia/Shanghai", now)).toBe("2026-08-27");
    expect(localDateInTimeZone("Pacific/Honolulu", now)).toBe("2026-08-27");
  });

  it("uses the timezone, not UTC, when they differ by date", () => {
    const now = new Date("2026-08-27T04:00:00.000Z");
    expect(localDateInTimeZone("UTC", now)).toBe("2026-08-27");
    expect(localDateInTimeZone("Pacific/Honolulu", now)).toBe("2026-08-26");
  });

  it("does not invent a date for an invalid timezone", () => {
    expect(localDateInTimeZone("not-a-zone")).toBeUndefined();
    expect(localDateInTimeZone("")).toBeUndefined();
  });
});

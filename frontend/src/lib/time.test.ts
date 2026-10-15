import { describe, expect, it } from "vitest";
import { formatCivilDateRange as format } from "./time";

const formatCivilDateRange = (...args: Parameters<typeof format>) => format(...args).replace(/\s/g, " ");

describe("formatCivilDateRange", () => {
  it("formats civil dates without shifting them across timezones and collapses shared parts", () => {
    expect(formatCivilDateRange("2026-10-05", "2026-10-14", "en")).toBe("Oct 5 – 14, 2026");
    expect(formatCivilDateRange("2026-09-15", "2026-10-14", "en")).toBe("Sep 15 – Oct 14, 2026");
    expect(formatCivilDateRange("2026-10-14", "2026-10-14", "en")).toBe("Oct 14, 2026");
    expect(formatCivilDateRange("bad", "2026-10-14", "en")).toBe("bad – 2026-10-14");
  });
});

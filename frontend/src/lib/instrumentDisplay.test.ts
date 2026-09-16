import { describe, expect, it } from "vitest";
import { instrumentDisplayLabel, instrumentDisplayLabels, instrumentSecondaryName } from "./instrumentDisplay";

describe("instrumentDisplayLabel", () => {
  it("prefers the ticker over a long legal name", () => {
    expect(instrumentDisplayLabel({ name: "Alpha Architect 1-3 Month Box ETF", symbol: "BOXX" })).toBe("BOXX");
  });

  it("falls back to the name when no ticker is stored", () => {
    expect(instrumentDisplayLabel({ name: "Family Fund", symbol: "  " }, "unknown")).toBe("Family Fund");
    expect(instrumentDisplayLabel({}, "unknown")).toBe("unknown");
  });
});

describe("instrumentSecondaryName", () => {
  it("hides a name that only repeats the ticker", () => {
    expect(instrumentSecondaryName({ name: "qqq", symbol: "QQQ" })).toBeUndefined();
    expect(instrumentSecondaryName({ name: "Family Fund" })).toBeUndefined();
  });

  it("keeps a distinct legal name", () => {
    expect(instrumentSecondaryName({ name: "NVIDIA Corporation", symbol: "NVDA" })).toBe("NVIDIA Corporation");
  });
});

describe("instrumentDisplayLabels", () => {
  it("maps ids to ticker-first labels", () => {
    expect(
      instrumentDisplayLabels([
        { id: "i1", name: "NVIDIA Corporation", symbol: "NVDA" },
        { id: "i2", name: "Cash-like" },
      ]),
    ).toEqual(
      new Map([
        ["i1", "NVDA"],
        ["i2", "Cash-like"],
      ]),
    );
  });
});

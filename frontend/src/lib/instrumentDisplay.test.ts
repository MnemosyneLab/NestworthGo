import { describe, expect, it } from "vitest";
import { instrumentDisplayLabel, instrumentDisplayLabels, instrumentSecondaryName, instrumentSelectionLabel } from "./instrumentDisplay";

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


describe("short names", () => {
  it.each([
    [{ shortName: "  家庭 🌿  ", symbol: "001234", name: "Full name" }, "家庭 🌿"],
    [{ shortName: "\u2003 \n", symbol: " CODE ", name: "Full name" }, "CODE"],
    [{ shortName: "", symbol: "", name: " Full name " }, "Full name"],
    [{ shortName: "\u0085\uFEFF", symbol: " CODE ", name: "Full name" }, "CODE"],
    [{ metalTemplate: "gold", shortName: "Alias", symbol: "XAU", name: "Gold" }, "Alias"],
    [{ metalTemplate: "gold", symbol: "XAU", name: "Gold" }, "XAU"],
  ])("uses the same universal rule for %j", (identity, expected) => {
    expect(instrumentDisplayLabel(identity)).toBe(expected);
  });
  it("keeps full-name subtitle and duplicate identities", () => {
    const a = { id: "a", shortName: "Family fund", symbol: "000001", name: "Fund A", isin: "A", quoteCurrency: "CNY" };
    const b = { ...a, id: "b", symbol: "000002", name: "Fund B", isin: "B" };
    expect(instrumentSecondaryName(a)).toBe("Fund A");
    expect(instrumentDisplayLabels([a, b])).toEqual(new Map([["a", "Family fund"], ["b", "Family fund"]]));
    expect(instrumentSelectionLabel(a)).toBe("Family fund · 000001 · Fund A · CNY · A");
    expect(instrumentSelectionLabel(b)).toBe("Family fund · 000002 · Fund B · CNY · B");
    expect(instrumentDisplayLabel({ shortName: "界".repeat(120), name: "Full name" })).toBe("界".repeat(120));
  });
});

it("retains unit and provider identity in selectors without changing the primary label", () => {
  const common = { name: "Gold", shortName: "Family gold", symbol: "XAU", quoteCurrency: "USD", providerKey: "yahoo_finance", providerSymbol: "GC=F" };
  expect(instrumentDisplayLabel({ ...common, quantityUnit: "g" })).toBe("Family gold");
  expect(instrumentSelectionLabel({ ...common, quantityUnit: "g" })).toBe("Family gold · XAU · Gold · USD · g · yahoo_finance · GC=F");
  expect(instrumentSelectionLabel({ ...common, quantityUnit: "troy_oz" })).toContain("troy_oz");
});

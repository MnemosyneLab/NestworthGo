import { describe, expect, it } from "vitest";
import { instrumentDisplayLabel, instrumentDisplayLabels, instrumentSecondaryName, instrumentSelectionLabel, instrumentSelectionLabels } from "./instrumentDisplay";

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


describe("compact selection labels", () => {
  it("shows only trimmed short name and code, without repeating identical text", () => {
    const metadata = { name: "An exceptionally long legal name", marketCode: "EXCHANGE", quoteCurrency: "USD", providerKey: "long-provider", providerSymbol: "provider-code" };
    expect(instrumentSelectionLabels([
      { ...metadata, id: "a", shortName: " Alias ", symbol: " ABC " },
      { ...metadata, id: "b", shortName: " ", symbol: "XYZ" },
      { ...metadata, id: "c", shortName: "SAME", symbol: "SAME" },
    ])).toEqual(new Map([["a", "Alias · ABC"], ["b", "XYZ"], ["c", "SAME"]]));
  });
  it("adds only the shortest distinguishing field to exact collisions", () => {
    expect(instrumentSelectionLabels([
      { id: "a", shortName: "Fund", symbol: "ABC", marketCode: "LONG-MARKET-A", quoteCurrency: "USD", providerKey: "provider-a" },
      { id: "b", shortName: "Fund", symbol: "ABC", marketCode: "LONG-MARKET-B", quoteCurrency: "EUR", providerKey: "provider-b" },
      { id: "c", shortName: "Other", symbol: "ABC" },
    ])).toEqual(new Map([["a", "Fund · ABC [USD]"], ["b", "Fund · ABC [EUR]"], ["c", "Other · ABC"]]));
  });
  it("preserves distinct IDs even when all identity metadata matches", () => {
    const labels = instrumentSelectionLabels([
      { id: "same-1-long", symbol: "ABC", name: "Same" },
      { id: "same-2-long", symbol: "ABC", name: "Same" },
    ]);
    expect(labels).toEqual(new Map([["same-1-long", "ABC [#1]"], ["same-2-long", "ABC [#2]"]]));
  });
  it("handles missing codes with the existing primary fallback", () => {
    expect(instrumentSelectionLabels([{ id: "a", name: "Manual fund" }, { id: "b" }], "Unknown"))
      .toEqual(new Map([["a", "Manual fund"], ["b", "Unknown"]]));
  });
  it("disambiguates a literal alias matching a decorated label", () => {
    const labels = instrumentSelectionLabels([
      { id: "a", symbol: "ABC", quoteCurrency: "USD" },
      { id: "b", symbol: "ABC", quoteCurrency: "EUR" },
      { id: "c", shortName: "ABC [USD]" },
    ]);
    expect(new Set(labels.values()).size).toBe(3);
  });
});

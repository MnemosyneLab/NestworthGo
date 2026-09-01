import { describe, expect, it } from "vitest";
import { groupByInstrumentType, sortFxPairs } from "./groupByInstrumentType";

describe("groupByInstrumentType", () => {
  it("groups by domain type order and sorts by currency then name", () => {
    const grouped = groupByInstrumentType([
      { name: "Beta", type: "etf", quoteCurrency: "USD" },
      { name: "Alpha", type: "stock", quoteCurrency: "USD" },
      { name: "Yuan Stock", type: "stock", quoteCurrency: "CNY" },
      { name: "Gold", type: "precious_metal", quoteCurrency: "USD" },
      { name: "Mystery", type: "custom_type", quoteCurrency: "USD" },
    ]);
    expect(grouped.map((group) => group.key)).toEqual(["stock", "etf", "precious_metal", "custom_type"]);
    expect(grouped[0]?.items.map((item) => item.name)).toEqual(["Yuan Stock", "Alpha"]);
  });

  it("places instruments without a type in other", () => {
    const grouped = groupByInstrumentType([{ name: "Cash-like", quoteCurrency: "USD" }]);
    expect(grouped.map((group) => group.key)).toEqual(["other"]);
  });
});

describe("sortFxPairs", () => {
  it("sorts by currencyA then currencyB", () => {
    expect(
      sortFxPairs([
        { currencyA: "USD", currencyB: "SGD" },
        { currencyA: "CNY", currencyB: "SGD" },
        { currencyA: "CNY", currencyB: "USD" },
      ]).map((pair) => `${pair.currencyA}/${pair.currencyB}`),
    ).toEqual(["CNY/SGD", "CNY/USD", "USD/SGD"]);
  });
});

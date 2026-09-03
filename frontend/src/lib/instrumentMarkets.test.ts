import { describe, expect, it } from "vitest";
import { countryForMarket, marketsForCountry } from "./instrumentMarkets";

const CATALOG = ["ASX", "SSE", "SZSE", "NASDAQ", "NYSE", "AMEX", "HKEX", "SGX"];

describe("instrument market country mapping", () => {
  it("maps a market to its country", () => {
    expect(countryForMarket("NASDAQ")).toBe("US");
    expect(countryForMarket("HKEX")).toBe("HK");
    expect(countryForMarket("UNKNOWN")).toBeUndefined();
  });

  it("filters markets to the selected country and keeps a saved outlier", () => {
    expect(marketsForCountry(CATALOG, "")).toEqual(CATALOG);
    expect(marketsForCountry(CATALOG, "US")).toEqual(["NASDAQ", "NYSE", "AMEX"]);
    expect(marketsForCountry(CATALOG, "US", "SGX")).toEqual(["NASDAQ", "NYSE", "AMEX", "SGX"]);
  });
});

import { describe, expect, it } from "vitest";
import { accountCompositionItems, sortValuationComponents } from "./accountComposition";
import type { ValuationComponentDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

function component(overrides: Partial<ValuationComponentDTO>): ValuationComponentDTO {
  return {
    accountId: "acc-1",
    nativeAmount: "0",
    nativeCurrency: "USD",
    available: true,
    ...overrides,
  };
}

describe("accountCompositionItems", () => {
  it("sorts valued slices by household amount and keeps native amounts", () => {
    const items = accountCompositionItems(
      [
        component({ nativeCurrency: "SGD", nativeAmount: "200", baseAmount: { amount: "150", currency: "USD" } }),
        component({
          holdingId: "h1",
          instrumentId: "i1",
          instrumentName: "NVIDIA",
          nativeAmount: "1500",
          nativeCurrency: "USD",
          baseAmount: { amount: "1500", currency: "USD" },
        }),
        component({ nativeCurrency: "USD", nativeAmount: "800", baseAmount: { amount: "800", currency: "USD" } }),
      ],
      "USD",
    );
    expect(items.map((item) => item.key)).toEqual(["h1", "cash-USD", "cash-SGD"]);
    expect(items[2]?.nativeAmount).toBe("200");
    expect(items[2]?.nativeCurrency).toBe("SGD");
    expect(items.reduce((sum, item) => sum + item.shareBps, 0)).toBe(10000);
  });

  it("omits unavailable components from the donut", () => {
    const items = accountCompositionItems(
      [
        component({ nativeAmount: "800", nativeCurrency: "USD" }),
        component({ nativeCurrency: "SGD", nativeAmount: "200", available: false }),
      ],
      "USD",
    );
    expect(items.map((item) => item.key)).toEqual(["cash-USD"]);
  });
});

describe("sortValuationComponents", () => {
  it("puts larger household amounts first and missing conversion last", () => {
    const sorted = sortValuationComponents(
      [
        component({ nativeCurrency: "SGD", nativeAmount: "200", available: false }),
        component({ nativeCurrency: "USD", nativeAmount: "100" }),
        component({ nativeCurrency: "USD", nativeAmount: "400" }),
      ],
      "USD",
    );
    expect(sorted.map((item) => item.nativeAmount)).toEqual(["400", "100", "200"]);
  });
});

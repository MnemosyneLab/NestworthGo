import { describe, expect, it } from "vitest";
import { sortValuationComponents } from "./accountComposition";
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

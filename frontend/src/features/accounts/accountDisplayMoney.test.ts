import { describe, expect, it } from "vitest";
import { accountDisplayMoney } from "@/features/accounts/accountDisplayMoney";
import type { AccountRecordDTO, AccountValuationDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

function record(trackingMode: string, latestValue?: { amount: { amount: string; currency: string } }): AccountRecordDTO {
  return { account: { trackingMode }, latestValue } as AccountRecordDTO;
}

describe("accountDisplayMoney", () => {
  it("uses only the authoritative base total for a composite account", () => {
    const valuation = {
      baseValue: { amount: "35706.45", currency: "CNY" },
      components: [
        { nativeAmount: "3542.10", nativeCurrency: "USD" },
        { nativeAmount: "2000", nativeCurrency: "SGD" },
      ],
    } as AccountValuationDTO;

    expect(accountDisplayMoney(record("holdings"), valuation)).toEqual({
      primary: { amount: "35706.45", currency: "CNY" },
      pendingConversion: false,
    });
  });

  it("does not turn the first composite component into an account total", () => {
    const valuation = {
      components: [{ nativeAmount: "3542.10", nativeCurrency: "USD" }],
    } as AccountValuationDTO;

    expect(accountDisplayMoney(record("holdings"), valuation)).toEqual({
      primary: undefined,
      pendingConversion: true,
    });
  });

  it("keeps a simple account's native value primary and converted value secondary", () => {
    const valuation = { baseValue: { amount: "710", currency: "CNY" } } as AccountValuationDTO;

    expect(accountDisplayMoney(record("balance", { amount: { amount: "100", currency: "USD" } }), valuation)).toEqual({
      primary: { amount: "100", currency: "USD" },
      secondary: { amount: "710", currency: "CNY" },
      pendingConversion: false,
    });
  });
});

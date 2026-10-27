import { describe, expect, it } from "vitest";
import { accountDisplayMoney } from "./accountDisplayMoney";
import type { AccountRecordDTO, AccountValuationDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

const record = (trackingMode = "holdings", currency = "USD", nativeCurrency = currency) => ({
  account: { trackingMode, defaultCurrency: currency },
  latestValue: { amount: { amount: "100", currency: nativeCurrency } },
}) as AccountRecordDTO;
const valuation = (accountCurrency = "USD", householdCurrency = "CNY") => ({
  baseValue: { amount: "710", currency: householdCurrency }, complete: true,
  displayValue: { currency: accountCurrency, value: { amount: "100", currency: accountCurrency }, complete: true },
  components: [{ nativeAmount: "900", nativeCurrency: "SGD" }], missingInputs: [],
}) as unknown as AccountValuationDTO;

describe("accountDisplayMoney", () => {
  it.each(["holdings", "balance", "manual_value"])("uses backend account currency primary for %s", (mode) => {
    const result = accountDisplayMoney(record(mode), valuation());
    expect(result.primary).toEqual({ amount: "100", currency: "USD" });
    expect(result.secondary).toEqual({ amount: "710", currency: "CNY" });
    expect(result.primaryPartial).toBe(false);
    expect(result.secondaryPartial).toBe(false);
  });
  it("uses one line when currencies match", () => {
    expect(accountDisplayMoney(record(), valuation("USD", "USD")).secondary).toBeUndefined();
  });
  it("does not promote household currency or the first component when account FX is missing", () => {
    const value = valuation();
    value.displayValue = { ...value.displayValue!, value: undefined, complete: false };
    const result = accountDisplayMoney(record(), value);
    expect(result.primary).toBeUndefined();
    expect(result.unavailableCurrency).toBe("USD");
    expect(result.secondary?.currency).toBe("CNY");
  });
  it("marks independent partial totals and preserves a known zero", () => {
    const value = valuation();
    value.displayValue = { ...value.displayValue!, value: { amount: "0", currency: "USD" }, complete: false };
    value.complete = false;
    const result = accountDisplayMoney(record(), value);
    expect(result.primary?.amount).toBe("0");
    expect(result.primaryPartial).toBe(true);
    expect(result.secondaryPartial).toBe(true);
  });
  it("retains complete account currency when household FX is missing", () => {
    const value = valuation();
    value.baseValue = undefined;
    value.complete = false;
    value.missingInputs = [{ kind: "fx_rate" }] as AccountValuationDTO["missingInputs"];
    const result = accountDisplayMoney(record(), value);
    expect(result.primary?.amount).toBe("100");
    expect(result.primaryPartial).toBe(false);
    expect(result.pendingConversion).toBe(true);
  });
  it("does not relabel a mismatched latest-value currency as the account currency", () => {
    expect(accountDisplayMoney(record("balance", "USD", "SGD")).primary).toBeUndefined();
    expect(accountDisplayMoney(record("manual_value", "USD", "SGD"), valuation()).primary?.currency).toBe("USD");
  });
  it("can show a matching recorded simple value while valuation loads", () => {
    expect(accountDisplayMoney(record("balance")).primary).toEqual({ amount: "100", currency: "USD" });
    expect(accountDisplayMoney(record()).primary).toBeUndefined();
  });
});

it("keeps same-currency known zero on one line even when household conversion lacks an unnecessary zero FX quote", () => {
  const value = valuation("USD", "USD");
  value.baseValue = undefined;
  value.complete = false;
  value.missingInputs = [{ kind: "fx_rate", baseCurrency: "USD", quoteCurrency: "EUR" }] as AccountValuationDTO["missingInputs"];
  value.displayValue = { ...value.displayValue!, value: { amount: "0", currency: "USD" } };
  const result = accountDisplayMoney(record(), value);
  expect(result.primary?.amount).toBe("0");
  expect(result.secondary).toBeUndefined();
  expect(result.pendingConversion).toBe(false);
});

it("shows liability balances as negative without changing ledger values or inventing missing amounts", () => {
  const record = { account: { id: "debt", trackingMode: "balance", balanceSheetRole: "liability", defaultCurrency: "USD" }, latestValue: { amount: { amount: "20.005", currency: "USD" } } } as Parameters<typeof accountDisplayMoney>[0];
  expect(accountDisplayMoney(record).primary?.amount).toBe("-20.005");
  expect(record!.latestValue!.amount.amount).toBe("20.005");
  expect(accountDisplayMoney({ ...record!, latestValue: null }).primary).toBeUndefined();
  expect(accountDisplayMoney({ ...record!, latestValue: { ...record!.latestValue!, amount: { amount: "0.000", currency: "USD" } } }).primary?.amount).toBe("0");
});

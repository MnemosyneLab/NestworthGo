import { signedAccountAmount } from "@/lib/liabilityDisplay";
import type { AccountRecordDTO, AccountValuationDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

type DisplayMoney = { amount: string; currency: string };

/** Display only authoritative totals; never add or convert money in the UI. */
export function accountDisplayMoney(
  record?: AccountRecordDTO,
  valuation?: AccountValuationDTO,
): { primary?: DisplayMoney; secondary?: DisplayMoney; pendingConversion: boolean; primaryPartial: boolean; secondaryPartial: boolean; unavailableCurrency?: string } {
  const currency = record?.account.defaultCurrency ?? valuation?.account?.defaultCurrency;
  const display = valuation?.displayValue;
  const base = valuation?.baseValue ?? undefined;
  // Before the projection arrives, a recorded simple value already in the
  // account currency is safe. A household total is safe only in that currency.
  const recorded = record?.account.trackingMode !== "holdings" ? record?.latestValue?.amount : undefined;
  const fallback = recorded?.currency === currency ? recorded : base?.currency === currency ? base : undefined;
  const primary = display ? display.value ?? undefined : fallback;
  const secondary = base && base.currency !== currency ? base : undefined;
  const liability = record?.account.balanceSheetRole === "liability" || valuation?.account?.balanceSheetRole === "liability";
  return {
    primary: liability && primary ? { ...primary, amount: signedAccountAmount(primary.amount, true) } : primary,
    secondary: liability && secondary ? { ...secondary, amount: signedAccountAmount(secondary.amount, true) } : secondary,
    pendingConversion: Boolean(primary && !base && valuation && valuation.missingInputs?.some((input) => input.kind === "fx_rate" && input.baseCurrency !== currency)),
    primaryPartial: Boolean(primary && (display ? !display.complete : primary === base && valuation && !valuation.complete)),
    secondaryPartial: Boolean(secondary && valuation && !valuation.complete),
    unavailableCurrency: !primary && valuation ? currency : undefined,
  };
}

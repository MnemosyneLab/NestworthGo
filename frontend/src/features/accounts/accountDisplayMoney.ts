import type { AccountRecordDTO, AccountValuationDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

type DisplayMoney = { amount: string; currency: string };

/**
 * Composite accounts can contain several cash and holding currencies, so only
 * the backend's baseValue is an authoritative account total. Simple accounts
 * retain their recorded native value as the primary amount.
 */
export function accountDisplayMoney(
  record?: AccountRecordDTO,
  valuation?: AccountValuationDTO,
): { primary?: DisplayMoney; secondary?: DisplayMoney; pendingConversion: boolean } {
  const base = valuation?.baseValue ?? undefined;
  if (record?.account.trackingMode === "holdings") {
    return {
      primary: base,
      pendingConversion: !base && (valuation?.components ?? []).some((component) => component.nativeAmount),
    };
  }

  const native = record?.latestValue?.amount ?? undefined;
  return {
    primary: native ?? base,
    secondary: native && base && native.currency !== base.currency ? base : undefined,
    pendingConversion: Boolean(native && !base),
  };
}

import type { AccountRecordDTO, AccountValuationDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export function nativeMoney(
  record?: AccountRecordDTO,
  valuation?: AccountValuationDTO,
): { amount: string; currency: string } | undefined {
  const latest = record?.latestValue?.amount;
  if (latest?.amount) {
    return { amount: latest.amount, currency: latest.currency };
  }
  const component = (valuation?.components ?? []).find((item) => item.nativeAmount);
  if (component?.nativeAmount) {
    return { amount: component.nativeAmount, currency: component.nativeCurrency };
  }
  return undefined;
}

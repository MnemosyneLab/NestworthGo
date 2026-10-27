import { compareCanonical } from "./money";

/** Ledger balances store debt magnitudes. Apply the sign once, at display. */
export function signedAccountAmount(amount: string, liability: boolean): string {
  if (compareCanonical(amount, "0") === 0) return "0";
  return liability && !amount.startsWith("-") ? `-${amount}` : amount;
}

export function negateDisplayAmount(amount: string): string {
  if (compareCanonical(amount, "0") === 0) return "0";
  return amount.startsWith("-") ? amount.slice(1) : `-${amount}`;
}
export function signedAccountMoney<T extends { amount: string }>(money: T | null | undefined, liability: boolean): T | null | undefined {
  return money ? { ...money, amount: signedAccountAmount(money.amount, liability) } : money;
}
export function signedAccountValuation<T extends { total?: { amount: string } | null; knownSubtotal?: { amount: string } | null }>(value: T | null | undefined, liability: boolean): T | null | undefined {
  return value ? { ...value, total: signedAccountMoney(value.total, liability), knownSubtotal: signedAccountMoney(value.knownSubtotal, liability) } : value;
}

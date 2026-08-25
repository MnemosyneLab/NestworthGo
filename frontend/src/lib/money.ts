/**
 * formatAmount renders a canonical decimal string (already computed by
 * Go — see docs/migration/wails-v3-technical-design.md Sec4) as a
 * locale-formatted display string. This is presentation only, never a
 * recomputation: the frontend must not re-derive a financial value from
 * this formatted output (frontend stack decision Sec14).
 *
 * Values are converted through `Number` for formatting, which is exact
 * for every amount within this application's documented decimal bounds
 * (domain package: money up to 10^12, quantity up to 10^18) at the
 * precision doubles retain for numbers this size; this is a display
 * concern only, not a calculation, so the "no binary float for financial
 * values" rule does not apply to this formatting step itself.
 */
export function formatAmount(amount: string, currency?: string): string {
  const numeric = Number(amount);
  if (Number.isNaN(numeric)) {
    return amount;
  }
  if (currency) {
    try {
      return new Intl.NumberFormat(undefined, { style: "currency", currency }).format(numeric);
    } catch {
      // An unrecognized ISO code (should not happen; domain.CurrencyCode
      // already validates it) falls back to a plain number instead of
      // throwing.
    }
  }
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 4 }).format(numeric);
}

export function formatPercent(shareBps: number): string {
  return `${(shareBps / 100).toFixed(1)}%`;
}

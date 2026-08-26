import i18n from "@/i18n";

const CANONICAL_DECIMAL = /^-?(?:0|[1-9]\d*)(?:\.\d+)?$/;
const NUMERIC_PARTS = new Set(["integer", "group", "decimal", "fraction"]);

function numericAffixes(formatter: Intl.NumberFormat, sample: number) {
  const parts = formatter.formatToParts(sample);
  const firstNumberPart = parts.findIndex((part) => part.type === "integer");
  let lastNumberPart = firstNumberPart;
  parts.forEach((part, index) => {
    if (NUMERIC_PARTS.has(part.type)) {
      lastNumberPart = index;
    }
  });
  return {
    prefix: parts.slice(0, firstNumberPart).map((part) => part.value).join(""),
    suffix: parts.slice(lastNumberPart + 1).map((part) => part.value).join(""),
  };
}

function groupInteger(integer: string, separator: string): string {
  let grouped = "";
  for (let end = integer.length; end > 0; end -= 3) {
    const start = Math.max(0, end - 3);
    grouped = `${integer.slice(start, end)}${grouped ? separator + grouped : ""}`;
  }
  return grouped;
}

/**
 * formatAmount renders a canonical decimal string (already computed by
 * Go — see docs/architecture/data-and-ipc-contracts.md) as a
 * locale-formatted display string. This is presentation only, never a
 * recomputation: the frontend must not re-derive a financial value from
 * this formatted output.
 *
 * The canonical digits stay strings throughout this function. Intl is used
 * only to discover locale presentation tokens and currency placement, never
 * to parse the financial value through a binary floating-point Number.
 */
export function formatAmount(amount: string, currency?: string): string {
  if (!CANONICAL_DECIMAL.test(amount)) {
    return amount;
  }

  const negative = amount.startsWith("-");
  const unsigned = negative ? amount.slice(1) : amount;
  const [integerPart, fractionPart] = unsigned.split(".");
  const locale = i18n.language || "en";
  const numberFormatter = new Intl.NumberFormat(locale, {
    useGrouping: true,
    maximumFractionDigits: 20,
  });
  const sampleParts = numberFormatter.formatToParts(1234567.89);
  const groupingSeparator = sampleParts.find((part) => part.type === "group")?.value ?? ",";
  const decimalSeparator = sampleParts.find((part) => part.type === "decimal")?.value ?? ".";
  const number = `${groupInteger(integerPart, groupingSeparator)}${fractionPart ? decimalSeparator + fractionPart : ""}`;

  if (currency) {
    try {
      const currencyDefaults = new Intl.NumberFormat(locale, { style: "currency", currency });
      const minimumFractionDigits = currencyDefaults.resolvedOptions().minimumFractionDigits ?? 0;
      const currencyFormatter = new Intl.NumberFormat(locale, {
        style: "currency",
        currency,
        minimumFractionDigits,
        maximumFractionDigits: 20,
      });
      const affixes = numericAffixes(currencyFormatter, negative ? -1234.5 : 1234.5);
      const currencyFraction = (fractionPart ?? "").padEnd(minimumFractionDigits, "0");
      const currencyNumber = `${groupInteger(integerPart, groupingSeparator)}${currencyFraction ? decimalSeparator + currencyFraction : ""}`;
      return `${affixes.prefix}${currencyNumber}${affixes.suffix}`;
    } catch {
      // An unrecognized ISO code (should not happen; domain.CurrencyCode
      // already validates it) falls back to a plain number instead of
      // throwing.
    }
  }
  return `${negative ? "-" : ""}${number}`;
}

export function formatPercent(shareBps: number): string {
  return `${(shareBps / 100).toFixed(1)}%`;
}

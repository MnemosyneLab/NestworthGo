import i18n from "@/i18n";

const CANONICAL_DECIMAL = /^-?(?:0|[1-9]\d*)(?:\.\d+)?$/;
const NUMERIC_PARTS = new Set(["integer", "group", "decimal", "fraction"]);

// ISO 4217 minor-unit scale for the closed currency catalog. Keep in sync
// with internal/domain.CurrencyFractionDigits. Unknown codes fall back to
// Intl, then to 2.
const CURRENCY_FRACTION_DIGITS: Record<string, number> = {
  AUD: 2,
  CHF: 2,
  CNY: 2,
  EUR: 2,
  GBP: 2,
  HKD: 2,
  JPY: 0,
  KRW: 0,
  SGD: 2,
  TWD: 2,
  USD: 2,
};

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

function lastDigitIsOdd(integer: string, fraction: string): boolean {
  const digits = fraction || integer;
  return (digits.charCodeAt(digits.length - 1) - 48) % 2 === 1;
}

function incrementDecimal(integer: string, fraction: string): [string, string] {
  const fractionLength = fraction.length;
  const digits = [...`${integer}${fraction}`];
  for (let index = digits.length - 1; index >= 0; index -= 1) {
    if (digits[index] < "9") {
      digits[index] = String.fromCharCode(digits[index].charCodeAt(0) + 1);
      break;
    }
    digits[index] = "0";
    if (index === 0) {
      digits.unshift("1");
      break;
    }
  }
  let integerLength = digits.length - fractionLength;
  if (integerLength < 1) {
    integerLength = 1;
  }
  return [digits.slice(0, integerLength).join(""), digits.slice(integerLength).join("")];
}

function roundDecimal(integer: string, fraction: string, places: number): [string, string] {
  const scale = Math.max(0, places);
  if (fraction.length <= scale) {
    return [integer, fraction.padEnd(scale, "0")];
  }
  let kept = fraction.slice(0, scale);
  const next = fraction[scale];
  const restNonZero = fraction.slice(scale + 1).replace(/0+$/, "") !== "";
  const shouldRound = next > "5" || (next === "5" && (restNonZero || lastDigitIsOdd(integer, kept)));
  if (shouldRound) {
    [integer, kept] = incrementDecimal(integer, kept);
  }
  return [integer, kept.padEnd(scale, "0")];
}

export function currencyFractionDigits(currency: string, locale = i18n.language || "en"): number {
  if (currency in CURRENCY_FRACTION_DIGITS) {
    return CURRENCY_FRACTION_DIGITS[currency] ?? 2;
  }
  try {
    const resolved = new Intl.NumberFormat(locale, { style: "currency", currency }).resolvedOptions();
    return resolved.maximumFractionDigits ?? resolved.minimumFractionDigits ?? 2;
  } catch {
    return 2;
  }
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
 * When a currency code is supplied, the amount is rounded to that
 * currency's ISO 4217 fraction digits (2 for CNY/SGD, 0 for JPY/KRW).
 */
export function formatAmount(amount: string, currency?: string): string {
  if (!CANONICAL_DECIMAL.test(amount)) {
    return amount;
  }

  const negative = amount.startsWith("-");
  const unsigned = negative ? amount.slice(1) : amount;
  const [rawInteger, rawFraction = ""] = unsigned.split(".");
  const locale = i18n.language || "en";
  const numberFormatter = new Intl.NumberFormat(locale, {
    useGrouping: true,
    maximumFractionDigits: 20,
  });
  const sampleParts = numberFormatter.formatToParts(1234567.89);
  const groupingSeparator = sampleParts.find((part) => part.type === "group")?.value ?? ",";
  const decimalSeparator = sampleParts.find((part) => part.type === "decimal")?.value ?? ".";

  if (currency) {
    try {
      const fractionDigits = currencyFractionDigits(currency, locale);
      const [integerPart, fractionPart] = roundDecimal(rawInteger, rawFraction, fractionDigits);
      const currencyFormatter = new Intl.NumberFormat(locale, {
        style: "currency",
        currency,
        minimumFractionDigits: fractionDigits,
        maximumFractionDigits: fractionDigits,
      });
      const affixes = numericAffixes(currencyFormatter, negative ? -1234.5 : 1234.5);
      const currencyNumber = `${groupInteger(integerPart, groupingSeparator)}${fractionPart ? decimalSeparator + fractionPart : ""}`;
      return `${affixes.prefix}${currencyNumber}${affixes.suffix}`;
    } catch {
      // An unrecognized ISO code (should not happen; domain.CurrencyCode
      // already validates it) falls back to a plain number instead of
      // throwing.
    }
  }
  const number = `${groupInteger(rawInteger, groupingSeparator)}${rawFraction ? decimalSeparator + rawFraction : ""}`;
  return `${negative ? "-" : ""}${number}`;
}

export function formatPercent(shareBps: number): string {
  return `${(shareBps / 100).toFixed(1)}%`;
}

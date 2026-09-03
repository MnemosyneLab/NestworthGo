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

type DecimalParts = { negative: boolean; integer: string; fraction: string };

function parseCanonicalParts(value: string): DecimalParts | undefined {
  if (!CANONICAL_DECIMAL.test(value)) {
    return undefined;
  }
  const negative = value.startsWith("-");
  const unsigned = negative ? value.slice(1) : value;
  const [integer, fraction = ""] = unsigned.split(".");
  return { negative, integer, fraction };
}

function pow10(exponent: number): bigint {
  return 10n ** BigInt(Math.max(0, exponent));
}

function roundedRational(numerator: bigint, denominator: bigint, places: number): string {
  if (denominator === 0n) {
    return "";
  }
  const negative = numerator < 0n;
  const absoluteNumerator = negative ? -numerator : numerator;
  const scale = Math.max(0, places);
  const scaled = absoluteNumerator * pow10(scale);
  let quotient = scaled / denominator;
  const remainder = scaled % denominator;
  const doubledRemainder = remainder * 2n;
  if (doubledRemainder > denominator || (doubledRemainder === denominator && quotient % 2n === 1n)) {
    quotient += 1n;
  }
  const digits = quotient.toString().padStart(scale + 1, "0");
  const integer = scale === 0 ? digits : digits.slice(0, -scale);
  const fraction = scale === 0 ? "" : digits.slice(-scale).replace(/0+$/, "");
  const unsigned = fraction ? `${integer}.${fraction}` : integer;
  return negative && quotient !== 0n ? `-${unsigned}` : unsigned;
}

/**
 * Multiplies two canonical decimal strings using integer arithmetic only.
 * The result is rounded to `places` with ties-to-even (banker's rounding).
 */
export function multiplyCanonical(left: string, right: string, places = 20): string {
  const first = parseCanonicalParts(left);
  const second = parseCanonicalParts(right);
  if (!first || !second) {
    return "";
  }
  const numerator = BigInt(`${first.integer}${first.fraction}`) * BigInt(`${second.integer}${second.fraction}`);
  const signedNumerator = first.negative !== second.negative ? -numerator : numerator;
  return roundedRational(signedNumerator, pow10(first.fraction.length + second.fraction.length), places);
}

/**
 * Adds two canonical decimal strings using integer arithmetic only.
 * The result keeps the larger input scale and then strips trailing zeros.
 */
export function addCanonical(left: string, right: string): string {
  const first = parseCanonicalParts(left);
  const second = parseCanonicalParts(right);
  if (!first || !second) {
    return "";
  }
  const scale = Math.max(first.fraction.length, second.fraction.length);
  const leftValue = BigInt(first.integer + first.fraction.padEnd(scale, "0"));
  const rightValue = BigInt(second.integer + second.fraction.padEnd(scale, "0"));
  const sum = (first.negative ? -leftValue : leftValue) + (second.negative ? -rightValue : rightValue);
  const negative = sum < 0n;
  const absolute = negative ? -sum : sum;
  const digits = absolute.toString().padStart(scale + 1, "0");
  const integer = scale === 0 ? digits : digits.slice(0, -scale);
  const fraction = scale === 0 ? "" : digits.slice(-scale);
  const unsigned = canonicalDecimal(fraction.replace(/0+$/, "") ? `${integer}.${fraction}` : integer);
  return negative && unsigned !== "0" ? `-${unsigned}` : unsigned;
}

/**
 * Divides two canonical decimal strings using integer arithmetic only.
 * The result is rounded to `places` with ties-to-even (banker's rounding).
 */
export function divideCanonical(left: string, right: string, places = 20): string {
  const first = parseCanonicalParts(left);
  const second = parseCanonicalParts(right);
  if (!first || !second || BigInt(`${second.integer}${second.fraction}`) === 0n) {
    return "";
  }
  const numerator = BigInt(`${first.integer}${first.fraction}`) * pow10(second.fraction.length);
  const denominator = BigInt(`${second.integer}${second.fraction}`) * pow10(first.fraction.length);
  const signedNumerator = first.negative !== second.negative ? -numerator : numerator;
  return roundedRational(signedNumerator, denominator, places);
}

export function canonicalDecimal(value: string): string {
  const parts = parseCanonicalParts(value);
  if (!parts) {
    return value;
  }
  const integer = parts.integer.replace(/^0+(?=\d)/, "");
  const fraction = parts.fraction.replace(/0+$/, "");
  const unsigned = fraction ? `${integer}.${fraction}` : integer;
  return parts.negative && unsigned !== "0" ? `-${unsigned}` : unsigned;
}

export function sameCanonicalDecimal(left: string, right: string): boolean {
  return canonicalDecimal(left) === canonicalDecimal(right);
}

/**
 * Sorts items by canonical amount descending. Equal amounts keep a stable
 * tie-break via `tieBreak` so UI lists do not shuffle across renders.
 */
export function sortByCanonicalDesc<T>(
  items: readonly T[],
  amount: (item: T) => string | undefined,
  tieBreak?: (left: T, right: T) => number,
): T[] {
  return [...items].sort((left, right) => {
    const comparison = compareCanonical(amount(right) ?? "0", amount(left) ?? "0");
    if (comparison !== 0) {
      return comparison;
    }
    return tieBreak?.(left, right) ?? 0;
  });
}

/** Compares two canonical decimal strings without converting through Number. */
export function compareCanonical(left: string, right: string): number {
  const first = parseCanonicalParts(left);
  const second = parseCanonicalParts(right);
  if (!first && !second) {
    return 0;
  }
  if (!first) {
    return -1;
  }
  if (!second) {
    return 1;
  }
  if (first.negative !== second.negative) {
    return first.negative ? -1 : 1;
  }
  const scale = Math.max(first.fraction.length, second.fraction.length);
  const leftValue = BigInt(first.integer + first.fraction.padEnd(scale, "0"));
  const rightValue = BigInt(second.integer + second.fraction.padEnd(scale, "0"));
  if (leftValue === rightValue) {
    return 0;
  }
  const comparison = leftValue < rightValue ? -1 : 1;
  return first.negative ? -comparison : comparison;
}

export function isPositiveCanonical(value: string): boolean {
  const normalized = canonicalDecimal(value.trim());
  return normalized !== "" && normalized !== "0" && !normalized.startsWith("-");
}

const TOTAL_SHARE_BPS = 10000n;

/**
 * Allocates integer basis-point shares that sum to 10000. Remainders go to
 * the largest fractional parts so pie-chart percents stay exact.
 */
export function allocateShareBps(amounts: string[]): number[] {
  if (amounts.length === 0) {
    return [];
  }
  const parsed = amounts.map((amount) => parseCanonicalParts(amount));
  if (parsed.some((part) => !part || part.negative)) {
    return amounts.map(() => 0);
  }
  const scale = Math.max(0, ...parsed.map((part) => part!.fraction.length));
  const values = parsed.map((part) => BigInt(part!.integer + part!.fraction.padEnd(scale, "0")));
  let total = 0n;
  for (const value of values) {
    total += value;
  }
  if (total === 0n) {
    return amounts.map(() => 0);
  }
  const floors: number[] = [];
  const remainders: { index: number; remainder: bigint }[] = [];
  let allocated = 0;
  for (let index = 0; index < values.length; index += 1) {
    const scaled = values[index] * TOTAL_SHARE_BPS;
    const floor = Number(scaled / total);
    floors.push(floor);
    allocated += floor;
    remainders.push({ index, remainder: scaled % total });
  }
  remainders.sort((left, right) => {
    if (left.remainder === right.remainder) {
      return left.index - right.index;
    }
    return left.remainder > right.remainder ? -1 : 1;
  });
  const remaining = Number(TOTAL_SHARE_BPS) - allocated;
  for (let index = 0; index < remaining && index < remainders.length; index += 1) {
    floors[remainders[index].index] += 1;
  }
  return floors;
}

/**
 * percentToShareBps converts a user-entered percent string into integer
 * basis points with exact decimal arithmetic. 1% is 100 bps. Ties round to
 * even at the integer, matching multiplyCanonical. Invalid or negative
 * input becomes 0; Go ParseOwnership remains authoritative.
 */
export function percentToShareBps(percent: string): number {
  const product = multiplyCanonical(percent.trim(), "100", 0);
  if (!product || product.startsWith("-") || !/^\d+$/.test(product)) {
    return 0;
  }
  const value = Number(product);
  return Number.isSafeInteger(value) ? value : 0;
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

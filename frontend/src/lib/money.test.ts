import { afterEach, describe, expect, it } from "vitest";
import i18n from "@/i18n";
import { resetDisplayPreferences, setDisplayPreferences } from "./displayPreferences";
import { addCanonical, allocateShareBps, divideCanonical, formatAmount, formatAnnualRatePercent, formatAuditMoney, formatCount, formatDecimalPercent, formatDecimalRatioPercent, formatPercent, formatQuantity, formatSignedMoney, multiplyCanonical, percentToShareBps, sortByCanonicalDesc } from "./money";

afterEach(async () => {
  resetDisplayPreferences();
  await i18n.changeLanguage("en");
});

describe("formatAmount", () => {
  it.each(["en"])("shows exact signed subminor audit amounts in %s", async language => {
    await i18n.changeLanguage(language);
    for (const currency of ["USD", "CNY", "JPY", "KRW"]) {
      for (const amount of ["0.0001", "-0.0001"]) {
        const shown = formatAuditMoney(amount, currency);
        expect(shown).toContain(`${amount} ${currency}`);
        expect(shown).toContain(formatAmount(amount, currency));
      }
    }
    expect(formatAuditMoney("1.20", "USD")).toBe(formatAmount("1.20", "USD"));
  });
  it.each([
    ["en", "9007199254740993", "9,007,199,254,740,993"],
    ["en", "123456789012345678", "123,456,789,012,345,678"],
    ["en", "-123456789012345678.12345678", "-123,456,789,012,345,678.12345678"],
  ])("preserves canonical digits for %s %s", async (language, amount, expected) => {
    await i18n.changeLanguage(language);
    expect(formatAmount(amount)).toBe(expected);
  });

  it.each([
    ["en", "US$1,234.56"],
    ["zh-CN", "US$1,234.56"],
    ["zh-TW", "US$1,234.56"],
  ])("uses explicit locale currency presentation for %s", async (language, expected) => {
    await i18n.changeLanguage(language);
    expect(formatAmount("1234.56", "USD")).toBe(expected);
  });

  it("keeps dollar currencies distinguishable from a bare dollar sign", async () => {
    for (const language of ["en", "zh-CN", "zh-TW"]) {
      await i18n.changeLanguage(language);
      for (const currency of ["AUD", "CAD", "NZD", "SGD", "HKD", "TWD", "USD"]) {
        const shown = formatAmount("1234.5", currency);
        expect(shown, `${language} ${currency} ${shown}`).not.toMatch(/(^|[^A-Z])\$(?=\d)/);
      }
    }
    await i18n.changeLanguage("zh-TW");
    expect(formatAmount("1234.5", "TWD")).toContain("NT$");
    expect(formatAmount("1234.5", "HKD")).toContain("HK$");
    expect(formatAmount("1234.5", "SGD")).toContain("SGD");
  });

  it.each([
    ["CNY", "1234", /1,234\.00/],
    ["CNY", "1234.5", /1,234\.50/],
    ["SGD", "1234.5", /1,234\.50/],
    ["USD", "1234.567", /1,234\.57/],
  ])("pads and rounds %s to two fraction digits", async (currency, amount, pattern) => {
    await i18n.changeLanguage("en");
    expect(formatAmount(amount, currency)).toMatch(pattern);
  });

  it.each(["JPY", "KRW"])("uses the ISO zero scale for %s", async (currency) => {
    await i18n.changeLanguage("en");
    const formatted = formatAmount("1234.56", currency);
    expect(formatted).toMatch(/1,235/);
    expect(formatted).not.toMatch(/\.56/);
    expect(formatAmount("1000", currency)).not.toMatch(/\.00/);
    expect(formatAmount("1234.00", currency)).not.toMatch(/\./);
  });

  it("keeps zero and invalid values stable", () => {
    expect(formatAmount("0")).toBe("0");
    expect(formatAmount("not-a-decimal")).toBe("not-a-decimal");
  });

  it("round-trips canonical digits after stripping grouping and currency tokens", async () => {
    const amounts = ["0", "9007199254740993", "123456789012345678", "-123456789012345678.12345678", "1234.56"];
    for (const language of ["en"]) {
      await i18n.changeLanguage(language);
      for (const amount of amounts) {
        expect(canonicalDigits(formatAmount(amount), language)).toBe(amount);
      }
    }
  });

  it("round-trips currency amounts at the currency scale", async () => {
    await i18n.changeLanguage("en");
    expect(canonicalDigits(formatAmount("1234.56", "USD"), "en")).toBe("1234.56");
    expect(canonicalDigits(formatAmount("1234.567", "USD"), "en")).toBe("1234.57");
    expect(canonicalDigits(formatAmount("1234.56", "JPY"), "en")).toBe("1235");
    expect(canonicalDigits(formatAmount("1000", "JPY"), "en")).toBe("1000");
  });
});

describe("canonical decimal defaults", () => {
  it.each([
    ["2", "3.5", 2, "7"],
    ["1.005", "1", 2, "1"],
    ["1.004", "1", 2, "1"],
    ["1234.6", "1", 0, "1235"],
    ["1.015", "1", 2, "1.02"],
  ])("multiplies and rounds %s × %s to %s places", (left, right, places, expected) => {
    expect(multiplyCanonical(left, right, places)).toBe(expected);
  });

  it.each([
    ["1", "8", 2, "0.12"],
    ["1", "40", 2, "0.02"],
    ["1", "200", 2, "0"],
    ["1", "3", 0, "0"],
  ])("divides and rounds %s ÷ %s to %s places", (left, right, places, expected) => {
    expect(divideCanonical(left, right, places)).toBe(expected);
  });

  it("uses ties-to-even at a half", () => {
    expect(multiplyCanonical("1.005", "1", 2)).toBe("1");
    expect(multiplyCanonical("1.015", "1", 2)).toBe("1.02");
    expect(divideCanonical("1", "400", 2)).toBe("0");
    expect(divideCanonical("3", "400", 2)).toBe("0.01");
  });

  it("adds canonical decimals without converting through Number", () => {
    expect(addCanonical("10", "20")).toBe("30");
    expect(addCanonical("1.5", "2.25")).toBe("3.75");
    expect(addCanonical("10", "-2.5")).toBe("7.5");
  });

  it("sorts by canonical amount descending with a stable key tie-break", () => {
    const sorted = sortByCanonicalDesc(
      [
        { key: "b", amount: "10" },
        { key: "a", amount: "50" },
        { key: "c", amount: "10" },
        { key: "d", amount: "30" },
      ],
      (item) => item.amount,
      (left, right) => left.key.localeCompare(right.key),
    );
    expect(sorted.map((item) => item.key)).toEqual(["a", "d", "b", "c"]);
  });

  it("allocates basis-point shares that sum to 10000", () => {
    expect(allocateShareBps(["50", "50"])).toEqual([5000, 5000]);
    const shares = allocateShareBps(["1", "1", "1"]);
    expect(shares.reduce((sum, share) => sum + share, 0)).toBe(10000);
    expect(Math.max(...shares) - Math.min(...shares)).toBeLessThanOrEqual(1);
  });

  it("converts ownership percents to basis points without JavaScript Number", () => {
    expect(percentToShareBps("70")).toBe(7000);
    expect(percentToShareBps("33.335")).toBe(3334);
    expect(percentToShareBps("66.665")).toBe(6666);
    expect(percentToShareBps("12.34567890123456789")).toBe(1235);
    expect(percentToShareBps("not-a-number")).toBe(0);
  });
});

function canonicalDigits(formatted: string, language: string): string {
  const sample = new Intl.NumberFormat(language, { useGrouping: true, maximumFractionDigits: 20 }).formatToParts(1234567.89);
  const grouping = sample.find((part) => part.type === "group")?.value ?? ",";
  const decimal = sample.find((part) => part.type === "decimal")?.value ?? ".";
  const negative = /[-−(]/.test(formatted);
  const escapedGrouping = grouping.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const escapedDecimal = decimal.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  let numeric = formatted.replace(new RegExp(`[^\\d${escapedDecimal}${escapedGrouping}-]`, "g"), "");
  numeric = numeric.replace(new RegExp(escapedGrouping, "g"), "");
  const [integerPart, fractionPart = ""] = numeric.replace(/-/g, "").split(decimal);
  const fraction = fractionPart.replace(/0+$/, "");
  const integer = (integerPart || "0").replace(/^0+(?=\d)/, "") || "0";
  const unsigned = fraction ? `${integer}.${fraction}` : integer;
  return negative && unsigned !== "0" ? `-${unsigned}` : unsigned;
}

it("keeps display scales separate from command scales and normalizes negative display zero", async () => {
  const { currencyFractionDigits, formatQuotePrice, formatMoney } = await import("./money");
  await i18n.changeLanguage("en");
  expect(currencyFractionDigits("JPY")).toBe(0);
  expect(formatAmount("12.3456", "JPY")).toBe("¥12");
  expect(formatAmount("1000", "JPY")).toBe("¥1,000");
  expect(formatAmount("1500", "KRW")).toBe("₩1,500");
  await i18n.changeLanguage("zh-CN");
  expect(formatAmount("1000", "JPY")).toBe("¥1,000");
  expect(formatAmount("1500", "KRW")).not.toMatch(/\.00/);
  await i18n.changeLanguage("en");
  expect(formatQuotePrice("1.1066", "USD")).toBe("US$1.1066");
  expect(formatQuotePrice("0.0000000012", "USD")).toBe("US$0.0000000012");
  expect(formatMoney("12.3456")).toBe("12.35");
  expect(formatAmount("-0.004", "USD")).toBe("US$0.00 · − nonzero, below US$0.01");
  expect(formatAmount("0.1", "JPY")).toBe("¥0 · + nonzero, below ¥1");
  expect(formatAmount("-0.1", "JPY")).toBe("¥0 · − nonzero, below ¥1");
  expect(formatAmount("-0.4", "KRW")).toBe("₩0 · − nonzero, below ₩1");
  await i18n.changeLanguage("zh-CN");
  expect(formatAmount("0.1", "JPY")).toBe("¥0 · + 非零，小于 ¥1");
  expect(formatAmount("-0.1", "JPY")).toBe("¥0 · − 非零，小于 ¥1");
  await i18n.changeLanguage("zh-TW");
  expect(formatAmount("-0.1", "JPY")).toBe("¥0 · − 非零，小於 ¥1");
  await i18n.changeLanguage("en");
  expect(formatAmount("-0.00", "USD")).toBe("US$0.00");
  expect(formatAmount("0.005", "USD")).toContain("US$0.00");
  expect(formatAmount("0.015", "USD")).toBe("US$0.02");
  expect(formatAmount("9".repeat(400), "USD")).not.toContain("Infinity");
});

it("keeps percentage presentation concise and signed rounded money zero neutral", () => {
 expect(formatDecimalPercent("33.3333333333333333333333")).toBe("33.33%");
 expect(formatDecimalPercent("-0.004")).toBe("0.00%");
 expect(formatSignedMoney("0.004", "USD")).toBe("US$0.00 · + nonzero, below US$0.01");
 expect(formatSignedMoney("-0.004", "USD")).toBe("US$0.00 · − nonzero, below US$0.01");
 expect(formatDecimalPercent("3.525")).toBe("3.52%");
 expect(formatSignedMoney("1.23", "USD")).toBe("+US$1.23");
});

it("formats return ratios with decimal half-even rounding and preserves display precision", () => {
  expect(formatDecimalRatioPercent("0.01015", 2, false, false)).toBe("1.02%");
  expect(formatDecimalRatioPercent("0.01005", 2, false, false)).toBe("1.00%");
  expect(formatDecimalRatioPercent("-0.01015", 2, false, false)).toBe("-1.02%");
  expect(formatDecimalRatioPercent("-0.01005", 2, false, false)).toBe("-1.00%");
  expect(formatDecimalRatioPercent("0.01015", 2, true)).toBe("+1.02%");
  expect(formatDecimalRatioPercent("-0.01015", 2, true)).toBe("-1.02%");
  expect(formatDecimalRatioPercent("0.00001", 2, true, false)).toBe("0.00%");
  expect(formatDecimalRatioPercent("0", 2, true, false)).toBe("0.00%");
  expect(formatDecimalRatioPercent(null)).toBe("—");
  expect(formatDecimalRatioPercent("not-a-rate")).toBe("—");
  expect(formatDecimalRatioPercent(1e-7, 2, false, false)).toBe("0.00%");
});

it("formats integer basis-point percentages with exact half-even rounding", () => {
  expect(formatPercent(15)).toBe("0.2%");
  expect(formatPercent(25)).toBe("0.2%");
  expect(formatPercent(35)).toBe("0.4%");
  expect(formatPercent(-25)).toBe("-0.2%");
});


it("formats quantities, counts, annual rates and separator settings without changing the canonical value", () => {
  expect(formatQuantity("126.5000")).toBe("126.5");
  expect(formatQuantity("1000")).toBe("1,000");
  expect(formatQuantity("-0.0100")).toBe("-0.01");
  expect(formatCount(1500)).toBe("1,500");
  expect(formatAnnualRatePercent("0.035")).toBe("3.5%");
  expect(formatAnnualRatePercent("0.03525")).toBe("3.525%");
  expect(formatAnnualRatePercent("0.0250")).toBe("2.5%");
  expect(formatAnnualRatePercent("0.1000")).toBe("10%");
  expect(formatAnnualRatePercent("-0.03525")).toBe("-3.525%");
  expect(formatAuditMoney("1000.00", "JPY")).toBe("¥1,000");
  expect(formatAuditMoney("1000.40", "JPY")).toBe("¥1,000 (1000.40 JPY)");
  setDisplayPreferences({ decimalSeparator: ",", groupingSeparator: "." });
  expect(formatAmount("1234.5", "EUR")).toBe("€1.234,50");
  setDisplayPreferences({ decimalSeparator: ".", groupingSeparator: "space" });
  expect(formatAmount("1234.5", "USD")).toBe("US$1 234.50");
  setDisplayPreferences({ groupingSeparator: "none" });
  expect(formatAmount("1234.5", "USD")).toBe("US$1234.50");
});

it.each([
  ["0.004", "0.008", "US$0.01", "+"],
  ["-0.004", "-0.008", "-US$0.01", "−"],
])("formats exact rows %s independently of the authoritative aggregate %s", (row, aggregate, total, direction) => {
  expect(formatAmount(row, "USD")).toBe(`US$0.00 · ${direction} nonzero, below US$0.01`);
  expect(formatAmount(aggregate, "USD")).toBe(total);
  expect(formatSignedMoney(aggregate, "USD")).toBe(aggregate.startsWith("-") ? total : `+${total}`);
});


it.each(["JPY", "KRW", "USD"])("adds a sign only to a nonzero currency-rounded %s amount", currency => {
  const boundary = currency === "USD" ? "0.005" : "0.5";
  const above = currency === "USD" ? "0.0050001" : "0.5000001";
  for (const value of ["0", "-0", "0.005", "0.1", boundary, `-${boundary}`]) {
    const nonzero = currency === "USD" && value === "0.1";
    expect(formatSignedMoney(value, currency)).toBe(`${nonzero ? "+" : ""}${formatAmount(value, currency)}`);
  }
  expect(formatSignedMoney(above, currency)).toBe(`+${formatAmount(above, currency)}`);
  expect(formatSignedMoney(`-${above}`, currency)).toBe(formatAmount(`-${above}`, currency));
  expect(formatSignedMoney(`-${boundary}`, currency)).toContain(" · − nonzero, below ");
  expect(formatSignedMoney(boundary, currency)).toContain(" · + nonzero, below ");
});

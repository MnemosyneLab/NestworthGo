import { afterEach, describe, expect, it } from "vitest";
import i18n from "@/i18n";
import { divideCanonical, formatAmount, multiplyCanonical } from "./money";

afterEach(async () => {
  await i18n.changeLanguage("en");
});

describe("formatAmount", () => {
  it.each([
    ["en", "9007199254740993", "9,007,199,254,740,993"],
    ["en", "123456789012345678", "123,456,789,012,345,678"],
    ["en", "-123456789012345678.12345678", "-123,456,789,012,345,678.12345678"],
    ["zh-CN", "9007199254740993", "9,007,199,254,740,993"],
    ["zh-CN", "123456789012345678", "123,456,789,012,345,678"],
    ["zh-CN", "-123456789012345678.12345678", "-123,456,789,012,345,678.12345678"],
    ["zh-TW", "9007199254740993", "9,007,199,254,740,993"],
    ["zh-TW", "123456789012345678", "123,456,789,012,345,678"],
    ["zh-TW", "-123456789012345678.12345678", "-123,456,789,012,345,678.12345678"],
  ])("preserves canonical digits for %s %s", async (language, amount, expected) => {
    await i18n.changeLanguage(language);
    expect(formatAmount(amount)).toBe(expected);
  });

  it.each([
    ["en", "$1,234.56"],
    ["zh-CN", "US$1,234.56"],
    ["zh-TW", "US$1,234.56"],
  ])("uses explicit locale currency presentation for %s", async (language, expected) => {
    await i18n.changeLanguage(language);
    expect(formatAmount("1234.56", "USD")).toBe(expected);
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

  it.each(["JPY", "KRW"])("hides fraction digits for zero-scale %s", async (currency) => {
    await i18n.changeLanguage("en");
    const formatted = formatAmount("1234.56", currency);
    expect(formatted).toMatch(/1,235/);
    expect(formatted).not.toMatch(/\.56/);
    expect(formatAmount("1234.00", currency)).not.toMatch(/\.00/);
  });

  it("keeps zero and invalid values stable", () => {
    expect(formatAmount("0")).toBe("0");
    expect(formatAmount("not-a-decimal")).toBe("not-a-decimal");
  });

  it("round-trips canonical digits after stripping grouping and currency tokens", async () => {
    const amounts = ["0", "9007199254740993", "123456789012345678", "-123456789012345678.12345678", "1234.56"];
    for (const language of ["en", "zh-CN", "zh-TW"]) {
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

  it("returns an empty default for invalid input or zero denominator", () => {
    expect(multiplyCanonical("", "2", 2)).toBe("");
    expect(divideCanonical("2", "0", 2)).toBe("");
    expect(divideCanonical("not-a-decimal", "2", 2)).toBe("");
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

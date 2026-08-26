import { afterEach, describe, expect, it } from "vitest";
import i18n from "@/i18n";
import { formatAmount } from "./money";

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
        expect(canonicalDigits(formatAmount(amount, "USD"), language)).toBe(amount);
      }
    }
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

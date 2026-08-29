import { describe, expect, it } from "vitest";
import { computeFilledSide, convertWithQuote, isPositiveMoney, tradeFillFromPrice } from "./CalculateAmounts";
import { canonicalDecimal } from "@/lib/money";
import type { FXQuoteDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

describe("isPositiveMoney", () => {
  it("rejects empty and zero amounts", () => {
    expect(isPositiveMoney("")).toBe(false);
    expect(isPositiveMoney("0")).toBe(false);
    expect(isPositiveMoney("0.00")).toBe(false);
  });

  it("accepts a positive canonical amount", () => {
    expect(isPositiveMoney("1.50")).toBe(true);
  });
});

describe("computeFilledSide", () => {
  it("fills B when only A is positive", () => {
    expect(computeFilledSide("10", "")).toBe("b");
  });

  it("fills A when only B is positive", () => {
    expect(computeFilledSide("", "10")).toBe("a");
  });

  it("does not change when both sides are positive", () => {
    expect(computeFilledSide("10", "20")).toBe("none");
  });

  it("blocks when both sides are empty or zero", () => {
    expect(computeFilledSide("", "")).toBe("both-empty");
    expect(computeFilledSide("0", "0.00")).toBe("both-empty");
  });
});

describe("convertWithQuote", () => {
  const quote = {
    baseCurrency: "USD",
    quoteCurrency: "CNY",
    rate: "7",
  } as FXQuoteDTO;

  it("multiplies along the quote direction", () => {
    expect(canonicalDecimal(convertWithQuote(quote, "10", "USD", "CNY"))).toBe("70");
  });

  it("divides against the quote direction", () => {
    expect(canonicalDecimal(convertWithQuote(quote, "70", "CNY", "USD"))).toBe("10");
  });
});

describe("tradeFillFromPrice", () => {
  it("fills gross from quantity", () => {
    expect(canonicalDecimal(tradeFillFromPrice("10", "", "1.5", "USD")?.gross ?? "")).toBe("15");
  });

  it("fills quantity from gross", () => {
    expect(canonicalDecimal(tradeFillFromPrice("", "15", "1.5", "USD", 8)?.quantity ?? "")).toBe("10");
  });
});

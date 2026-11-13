import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import i18n from "@/i18n";
import { formatAmount } from "@/lib/money";
import { MoneyText } from "./money-text";

afterEach(async () => { await i18n.changeLanguage("en"); });

describe("MoneyText narrow-card wrapping", () => {
  it("exposes distinct Japanese currency prefixes in the status text", async () => {
    await i18n.changeLanguage("ja");
    render(<div role="status"><MoneyText>{formatAmount("1234.56", "CNY")}</MoneyText>{" · "}<MoneyText>{formatAmount("1234", "JPY")}</MoneyText></div>);
    expect(screen.getByRole("status")).toHaveTextContent("CN¥1,234.56 · JP¥1,234");
  });
  it("keeps every digit and the currency available when wrapping is requested", () => {
    const amount = "-US$1,234,567,890,123.45";
    render(<MoneyText wrap size="xl">{amount}</MoneyText>);
    const money = screen.getByText(amount);
    expect(money).toHaveTextContent(amount);
    expect(money).toHaveClass("whitespace-normal", "[overflow-wrap:anywhere]");
    expect(money).not.toHaveClass("whitespace-nowrap", "truncate");
    expect(money).not.toHaveAttribute("wrap");
  });

  it("preserves the default single-line contract for table callers", () => {
    render(<MoneyText>US$123.45</MoneyText>);
    expect(screen.getByText("US$123.45")).toHaveClass("whitespace-nowrap");
  });
});

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MoneyText } from "./money-text";

describe("MoneyText narrow-card wrapping", () => {
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

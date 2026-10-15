import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import i18n from "@/i18n";
import { changeDirection, SignedChange } from "./SignedChange";

describe("changeDirection", () => {
  it("classifies exact decimals and treats amounts that display as zero as flat", () => {
    expect(changeDirection("12.5")).toBe("up");
    expect(changeDirection("-0.01")).toBe("down");
    expect(changeDirection("0.004")).toBe("flat");
    expect(changeDirection("-0.004")).toBe("flat");
    expect(changeDirection("0")).toBe("flat");
  });

  it("never turns missing or malformed values into a direction", () => {
    expect(changeDirection(null)).toBeNull();
    expect(changeDirection(undefined)).toBeNull();
    expect(changeDirection("abc")).toBeNull();
  });
});

describe("SignedChange", () => {
  it("adds a direction cue for gains and losses", () => {
    const { rerender, container } = render(<SignedChange value="10" currency="CNY" />);
    expect(container).toHaveTextContent(i18n.t("holdingPerformance.direction.up"));
    rerender(<SignedChange value="-10" currency="CNY" />);
    expect(container).toHaveTextContent(i18n.t("holdingPerformance.direction.down"));
  });

  it("stays neutral for balances that are not returns", () => {
    const { container } = render(<SignedChange value="10" currency="CNY" neutral />);
    expect(container).not.toHaveTextContent(i18n.t("holdingPerformance.direction.up"));
    expect(container.querySelector(".text-gain-positive")).toBeNull();
  });

  it("renders nothing for an unknown value", () => {
    const { container } = render(<SignedChange value={null} currency="CNY" />);
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByText(/\d/)).toBeNull();
  });
});

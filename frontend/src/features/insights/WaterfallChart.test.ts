import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { createElement } from "react";
import { stepsFromData, WaterfallChart, waterfallReconciles } from "./WaterfallChart";

afterEach(() => cleanup());

describe("stepsFromData", () => {
  it("keeps signed driver steps reconciled with the ending value", () => {
    const steps = stepsFromData(
      {
        beginningValue: { amount: "100", currency: "USD" },
        endingValue: { amount: "125", currency: "USD" },
        change: { amount: "25", currency: "USD" },
      },
      [
        { key: "income", label: "Income", bucket: "income", amount: { amount: "10", currency: "USD" } },
        { key: "price_change", label: "Price change", bucket: "price_change", amount: { amount: "20", currency: "USD" } },
        { key: "residual", label: "Unexplained difference", bucket: "residual", amount: { amount: "-5", currency: "USD" } },
      ],
      { beginning: "Beginning", ending: "Ending" },
    );

    expect(steps.map((step) => [step.key, step.start, step.end])).toEqual([
      ["beginning", "0", "100"],
      ["income", "100", "110"],
      ["price_change", "110", "130"],
      ["residual", "130", "125"],
      ["ending", "0", "125"],
    ]);
    expect(steps[steps.length - 2]?.end).toBe(steps[steps.length - 1]?.end);
    expect(waterfallReconciles(steps)).toBe(true);
  });

  it("reports an identity mismatch instead of treating the ending bar as reconciled", () => {
    const steps = stepsFromData(
      {
        beginningValue: { amount: "100", currency: "USD" },
        endingValue: { amount: "125", currency: "USD" },
      },
      [{ key: "income", label: "Income", bucket: "income", amount: { amount: "10", currency: "USD" } }],
      { beginning: "Beginning", ending: "Ending" },
    );
    expect(waterfallReconciles(steps)).toBe(false);
  });

  it("surfaces an identity warning when the ending bar does not reconcile", () => {
    render(createElement(WaterfallChart, {
      summary: { beginningValue: { amount: "100", currency: "USD" }, endingValue: { amount: "125", currency: "USD" } },
      rows: [{ key: "income", label: "Income", bucket: "income", amount: { amount: "10", currency: "USD" } }],
      labels: { beginning: "Beginning", ending: "Ending", amount: "Amount", empty: "No data" },
    }));
    expect(screen.getByRole("alert")).toHaveTextContent("The waterfall drivers do not reconcile to the ending value.");
  });
});

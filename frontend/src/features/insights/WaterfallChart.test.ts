import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { createElement } from "react";
import * as echarts from "echarts/core";
import { BarChart } from "echarts/charts";
import { GridComponent } from "echarts/components";
import { SVGRenderer } from "echarts/renderers";
import { stepsFromData, WaterfallChart, waterfallReconciles } from "./WaterfallChart";

echarts.use([BarChart, GridComponent, SVGRenderer]);

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

  it("does not invent an ending bar when the ending value is unknown", () => {
    const steps = stepsFromData(
      { beginningValue: { amount: "100", currency: "USD" } },
      [],
      { beginning: "Beginning", ending: "Ending" },
    );
    expect(steps.map((step) => step.key)).toEqual(["beginning"]);
    render(createElement(WaterfallChart, {
      summary: { beginningValue: { amount: "100", currency: "USD" } },
      rows: [],
      labels: { beginning: "Beginning", ending: "Ending", amount: "Amount", empty: "No data" },
    }));
    expect(screen.getByRole("alert")).toHaveTextContent("Ending value is unknown");
  });

  it("keeps signed start and end for negative and cross-zero intervals", () => {
    const negative = stepsFromData(
      {
        beginningValue: { amount: "-100", currency: "USD" },
        endingValue: { amount: "-80", currency: "USD" },
      },
      [{ key: "income", label: "Income", bucket: "income", amount: { amount: "20", currency: "USD" } }],
      { beginning: "Beginning", ending: "Ending" },
    );
    expect(negative.map((step) => [step.key, step.start, step.end, step.base, step.span])).toEqual([
      ["beginning", "0", "-100", "-100", "100"],
      ["income", "-100", "-80", "-100", "20"],
      ["ending", "0", "-80", "-80", "80"],
    ]);
    expect(negative.map((step) => Number(step.base) + Number(step.span))).toEqual([0, -80, 0]);
    expect(echartsStackedEnds(negative.map((step) => Number(step.base)), negative.map((step) => Number(step.span)))).toEqual([0, -80, 0]);

    const crossZero = stepsFromData(
      {
        beginningValue: { amount: "50", currency: "USD" },
        endingValue: { amount: "-30", currency: "USD" },
      },
      [{ key: "spending", label: "Spending", bucket: "spending", amount: { amount: "-80", currency: "USD" } }],
      { beginning: "Beginning", ending: "Ending" },
    );
    expect(crossZero.map((step) => [step.key, step.start, step.end])).toEqual([
      ["beginning", "0", "50"],
      ["spending", "50", "-30"],
      ["ending", "0", "-30"],
    ]);
  });
});

function echartsStackedEnds(bases: number[], spans: number[]): number[] {
  const chart = echarts.init(null, undefined, { renderer: "svg", ssr: true, width: 400, height: 300 });
  chart.setOption({
    animation: false,
    xAxis: { type: "category", data: bases.map((_, index) => String(index)) },
    yAxis: { type: "value" },
    series: [
      { type: "bar", stack: "waterfall", stackStrategy: "all", data: bases },
      { type: "bar", stack: "waterfall", stackStrategy: "all", data: spans },
    ],
  });
  const data = (chart as unknown as { getModel: () => { getSeriesByIndex: (index: number) => { getData: () => { getCalculationInfo: (name: string) => unknown; get: (dim: unknown, index: number) => unknown; getDimensionIndex: (name: string) => number } } } }).getModel().getSeriesByIndex(1).getData();
  const stackedDim = data.getCalculationInfo("stackedOverDimension") as string | number | undefined;
  const valueDim = (data.getCalculationInfo("stackedDimension") ?? data.getDimensionIndex("value")) as string | number;
  return bases.map((_, index) => {
    const stackedOver = stackedDim == null ? 0 : Number(data.get(stackedDim, index) ?? 0);
    const value = Number(data.get(valueDim, index) ?? 0);
    return stackedOver + value;
  });
}

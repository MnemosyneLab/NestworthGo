import css from "../../index.css?raw";
import { afterEach, expect, it, vi } from "vitest";
import { render } from "@testing-library/react";
import * as echarts from "echarts";
import { CompositionChart } from "./CompositionChart";
import { chartTheme } from "./chartTheme";
import { applyChartPolicy, type EChartsOption } from "./EChart";

const captured = vi.hoisted(() => ({ option: {} as EChartsOption }));
vi.mock("./EChart", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./EChart")>();
  return { ...actual, EChart: ({ option }: { option: EChartsOption }) => { captured.option = option; return null; } };
});
afterEach(() => {
  document.documentElement.style.removeProperty("--color-brand-from");
  document.documentElement.style.removeProperty("--color-cat-1");
});

function realChart(option: EChartsOption) {
  const chart = echarts.init(null, undefined, { renderer: "svg", ssr: true, width: 600, height: 320 });
  chart.setOption({ ...option, animation: false, tooltip: { show: false }, legend: { show: false } });
  return chart;
}

it("reproduces the real OKLCH pie emphasis failure and preserves fills through the shared policy", () => {
  const color = "oklch(58% 0.19 268)";
  // This is the installed renderer's actual parser/lift, not a mocked color.
  expect(echarts.color.lift(color, -0.1)).toBeUndefined();
  const option: EChartsOption = { series: [{ type: "pie", radius: ["30%", "70%"], emphasis: { scale: true, scaleSize: 4 },
    data: [{ name: "stock", value: 1, itemStyle: { color, opacity: 0.65 } }, { name: "cash", value: 1, itemStyle: { color: "#abc" } }] }] };
  const broken = realChart(option);
  const fixed = realChart(applyChartPolicy(option, true));
  try {
    const missing = broken.getZr().storage.getDisplayList().filter((item) => item.type === "sector")[0];
    const visible = fixed.getZr().storage.getDisplayList().filter((item) => item.type === "sector")[0] as InstanceType<typeof echarts.graphic.Sector>;
    expect(missing.style.fill).toBe(color);
    broken.dispatchAction({ type: "highlight", seriesIndex: 0, name: "stock" });
    missing.useState("emphasis");
    expect(missing.style.fill).toBeUndefined();
    const radius = visible.shape.r;
    fixed.dispatchAction({ type: "highlight", seriesIndex: 0, name: "stock" });
    visible.useState("emphasis");
    expect(visible.style.fill).toBe(color);
    expect(visible.style.opacity).toBe(0.65);
    expect(visible.shape.r).toBeGreaterThan(radius);
    visible.clearStates();
    expect(visible.style.fill).toBe(color);
  } finally { broken.dispose(); fixed.dispose(); }
});

it("keeps actual account/Portfolio donut palette fills visible for stock and mapped remainder", () => {
  document.documentElement.style.setProperty("--color-brand-from", "oklch(58% 0.19 268)");
  document.documentElement.style.setProperty("--color-cat-1", "oklch(64% 0.15 245)");
  render(<CompositionChart title="Account" currency="USD" centerValue="2" centerCaption="Subtotal" ariaLabel="Account" summary="Account" precomputedCategories
    items={[{ key: "stock", label: "Stock", amount: "1", shareBps: 5000 }, { key: "account-composition:remainder", label: "Remaining", amount: "1", shareBps: 5000 }]} />);
  const palette = chartTheme().palette;
  const chart = realChart(applyChartPolicy(captured.option, true));
  try {
    const slices = chart.getZr().storage.getDisplayList().filter((element) => element.type === "sector");
    for (const [index, name] of ["stock", "account-composition:remainder"].entries()) {
      chart.dispatchAction({ type: "highlight", seriesIndex: 0, name });
      slices[index].useState("emphasis");
      expect(slices[index].style.fill).toBe(palette[index]);
      slices[index].clearStates();
    }
  } finally { chart.dispose(); }
});

it("preserves every shipped OKLCH theme value and alpha in real bar and line-symbol emphasis", () => {
  const colors = [...new Set(css.match(/oklch\([^)]*\)/g) ?? [])];
  expect(colors.length).toBeGreaterThan(80);
  const chart = realChart(applyChartPolicy({ xAxis: { type: "category", data: colors.map((_, i) => String(i)) }, yAxis: {},
    series: [
      { type: "bar", data: colors.map((color) => ({ value: 1, itemStyle: { color } })) },
      { type: "line", showSymbol: true, showAllSymbol: true, symbolSize: 8, data: colors.map((color) => ({ value: 2, itemStyle: { color } })) },
    ],
  }, true));
  try {
    chart.dispatchAction({ type: "highlight", seriesIndex: 0 });
    chart.dispatchAction({ type: "highlight", seriesIndex: 1 });
    const shapes = chart.getZr().storage.getDisplayList().filter((element) => (typeof element.style.fill === "string" && colors.includes(element.style.fill)) || (typeof element.style.stroke === "string" && colors.includes(element.style.stroke)));
    expect(shapes.length).toBeGreaterThanOrEqual(colors.length * 2);
    for (const shape of shapes) {
      const fill = shape.style.fill;
      const stroke = shape.style.stroke;
      shape.useState("emphasis");
      expect(shape.style.fill).toBe(fill);
      expect(shape.style.stroke).toBe(stroke);
      shape.clearStates();
    }
  } finally { chart.dispose(); }
});

it("preserves explicit emphasis colors, opacity, focus, and the caller's option objects", () => {
  const option: EChartsOption = { series: { type: "bar", itemStyle: { color: "oklch(60% 0.15 155 / 0.6)" },
    emphasis: { focus: "series", itemStyle: { color: "#ff00ff", opacity: 0.25 } }, data: [1] } };
  const before = JSON.stringify(option);
  const result = applyChartPolicy(option, true);
  expect(result.series).toMatchObject({ emphasis: { focus: "series", itemStyle: { color: "#ff00ff", opacity: 0.25 } } });
  expect(JSON.stringify(option)).toBe(before);
});

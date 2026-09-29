import { render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import * as echarts from "echarts";
import type { EChartsOption } from "echarts";
import { TrendChart } from "./TrendChart";

const captured = vi.hoisted(() => ({ option: {} as EChartsOption }));
vi.mock("./EChart", () => ({ EChart: ({ option }: { option: EChartsOption }) => { captured.option = option; return null; } }));
afterEach(() => document.documentElement.style.removeProperty("--color-primary"));

it("retains OKLCH strokes and area fills when ECharts highlights a trend", () => {
  const color = "oklch(0.6 0.2 260)";
  document.documentElement.style.setProperty("--color-primary", color);
  render(<TrendChart ariaLabel="Trend" summary="Trend" currency="USD" emptyTitle="Empty"
    dates={["2026-09-01", "2026-09-02", "2026-09-03"]}
    series={[{ key: "value", name: "Value", color: "var(--color-primary)", values: ["1", "2", "3"] }]} />);
  const chart = echarts.init(null, undefined, { renderer: "svg", ssr: true, width: 600, height: 300 });
  try {
    chart.setOption({ ...captured.option, animation: false, tooltip: { show: false }, legend: { show: false } });
    chart.dispatchAction({ type: "highlight", seriesIndex: 0 });
    const elements = chart.getZr().storage.getDisplayList();
    const line = elements.find((element) => element.type === "ec-polyline");
    const area = elements.find((element) => element.type === "ec-polygon");
    expect(line).toBeDefined();
    expect(area).toBeDefined();
    // Applying the generated renderer state reproduces ECharts' hover path.
    line!.useState("emphasis");
    area!.useState("emphasis");
    expect(line!.style.stroke).toBe(color);
    expect(area!.style.fill).toBe(color);
  } finally {
    chart.dispose();
  }
});

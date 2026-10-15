import { render, screen, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import * as echarts from "echarts/core";
import { TrendChart } from "./TrendChart";

const instances = vi.hoisted(() => [] as unknown[]);
vi.mock("echarts/core", async (importOriginal) => {
  const actual = await importOriginal<typeof import("echarts/core")>();
  const { SVGRenderer } = await import("echarts/renderers"); actual.use([SVGRenderer]);
  return { ...actual, init: (container: HTMLDivElement) => {
    const chart = actual.init(container, undefined, { renderer: "svg", width: 600, height: 240 });
    instances.push(chart); return chart;
  } };
});
const latest = () => instances[instances.length - 1] as echarts.ECharts;
const dates = ["2026-10-12", "2026-10-13", "2026-10-14"];
const props = { ariaLabel: "Price", summary: "Price", currency: "USD", emptyTitle: "Empty", dates,
  series: [{ key: "value", name: "Price", color: "red", values: ["1", "2", "3"], pointMeta: [{ sourceLabel: "Agent", observationLabel: "Daily close" }, {}, {}] }] };
type Option = { legend: Array<{ show?: boolean }>; yAxis: Array<{ splitLine: { lineStyle: { type: string; opacity: number } } }>; series: Array<{ showSymbol: boolean; areaStyle: { opacity: number } }>; tooltip: Array<{ formatter: (params: unknown) => string }> };

it("minimal variant hides the legend, softens gridlines and keeps the tooltip to date and value", async () => {
  render(<TrendChart {...props} variant="minimal" />);
  await waitFor(() => expect((latest().getOption() as unknown as Option).series?.length).toBe(1));
  const option = latest().getOption() as unknown as Option;
  expect(option.legend[0].show).toBe(false);
  expect(option.yAxis[0].splitLine.lineStyle).toMatchObject({ type: "dashed", opacity: 0.5 });
  expect(option.series[0].showSymbol).toBe(false);
  expect(option.series[0].areaStyle.opacity).toBe(0.14);
  const tooltip = option.tooltip[0].formatter([{ axisValue: Date.UTC(2026, 9, 12), seriesName: "Price", seriesIndex: 0, dataIndex: 0, data: { originalIndex: 0 } }]);
  expect(tooltip).toContain("Oct 12, 2026");
  expect(tooltip).toContain("$1.00");
  expect(tooltip).not.toMatch(/Price:|Agent|Daily close|Source/);
  expect(screen.getByText("View data table")).toHaveClass("text-xs");
});

it("default variant keeps the legend, evidence tooltip and standard data-table link", async () => {
  render(<TrendChart {...props} />);
  await waitFor(() => expect((latest().getOption() as unknown as Option).series?.length).toBe(1));
  const option = latest().getOption() as unknown as Option;
  expect(option.legend[0].show).not.toBe(false);
  const tooltip = option.tooltip[0].formatter([{ axisValue: Date.UTC(2026, 9, 12), seriesName: "Price", seriesIndex: 0, dataIndex: 0, data: { originalIndex: 0 } }]);
  expect(tooltip).toContain("Agent");
  expect(screen.getByText("View data table")).toHaveClass("text-sm");
});

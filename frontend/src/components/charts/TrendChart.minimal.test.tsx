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
  expect(tooltip).toContain("2026-10-12");
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

const segmentedDates = ["2026-10-10", "2026-10-11", "2026-10-12", "2026-10-13", "2026-10-14"];
const segmentedProps = { ...props, dates: segmentedDates,
  series: [{ key: "value", name: "Price", color: "red", values: ["1", "2", "3", "4", "5"],
    // Breaks before index 2 and 3 produce the segments [0,1], [2] and [3,4].
    pointMeta: [{ sourceLabel: "Agent" }, {}, { breakBefore: true, sourceLabel: "Agent" }, { breakBefore: true }, {}] }] };
type SegmentedOption = { series: Array<{ showSymbol: boolean; data: Array<{ originalIndex: number }> }>; tooltip: Array<{ formatter: (params: unknown) => string }> };

it.each([["minimal", "minimal" as const], ["default", undefined]])("%s tooltip maps a flattened segment index back to the original point", async (_name, variant) => {
  render(<TrendChart {...segmentedProps} variant={variant} />);
  await waitFor(() => expect((latest().getOption() as unknown as SegmentedOption).series?.length).toBe(3));
  const option = latest().getOption() as unknown as SegmentedOption;
  expect(option.series.map((item) => item.data.map((entry) => entry.originalIndex))).toEqual([[0, 1], [2], [3, 4]]);
  const at = (seriesIndex: number, dataIndex: number, index: number) => option.tooltip[0].formatter([{ axisValue: Date.UTC(2026, 9, 10 + index), seriesName: "Price", seriesIndex, dataIndex, data: { originalIndex: index } }]);
  // Without the mapping, seriesIndex 2 pointed past the one-element series array and showed "unavailable".
  expect(at(0, 1, 1)).toContain("$2.00");
  expect(at(1, 0, 2)).toContain("$3.00");
  expect(at(2, 0, 3)).toContain("$4.00");
  expect(at(2, 1, 4)).toContain("$5.00");
  if (variant === undefined) expect(at(1, 0, 2)).toContain("Agent");
});

it("keeps an isolated observation visible per segment without turning on markers for the connected ones", async () => {
  render(<TrendChart {...segmentedProps} variant="minimal" />);
  await waitFor(() => expect((latest().getOption() as unknown as SegmentedOption).series?.length).toBe(3));
  expect((latest().getOption() as unknown as SegmentedOption).series.map((item) => item.showSymbol)).toEqual([false, true, false]);
});

it("keeps an isolated observation visible when the default chart is too dense for markers", async () => {
  const many = Array.from({ length: 30 }, (_, index) => new Date(Date.UTC(2026, 8, 1 + index)).toISOString().slice(0, 10));
  render(<TrendChart {...props} dates={many} series={[{ key: "value", name: "Price", color: "red", values: many.map((_, index) => String(index + 1)),
    pointMeta: many.map((_, index) => ({ breakBefore: index === 10 || index === 11 })) }]} />);
  await waitFor(() => expect((latest().getOption() as unknown as SegmentedOption).series?.length).toBe(3));
  expect((latest().getOption() as unknown as SegmentedOption).series.map((item) => item.showSymbol)).toEqual([false, true, false]);
});

it("plots interleaved evidence groups without fake gaps and keeps quote dots unconnected even on hover", async () => {
  render(<TrendChart {...props} series={[
    { key: "close", name: "Daily close", color: "red", values: ["1", "9", "3"], pointIndices: [0, 2] },
    { key: "intraday", name: "Intraday", color: "blue", values: ["1", "9", "3"], pointIndices: [1], presentation: "points" },
  ]} />);
  type EvidenceOption = { series: Array<{ data: Array<{ originalIndex: number }>; showSymbol: boolean; lineStyle: { width: number }; emphasis: { lineStyle: { width: number } }; areaStyle?: { opacity?: number } }> };
  await waitFor(() => expect((latest().getOption() as unknown as EvidenceOption).series).toHaveLength(2));
  const option = latest().getOption() as unknown as EvidenceOption;
  expect(option.series.map(series => series.data.map(point => point.originalIndex))).toEqual([[0, 2], [1]]);
  expect(option.series[0].lineStyle.width).toBe(2.5);
  expect(option.series[1]).toMatchObject({ showSymbol: true, lineStyle: { width: 0 }, emphasis: { lineStyle: { width: 0 } } });
  expect(option.series[1].areaStyle?.opacity ?? 0).toBe(0);
});

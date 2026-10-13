import { render, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import * as echarts from "echarts/core";
import { EChart, type EChartsOption } from "./EChart";
import { TrendChart } from "./TrendChart";
const instances = vi.hoisted(() => [] as unknown[]);
vi.mock("echarts/core", async (importOriginal) => {
  const actual = await importOriginal<typeof import("echarts/core")>();
  const { SVGRenderer } = await import("echarts/renderers"); actual.use([SVGRenderer]);
  return { ...actual, init: (container: HTMLDivElement) => {
    const chart = actual.init(container, undefined, { renderer: "svg", width: 600, height: 320 });
    instances.push(chart); return chart;
  } };
});
function latest() { return instances[instances.length - 1] as echarts.ECharts; }
function series(chart: echarts.ECharts) {
  return (chart.getOption().series as Array<{ id: string; data: Array<{ value: [number, number | null] }>; lineStyle: { type: string } }>).filter(Boolean);
}
it("replaces real provisional segments on NAV revision, removing old values/dates and retaining true gaps", async () => {
  const dates = ["2026-10-09", "2026-10-10", "2026-10-11", "2026-10-12", "2026-10-13", "2026-10-14"];
  const props = { ariaLabel: "NAV", summary: "NAV", currency: "CNY", emptyTitle: "Empty" };
  const view = render(<TrendChart {...props} dates={dates} series={[{ key: "net", name: "Net", color: "blue", values: dates.map(() => "200"),
    pointMeta: [{}, { provisional: true }, {}, {}, { provisional: true }, {}] }]} />);
  const chart = latest();
  await waitFor(() => expect(series(chart)).toHaveLength(4));
  expect(series(chart).some(item => item.lineStyle.type === "dashed")).toBe(true);
  view.rerender(<TrendChart {...props} dates={["2026-10-12", "2026-10-13", "2026-10-14"]}
    series={[{ key: "net", name: "Net", color: "blue", values: ["210", null, "220"], pointMeta: [{}, {}, {}] }]} />);
  await waitFor(() => expect(series(chart)).toHaveLength(1)); expect(latest()).toBe(chart);
  expect(series(chart)[0].lineStyle.type).toBe("solid");
  expect(series(chart)[0].data.map(point => point.value)).toEqual([
    [Date.UTC(2026, 9, 12), 210], [Date.UTC(2026, 9, 13), null], [Date.UTC(2026, 9, 14), 220],
  ]);
  const model = chart as unknown as { getModel(): { getSeriesByIndex(index: number): { getData(): { get(dimension: string, index: number): number } }; getSeries(): unknown[] } };
  expect(model.getModel().getSeries()).toHaveLength(1);
  expect(Number.isNaN(model.getModel().getSeriesByIndex(0).getData().get("y", 1))).toBe(true);
});
it("replaces dynamic series while preserving user zoom and legend state on the same chart", async () => {
  const option: EChartsOption = { animation: false, legend: {}, xAxis: { type: "category", data: ["A", "B", "C"] }, yAxis: {},
    dataZoom: [{ id: "range", type: "inside", start: 0, end: 100 }],
    series: [{ id: "first", name: "First", type: "line", data: [1, 2, 3] }, { id: "obsolete", name: "Obsolete", type: "line", data: [4, 5, 6] }] };
  const view = render(<EChart option={option} replaceSeries ariaLabel="Dynamic" />); const chart = latest();
  chart.dispatchAction({ type: "dataZoom", dataZoomId: "range", start: 25, end: 75 });
  chart.dispatchAction({ type: "legendUnSelect", name: "First" });
  const next = { ...option }; delete next.dataZoom;
  view.rerender(<EChart replaceSeries ariaLabel="Dynamic" option={{ ...next, series: [{ id: "first", name: "First", type: "line", data: [10, 20, 30] }] }} />);
  await waitFor(() => expect(series(chart)).toHaveLength(1)); expect(latest()).toBe(chart);
  const actual = chart.getOption() as { dataZoom: Array<{ start: number; end: number }>; legend: Array<{ selected: Record<string, boolean> }> };
  expect(actual.dataZoom[0]).toMatchObject({ start: 25, end: 75 }); expect(actual.legend[0].selected.First).toBe(false);
});

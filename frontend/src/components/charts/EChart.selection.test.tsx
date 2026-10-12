import { expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as echarts from "echarts";
import { AccountCompositionPanel } from "@/features/accounts/AccountCompositionPanel";
import { synchronizePieSelection, type EChartsOption } from "./EChart";
import fixture from "../../../../testdata/current/account-composition-shares.json";

const instances = vi.hoisted(() => [] as unknown[]);
vi.mock("echarts/core", async (importOriginal) => {
  const actual = await importOriginal<typeof import("echarts/core")>();
  const { SVGRenderer } = await import("echarts/renderers");
  actual.use([SVGRenderer]);
  return { ...actual, init: (container: HTMLDivElement) => {
    const chart = actual.init(container, undefined, { renderer: "svg", width: 600, height: 320 });
    instances.push(chart);
    return chart;
  } };
});

// Inspect ECharts' real internal selectedMap as well as the rendered sector
// offsets; DOM aria flags or data.selected alone cannot detect this defect.
type Sector = { x: number; y: number };
type PieModel = { isSelected(index: number): boolean; getData(): { count(): number; getName(index: number): string; getItemGraphicEl(index: number): Sector } };
function model(chart: unknown): PieModel {
  return (chart as { getModel(): { getSeriesByIndex(index: number): PieModel } }).getModel().getSeriesByIndex(0);
}
function state(chart: unknown, name: string) {
  const series = model(chart);
  const data = series.getData();
  const index = Array.from({ length: data.count() }, (_, i) => i).find((i) => data.getName(i) === name)!;
  return { selected: Boolean(series.isSelected(index)), sector: data.getItemGraphicEl(index) };
}
function latest() { return instances[instances.length - 1]; }

it("reproduces merge-retained selection and clears the real selectedMap with explicit actions", () => {
  const chart = echarts.init(null, undefined, { renderer: "svg", ssr: true, width: 500, height: 300 });
  const option: EChartsOption = { animation: false, series: [{ type: "pie", selectedMode: "single", data: [
    { name: "ETF", value: 1, selected: false }, { name: "Cash", value: 1, selected: false },
  ] }] };
  try {
    chart.setOption(option);
    chart.dispatchAction({ type: "select", seriesIndex: 0, name: "ETF" });
    chart.setOption(option, { notMerge: false });
    expect(state(chart, "ETF").selected).toBe(true);
    synchronizePieSelection(chart, option, null);
    expect(state(chart, "ETF").selected).toBe(false);
    synchronizePieSelection(chart, option, "Cash");
    expect(state(chart, "Cash").selected).toBe(true);
    expect(state(chart, "ETF").selected).toBe(false);
  } finally { chart.dispose(); }
});

it("keeps actual wedge offsets synchronized with deselect, tail collapse, refresh and account switch", async () => {
  const user = userEvent.setup();
  const { rerender } = render(<AccountCompositionPanel key="first" composition={fixture} updatedAt={1} />);
  const etf = "account-composition:type:etf";
  const etfButton = screen.getByTestId(`composition-slice-${etf}`);
  await user.click(etfButton);
  await waitFor(() => expect(state(latest(), etf).selected).toBe(true));
  await waitFor(() => expect(Math.hypot(state(latest(), etf).sector.x, state(latest(), etf).sector.y)).toBeGreaterThan(0));
  await user.click(etfButton);
  await user.click(screen.getByText("Account composition"));
  expect(etfButton).toHaveAttribute("aria-pressed", "false");
  await waitFor(() => expect(state(latest(), etf).selected).toBe(false));
  await waitFor(() => expect(Math.hypot(state(latest(), etf).sector.x, state(latest(), etf).sector.y)).toBeCloseTo(0));

  await user.click(screen.getByRole("radio", { name: "By detail" }));
  await user.click(screen.getByText("View data table"));
  await user.click(screen.getByRole("button", { name: "Show all 12 items" }));
  const remainder = "account-composition:remainder";
  await user.click(screen.getByTestId("composition-slice-account-composition:holding:item-10"));
  await waitFor(() => expect(state(latest(), remainder).selected).toBe(true));
  await waitFor(() => expect(Math.hypot(state(latest(), remainder).sector.x, state(latest(), remainder).sector.y)).toBeGreaterThan(0));
  await user.click(screen.getByRole("button", { name: "Show top 10" }));
  expect(screen.getByRole("table")).toBeInTheDocument();
  await waitFor(() => expect(state(latest(), remainder).selected).toBe(false));
  await waitFor(() => expect(Math.hypot(state(latest(), remainder).sector.x, state(latest(), remainder).sector.y)).toBeCloseTo(0));

  await user.click(screen.getByTestId(`composition-slice-${remainder}`));
  await waitFor(() => expect(state(latest(), remainder).selected).toBe(true));
  rerender(<AccountCompositionPanel key="first" composition={fixture} updatedAt={2} />);
  expect(screen.getByRole("radio", { name: "By detail" })).toBeChecked();
  await waitFor(() => expect(state(latest(), remainder).selected).toBe(false));
  rerender(<AccountCompositionPanel key="second" composition={fixture} updatedAt={2} />);
  expect(screen.getByRole("radio", { name: "By type" })).toBeChecked();
  await waitFor(() => expect(state(latest(), etf).selected).toBe(false));
});

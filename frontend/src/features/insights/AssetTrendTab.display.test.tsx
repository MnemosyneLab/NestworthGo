import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { resetDisplayPreferences, setDisplayPreferences } from "@/lib/displayPreferences";
import * as echarts from "echarts/core";
import { AssetTrendTab } from "./AssetTrendTab";
import { useAnalysisStore } from "@/stores/analysis";
const harness = vi.hoisted(() => ({ data: {} as Record<string, unknown>, charts: [] as unknown[] }));
vi.mock("@/queries/returnAnalysis", () => ({ useAssetTrend: () => ({ data: harness.data }) }));
vi.mock("@/queries/settings", () => ({ useSettings: () => ({ data: { currency: "CNY" } }) }));
vi.mock("./analysisProjectionContext", () => ({ useAnalysisProjectionContext: () => ({ request: {}, enabled: true, origin: { data: { timezone: "Asia/Shanghai" } }, scopeReady: true, rangeAvailable: true }) }));
vi.mock("echarts/core", async (importOriginal) => {
  const actual = await importOriginal<typeof import("echarts/core")>(); const { SVGRenderer } = await import("echarts/renderers"); actual.use([SVGRenderer]);
  return { ...actual, init: (container: HTMLDivElement) => { const chart = actual.init(container, undefined, { renderer: "svg", width: 600, height: 320 }); harness.charts.push(chart); return chart; } };
});
const quality = { availability: "partial", coverage: "provisional", includedCount: 2, valuedCount: 1, unpricedCount: 1, provisionalCount: 1, carriedCount: 1, staleCount: 1, oldestMarketDate: "2026-10-09", newestMarketDate: "2026-10-09", reasons: ["carried_nav"] };
const partial = { currency: "CNY", total: null, knownSubtotal: { amount: "200", currency: "CNY" }, quality };
function point(period: string, displayValuation: unknown = partial) { return { period, value: null, rate: null, available: true, status: "partial", ratedDays: 0, totalDays: 1, valuationForced: null, displayValuation }; }
function chartData() {const chart = harness.charts[harness.charts.length - 1] as echarts.ECharts;return (chart.getOption().series as Array<{ data: Array<{ value: [number, number | null] }> }>).flatMap(series => series.data.map(point => point.value[1]));}
beforeEach(() => { useAnalysisStore.getState().reset(); harness.charts = []; harness.data = { available: true, status: "partial", ratedDays: 0, totalDays: 2, valuationForced: null, points: [] }; });
afterEach(() => resetDisplayPreferences());
it("keeps all-partial known contributions and quality in the actual keyboard-accessible source table without plotting a total", async () => {
  harness.data = { ...harness.data, available: false, points: [point("2026-10-12"), point("2026-10-13")] };
  render(<AssetTrendTab session={useAnalysisStore.getState()} />);
  const summary = screen.getByText("View data table"); summary.focus(); await userEvent.keyboard("{Enter}");
  const table = screen.getByRole("table"); expect(table).toHaveTextContent("200.00"); expect(table).toHaveTextContent("Known net contribution"); expect(table).toHaveTextContent("2026-10-09"); expect(table).toHaveTextContent("1 item has no usable valuation");
  expect(chartData().every(value => value === null)).toBe(true);
});
it("renders mixed partial, full provisional and unknown rows truthfully while only plotting full totals", async () => {
  const full = { ...partial, total: { amount: "210", currency: "CNY" }, knownSubtotal: { amount: "210", currency: "CNY" }, quality: { ...quality, availability: "full", unpricedCount: 0, includedCount: 1 } };
  const unknown = { ...partial, knownSubtotal: null, quality: { ...quality, availability: "none", valuedCount: 0, unpricedCount: 2 } };
  harness.data.points = [point("2026-10-11"), point("2026-10-12", full), point("2026-10-13", unknown), point("2026-10-14", {...full,total:{amount:"220",currency:"CNY"},knownSubtotal:{amount:"220",currency:"CNY"}})];
  render(<AssetTrendTab session={useAnalysisStore.getState()} />); await userEvent.click(screen.getByText("View data table"));
  const rows = within(screen.getByRole("table")).getAllByRole("row");
  expect(rows[1]).toHaveTextContent("Known net contribution"); expect(rows[2]).toHaveTextContent("Estimated valuation"); expect(rows[3]).toHaveTextContent("No current value"); expect(rows[3]).toHaveTextContent("2 items have no usable valuation");
  expect(chartData()).toEqual([210, 220]);
 expect(((harness.charts[harness.charts.length-1] as echarts.ECharts).getOption().series as unknown[])).toHaveLength(2);
});

it.each(["USD", ""])("uses declared/subtotal native currency for an all-partial series in a CNY household (%s)", async declared => {
  harness.data = { ...harness.data, available: false, points: [point("2026-10-12", { ...partial, currency: declared, knownSubtotal: { amount: "200", currency: "USD" } })] };
  render(<AssetTrendTab session={useAnalysisStore.getState()} />);
  expect(screen.getByRole("heading", { name: /USD/ })).not.toHaveTextContent("CNY");
  await userEvent.click(screen.getByText("View data table"));
  expect(screen.getByRole("table")).toHaveTextContent("US$200.00");
});

it("formats asset trend table dates with the display preference", async () => {
  setDisplayPreferences({ dateFormat: "day-first" });
  harness.data = { ...harness.data, points: [point("2026-10-12"), point("2026-09")] };
  render(<AssetTrendTab session={useAnalysisStore.getState()} />);
  await userEvent.click(screen.getByText("View data table"));
  const table = screen.getByRole("table");
  expect(table).toHaveTextContent("12/10/2026");
  expect(table).not.toHaveTextContent("2026-10-12");
  expect(table).toHaveTextContent("2026-09");
});

it("keeps liability summary, chart and table signed consistently without changing asset summaries", async () => {
 const money = (amount: string) => ({amount, currency: "USD"});
 harness.data = { ...harness.data, points: [{period:"2026-10-12",value:money("30")},{period:"2026-10-13",value:money("35")}], valueChange: {beginningValue:money("30"),endingValue:money("35"),change:money("5"),changeRate:"0.1667"} };
 const view = render(<AssetTrendTab session={useAnalysisStore.getState()} />);
 await userEvent.selectOptions(screen.getByLabelText("Metric"), "liabilities");
 const summary = screen.getByText("Beginning value").closest("dl")!;
 expect(summary).toHaveTextContent("-US$30.00"); expect(summary).toHaveTextContent("-US$35.00"); expect(summary).toHaveTextContent("-US$5.00"); expect(summary).toHaveTextContent("+16.67%");
 expect(chartData()).toEqual([-30,-35]);
 await userEvent.click(screen.getByText("View data table"));
 expect(screen.getByRole("table")).toHaveTextContent("-US$30.00"); expect(screen.getByRole("table")).toHaveTextContent("-US$35.00");
 await userEvent.selectOptions(screen.getByLabelText("Metric"), "assets");
 expect(summary).toHaveTextContent("+US$5.00"); expect(chartData()).toEqual([30,35]);
 expect((harness.data.valueChange as {change:{amount:string}}).change.amount).toBe("5");
 view.unmount();
});

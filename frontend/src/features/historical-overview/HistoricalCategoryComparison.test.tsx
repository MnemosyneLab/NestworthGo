import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import i18n from "@/i18n";
import { HistoricalCategoryComparisonView } from "./HistoricalCategoryComparison";
import type { HistoricalCategoryCell, HistoricalCategoryComparison, HistoricalOverviewResult } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";

const chart = vi.hoisted(() => ({ options: [] as unknown[] }));
vi.mock("@/components/charts/EChart", () => ({ EChart: ({ option, ariaLabel, onSelectName }: { option: { series: Array<{ data: Array<{ name: string } | null> }> }; ariaLabel: string; onSelectName: (key: string) => void }) => {
  chart.options.push(option);
  return <div role="img" aria-label={ariaLabel}>{option.series[1].data.map(item => item && <button key={item.name} onClick={() => onSelectName(item.name)}>chart-{item.name}</button>)}</div>;
} }));
const cell = (amount: string | null, share: string | null, patch: Partial<HistoricalCategoryCell> = {}): HistoricalCategoryCell => ({ amount, share, knownAmount: amount, status: "complete", shareBasis: "total", includedCount: 1, missingRowKeys: [], ...patch });
function category(patch: Partial<HistoricalCategoryComparison> = {}): HistoricalCategoryComparison {
  return { role: "asset", dimension: "byClass", key: "cash", left: cell("100", "50"), right: cell("100", "25"), amountChange: "0", shareChangePp: "-25", amountReasons: [], shareReasons: [], ...patch };
}
function result(categories: HistoricalCategoryComparison[]): HistoricalOverviewResult {
  const state = { date: "2026-08-01", current: false, cutoffAt: "2026-08-01T23:59:59Z", currency: "CNY", complete: true, assets: "200", liabilities: "0", netWorth: "200", knownAssets: "200", knownLiabilities: "0", byClass: [], byCurrency: [] };
  return { categories, rows: [], left: state, right: { ...state, date: "2026-08-09" }, change: null, timezone: "UTC", originDate: "2026-08-01", lastClosedDate: "2026-08-09", capturedAt: "2026-08-10T12:00:00Z", inputGeneration: 1, resolverPolicy: "policy" };
}
function show(data: HistoricalOverviewResult, role: "asset" | "liability" = "asset") {
  const onFilter = vi.fn();
  render(<HistoricalCategoryComparisonView data={data} role={role} dimension="byClass" onRole={vi.fn()} onDimension={vi.fn()} onFilter={onFilter} />);
  return onFilter;
}

it.each(["en", "zh-CN", "zh-TW"])("renders backend monetary and percentage-point deltas and keyboard navigation (%s)", async locale => {
  await i18n.changeLanguage(locale);
  const onFilter = show(result([category({ amountChange: "0.00012", shareChangePp: "-25" })]));
  expect(screen.getAllByText("+0.00012 CNY")[0]).toBeInTheDocument();
  expect(screen.getAllByText(i18n.t("historicalOverview.percentagePoints", { value: "-25" }))[0]).toBeInTheDocument();
  const button = screen.getByRole("button", { name: i18n.t("enum.cash") });
  button.focus(); await userEvent.keyboard("{Enter}");
  expect(onFilter).toHaveBeenCalledWith({ role: "asset", dimension: "byClass", key: "cash" }, button);
  await userEvent.click(screen.getByText(i18n.t("historicalOverview.fullChartData")));
  const table = screen.getByRole("table");
  expect(table).toHaveTextContent("+0.00012 CNY");
  expect(table).toHaveTextContent(i18n.t("historicalOverview.percentagePoints", { value: "-25" }));
  expect(table.querySelectorAll("tbody td")).toHaveLength(7);
});

it("does not fill absent categories with zero and retains partial and excluded reasons", async () => {
  await i18n.changeLanguage("en"); chart.options.length = 0;
  show(result([
    category({ key: "cash", left: cell(null, null, { status: "absent", knownAmount: null, includedCount: 0 }), amountChange: null, shareChangePp: null, amountReasons: ["absent", "existence_changed"], shareReasons: ["absent"] }),
    category({ key: "stock", left: cell(null, "20", { status: "partial", shareBasis: "known_subtotal", knownAmount: "1.00012", missingRowKeys: ["missing"] }), amountChange: null, shareChangePp: null, amountReasons: ["partial"], shareReasons: ["share_denominator_unavailable"] }),
    category({ key: "property", left: cell(null, null, { status: "excluded", includedCount: 0 }), amountChange: null, shareChangePp: null, amountReasons: ["scope_changed"], shareReasons: [] }),
  ]));
  const option = chart.options[chart.options.length - 1] as { series: Array<{ data: unknown[] }> };
  expect(option.series[0].data).toEqual([null, null, null]);
  expect(screen.getAllByText(/1.00012 CNY/)[0]).toBeInTheDocument();
  expect(screen.getAllByText(/A\/B|retained member set/)[0]).toBeInTheDocument();
  expect(screen.getAllByText(/of known subtotal only/)[0]).toBeInTheDocument();
  expect(screen.getAllByText(/Inclusion scope changed/).length).toBeGreaterThan(0);
});

it("keeps negative debt changes signed and chart selection keyed with escaped source tooltips", async () => {
  await i18n.changeLanguage("en"); chart.options.length = 0;
  const onFilter = show(result([category({ role: "liability", key: "<debt>", left: cell("-0.00012", null, { shareBasis: "unavailable" }), right: cell("0", null, { shareBasis: "unavailable" }), amountChange: "0.00012", shareChangePp: null, shareReasons: ["share_denominator_unavailable"] })]), "liability");
  const option = chart.options[chart.options.length - 1] as { tooltip: { formatter: (point: unknown) => string }; series: Array<{ data: Array<{value: number}> }> };
  expect(option.series[0].data[0].value).toBe(-0.00012);
  expect(option.tooltip.formatter({ name: "<debt>", seriesName: "A" })).toContain("&lt;Debt&gt;");
  expect(screen.getByText(/Debt categories show book balance changes/)).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "chart-<debt>" }));
  expect(onFilter.mock.calls[0][0]).toEqual({ role: "liability", dimension: "byClass", key: "<debt>" });
});

it("uses a table for zero or unrenderable values and suppresses incompatible-currency deltas", async () => {
  await i18n.changeLanguage("en");
  const zero = render(<HistoricalCategoryComparisonView data={result([category({ left: cell("0", null), right: cell("0", null), amountChange: "0", shareChangePp: null })])} role="asset" dimension="byClass" onRole={vi.fn()} onDimension={vi.fn()} onFilter={vi.fn()} />);
  expect(screen.queryByRole("img")).not.toBeInTheDocument(); expect(screen.getByRole("table")).toBeVisible(); zero.unmount();
  const data = result([category({ amountChange: "999", shareChangePp: "999" })]); data.right!.currency = "USD";
  show(data);
  expect(screen.queryByRole("img")).not.toBeInTheDocument(); expect(screen.queryByText(/999/)).not.toBeInTheDocument();
  expect(screen.getAllByText("100 USD")[0]).toBeInTheDocument();
  expect(within(screen.getByRole("region", { name: "Category comparison" })).getAllByText(i18n.t("historicalOverview.currencyMismatch"))[0]).toBeInTheDocument();
});

it("does not describe a zero with an absent opposite side as known zero on both sides", async () => {
  await i18n.changeLanguage("en");
  show(result([category({ left: cell(null, null, { status: "absent", shareBasis: "unavailable" }), right: cell("0", null, { shareBasis: "unavailable" }), amountChange: null, shareChangePp: null, amountReasons: ["absent"], shareReasons: ["absent"] })]));
  expect(screen.queryByRole("img")).not.toBeInTheDocument();
  expect(screen.getByText("Available category amounts are zero; unavailable sides remain unknown.")).toBeInTheDocument();
  expect(screen.queryByText(/zero on both sides/)).not.toBeInTheDocument();
  expect(screen.getByRole("table")).toBeVisible();
});

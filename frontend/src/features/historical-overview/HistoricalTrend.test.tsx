import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import i18n from "@/i18n";
import { HistoricalTrend, historicalTrendSegments } from "./HistoricalTrend";
import type { HistoricalOverviewSeriesResult, HistoricalSeriesPoint } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";

const chart = vi.hoisted(() => ({ options: [] as unknown[] }));
vi.mock("@/components/charts/EChart", () => ({ EChart: ({ option, ariaLabel, onSelectName }: { option: unknown; ariaLabel: string; onSelectName: (date: string) => void }) => { chart.options.push(option); return <div role="img" aria-label={ariaLabel}><button onClick={() => onSelectName("2026-08-01")}>Chart date</button></div>; } }));
function point(date: string, netWorth: string | null = "-0.00012", patch: Partial<HistoricalSeriesPoint> = {}): HistoricalSeriesPoint {
 return { date, cutoffAt: `${date}T23:59:59Z`, available: true, gapReason: "", assets: "1", liabilities: "1.00012", netWorth, knownAssets: "1", knownLiabilities: "1.00012", assetsComplete: true, liabilitiesComplete: true, assetCoverage: { included: 1, known: 1, missing: 0 }, liabilityCoverage: { included: 1, known: 1, missing: 0 }, missing: [], scopeChanged: false, basisChanged: false, connectAssets: true, connectLiabilities: true, connectNetWorth: true, ...patch };
}
function capture(points: HistoricalSeriesPoint[]): HistoricalOverviewSeriesResult {
 return { points, from: points[0].date, to: points[points.length - 1].date, work: { leaves: 2, queries: 1, candidateFacts: 2, candidateBytes: 100 }, overview: { capturedAt: "2026-08-10T12:00:00Z", timezone: "UTC", left: { currency: "CNY" } } as HistoricalOverviewSeriesResult["overview"] };
}
it("breaks gaps and scope/basis boundaries without dropping observed zeros or substituting subtotals", () => {
 const points = [point("2026-08-01", "0"), point("2026-08-02", null), point("2026-08-03", "1"), point("2026-08-04", "2", { scopeChanged: true, connectNetWorth: false }), point("2026-08-05", "3", { basisChanged: true, connectNetWorth: false })];
 const segments = historicalTrendSegments(points, "netWorth");
 expect(segments).toHaveLength(4); expect(segments[0][0]).toEqual({ name: "2026-08-01", value: 0 });
 expect(segments.every(segment => segment[1] === null)).toBe(true);
 expect(historicalTrendSegments([point("2026-08-01", null)], "netWorth")).toEqual([]);
 expect(historicalTrendSegments([point("2026-08-01", null, { liabilities: null, liabilitiesComplete: false })], "assets")[0][0]?.value).toBe(1);
});
it.each(["en", "zh-CN", "zh-TW"])("keeps exact source values, local selection and explicit recapture actions accessible (%s)", async locale => {
 await i18n.changeLanguage(locale); const select = vi.fn(); const endpoint = vi.fn(); const open = vi.fn(); const metric = vi.fn();
 render(<HistoricalTrend capture={capture([point("2026-08-01")])} metric="netWorth" selectedDate={null} onMetric={metric} onSelect={select} onEndpoint={endpoint} onOpenDay={open} />);
 expect(screen.getByText(i18n.t("historicalOverview.trend.onePoint"))).toBeInTheDocument();
 const table = screen.getByRole("table"); expect(table).toHaveTextContent("-0.00012 CNY");
 await userEvent.click(within(table).getByRole("button", { name: "2026-08-01" })); expect(select).toHaveBeenCalledWith("2026-08-01");
 await userEvent.click(screen.getByRole("button", { name: i18n.t("historicalOverview.trend.setA") })); expect(endpoint).toHaveBeenCalledWith("A", "2026-08-01");
 await userEvent.click(screen.getByRole("button", { name: i18n.t("historicalOverview.trend.openDay") })); expect(open).toHaveBeenCalledWith("2026-08-01");
 await userEvent.click(screen.getByRole("button", { name: i18n.t("overview.assets") })); expect(metric).toHaveBeenCalledWith("assets");
 const option = chart.options[chart.options.length - 1] as { tooltip: { formatter: (p: unknown) => string }; series: Array<{connectNulls: boolean}> };
 expect(option.tooltip.formatter({name:"2026-08-01"})).toContain("-0.00012 CNY"); expect(option.series[0].connectNulls).toBe(false);
});
it("provides an exact table fallback for numerical overflow and explicit skipped-day gaps", async () => {
 await i18n.changeLanguage("en");
 render(<HistoricalTrend capture={capture([point("2026-08-01", "9".repeat(400)), point("2026-08-02", null, { available: false, gapReason: "nonexistent_civil_date", assets: null, liabilities: null })])} metric="netWorth" selectedDate={null} onMetric={vi.fn()} onSelect={vi.fn()} onEndpoint={vi.fn()} onOpenDay={vi.fn()} />);
 expect(screen.queryByRole("img")).not.toBeInTheDocument(); expect(screen.getByRole("table")).toHaveTextContent("9".repeat(400));
 expect(screen.getByText(i18n.t("historicalOverview.trend.skippedDate"))).toBeInTheDocument();
});

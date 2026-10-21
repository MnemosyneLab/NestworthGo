import { formatAmount } from "@/lib/money";
import { useRef, useState, type ReactNode } from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import type { HistoricalOverviewCell, HistoricalOverviewResult, HistoricalOverviewRow, HistoricalOverviewState } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import { HistoricalAccountDeltaView, HistoricalAllocationView, HistoricalMetricCards } from "./HistoricalPanorama";
import { CompositionChart } from "@/components/charts/CompositionChart";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";

const chart = vi.hoisted(() => ({ options: [] as unknown[] }));
vi.mock("@/components/charts/EChart", () => ({
  EChart: ({ option, ariaLabel, onSelectName, center }: {
    option: { series?: Array<{ data?: Array<{ name?: string } | null> }> };
    ariaLabel: string; onSelectName?: (name: string) => void; center?: ReactNode;
  }) => {
    chart.options.push(option);
    return <div role="img" aria-label={ariaLabel}>{center}{option.series?.[0]?.data?.map(item => item?.name &&
      <button key={item.name} onClick={() => onSelectName?.(item.name!)}>chart-on-{item.name}</button>)}</div>;
  },
}));

function state(date: string, amount: string): HistoricalOverviewState {
  return { date, cutoffAt: `${date}T23:59:59Z`, current: false, currency: "CNY", complete: true,
    assets: amount, liabilities: "0", netWorth: amount, knownAssets: amount, knownLiabilities: "0",
    byClass: [{ key: "cash", amount, share: amount === "0" ? "0" : "100" }],
    byCurrency: [{ key: "CNY", amount, share: amount === "0" ? "0" : "100" }] };
}

function account(key: string, delta: string): HistoricalOverviewRow {
  const cell = (value: string) => ({ status: "active", included: true, complete: true, currency: "CNY",
    nativeAmount: value, baseAmount: value, quantity: null, assetClass: "cash", manual: false,
    valueSourceAt: "", valueSourceId: "", price: null, fx: null, missing: [], missingDetails: [] });
  return { key, accountId: "", holdingId: "", instrumentId: "", parentKey: "", kind: "account", name: "Duplicate", role: "asset", left: cell("100"), right: cell(delta === "10" ? "110" : "120"),
    baseChange: delta, nativeChange: delta, quantityChange: null, changed: true };
}

function result(right: HistoricalOverviewState | null): HistoricalOverviewResult {
  return { categories: [], timezone: "UTC", originDate: "2026-08-01", lastClosedDate: "2026-08-09", capturedAt: "2026-08-10T12:00:00Z",
    inputGeneration: 1, resolverPolicy: "market-date-daily-summary-v3", left: state("2026-08-01", "10"), right,
    change: right ? { assets: "10", liabilities: "0", netWorth: "10" } : null,
    rows: [account("first", "10"), account("second", "20")] };
}

function displayState(exact: string, projected: string, partial = false): HistoricalOverviewState {
  const data = state("2026-08-01", exact);
  data.currency = "USD";
  const money = { amount: projected, currency: "USD" };
  const assets = { currency: "USD", total: partial ? null : money, knownSubtotal: money,
    quality: { availability: partial ? "partial" : "full", coverage: partial ? "provisional" : "verified",
      includedCount: 2, valuedCount: partial ? 1 : 2, unpricedCount: partial ? 1 : 0, provisionalCount: 0,
      carriedCount: 0, staleCount: 0, oldestMarketDate: null, newestMarketDate: null, reasons: [] } };
  const zero = { amount: "0", currency: "USD" };
  const liabilities = { currency: "USD", total: zero, knownSubtotal: zero,
    quality: { ...assets.quality, availability: "full", coverage: "verified", includedCount: 0, valuedCount: 0, unpricedCount: 0 } };
  data.displayValuation = { assets, netWorth: assets, liabilities } as HistoricalOverviewState["displayValuation"];
  if (partial) { data.complete = false; data.assets = null; data.netWorth = null; }
  return data;
}

it("aligns complete metric text, hover text and both composition centers at a rounding boundary", () => {
  const data = { ...result(null), left: displayState("1.00505", "1.005") };
  render(<>
    <HistoricalMetricCards data={{ ...data, right: { ...data.left, date: "2026-08-09" } }} />
    <HistoricalAllocationView data={data} onFilter={vi.fn()} />
    <CompositionChart title="Account composition" currency="USD" centerValue="1.00505" centerCaption="Valued subtotal"
      ariaLabel="Account composition" summary="Synthetic account" items={[{ key: "cash", label: "Cash", amount: "1.00505", shareBps: 10000 }]} />
  </>);
  for (const metric of screen.getAllByLabelText("Assets")) {
    expect(metric).toHaveTextContent("US$1.01");
    expect(within(metric).getByTitle("US$1.01")).toHaveTextContent("US$1.01");
  }
  for (const name of ["Asset allocation", "Account composition"]) {
    expect(screen.getByRole("img", { name })).toHaveTextContent("US$1.01");
  }
  const option = [...chart.options].reverse().find(value => (value as { series?: Array<{ id?: string }> }).series?.[0]?.id === "historical-allocation") as {
    tooltip: { formatter: (point: unknown) => string }; series: Array<{ data: Array<{ value: number }> }>;
  };
  expect(option.tooltip.formatter({ name: "cash" })).toContain("US$1.01 · 100%");
  expect(option.series[0].data[0].value).toBe(1.00505);
  expect(data.left.assets).toBe("1.00505");
});

it("keeps partial and provisional metric projections separate from known-only donut totals", () => {
  const data = { ...result(null), left: displayState("1.00505", "1.005", true) };
  const view = render(<><HistoricalMetricCards data={data} /><HistoricalAllocationView data={data} onFilter={vi.fn()} /></>);
  const assets = screen.getByLabelText("Assets");
  expect(within(assets).getByTitle("US$1.00")).toHaveTextContent("US$1.00");
  expect(screen.getByRole("img", { name: "Asset allocation" })).toHaveTextContent("US$1.01");
  const estimate = data.left.displayValuation!.assets;
  estimate.total = { amount: "20", currency: "USD" };
  estimate.knownSubtotal = estimate.total;
  estimate.quality.availability = "full" as typeof estimate.quality.availability;
  estimate.quality.valuedCount = 2;
  estimate.quality.unpricedCount = 0;
  estimate.quality.provisionalCount = 1;
  view.rerender(<><HistoricalMetricCards data={data} /><HistoricalAllocationView data={data} onFilter={vi.fn()} /></>);
  expect(within(screen.getByLabelText("Assets")).getByTitle("US$20.00")).toHaveTextContent("US$20.00");
  expect(screen.getByRole("img", { name: "Asset allocation" })).toHaveTextContent("US$1.01");
});

it("opens the correct read-only account from a chart bar when names collide and restores focus", async () => {
  const data = result(state("2026-08-09", "20"));
  let openedTrigger: HTMLButtonElement | null = null;
  function Harness() {
    const [selected, setSelected] = useState<string | null>(null);
    const trigger = useRef<HTMLButtonElement | null>(null);
    return <>
      <HistoricalAccountDeltaView data={data} onOpen={(key, button) => { trigger.current = button; openedTrigger = button; setSelected(key); }} />
      <Sheet open={selected !== null} onOpenChange={open => { if (!open) setSelected(null); }}>
        <SheetContent finalFocus={() => trigger.current}>
          <SheetHeader><SheetTitle>Read-only account detail</SheetTitle></SheetHeader>
          <p>Selected key: {selected}</p>
        </SheetContent>
      </Sheet>
    </>;
  }
  render(<Harness />);
  const delta = screen.getByRole("region", { name: "Account balance changes" });
  expect(within(delta).getAllByRole("button", { name: "Duplicate" })).toHaveLength(2);
  await userEvent.click(within(delta).getByRole("button", { name: "chart-on-second" }));
  expect(await screen.findByRole("dialog")).toHaveTextContent("Selected key: second");
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(openedTrigger).toHaveFocus();
});

it("labels paired bars with both dates and returns the exact same-source tooltip values", () => {
  chart.options.length = 0;
  const data = result(state("2026-08-09", "20"));
  render(<HistoricalAllocationView data={data} onFilter={vi.fn()} />);
  const comparison = screen.getByRole("region", { name: "Asset category comparison" });
  expect(comparison).toHaveTextContent("A · 2026-08-01");
  expect(comparison).toHaveTextContent("B · 2026-08-09");
  const option = chart.options[chart.options.length - 1] as { tooltip: { formatter: (point: unknown) => string };
    series: Array<{ data: Array<{ name: string }> }> };
  expect(option.series[0].data[0].name).toBe("cash");
  expect(option.tooltip.formatter({ name: "cash", data: { name: "cash" }, seriesName: "A" })).toContain(`${formatAmount("10", "CNY")} · 100%`);
  expect(option.tooltip.formatter({ name: "cash", data: { name: "cash" }, seriesName: "B" })).toContain(`${formatAmount("20", "CNY")} · 100%`);
  expect(within(comparison).getByText("Full source data table")).toBeInTheDocument();
});

it("uses an honest empty chart state for nonempty all-zero asset categories", () => {
  const single = result(null);
  single.left = state("2026-08-01", "0");
  const first = render(<HistoricalAllocationView data={single} onFilter={vi.fn()} />);
  const allocation = screen.getByRole("region", { name: "Asset allocation" });
  expect(within(allocation).queryByRole("img")).not.toBeInTheDocument();
  expect(allocation).toHaveTextContent("Known assets are zero; no composition chart.");
  expect(within(allocation).getByRole("table")).toHaveTextContent("0");
  first.unmount();
  const paired = result(state("2026-08-09", "0"));
  paired.left = state("2026-08-01", "0");
  render(<HistoricalAllocationView data={paired} onFilter={vi.fn()} />);
  const comparison = screen.getByRole("region", { name: "Asset category comparison" });
  expect(within(comparison).queryByRole("img")).not.toBeInTheDocument();
  expect(comparison).toHaveTextContent("Known category amounts are zero on both sides");
  expect(within(comparison).getByRole("table")).toHaveTextContent("0");
});

it("labels an account delta as estimated from endpoint quality, including carried prices with verified coverage", () => {
  const amount = (value: string) => ({ amount: value, currency: "CNY" });
  const quality = (coverage: string, carriedCount = 0) => ({ availability: "full", coverage, includedCount: 1, valuedCount: 1, unpricedCount: 0, provisionalCount: coverage === "provisional" ? 1 : 0, carriedCount, staleCount: 0, oldestMarketDate: carriedCount ? "2026-08-01" : null, newestMarketDate: null, reasons: carriedCount ? ["carried_nav"] : coverage === "provisional" ? ["history_unverified"] : [] });
  const valued = (value: string, coverage: string, carriedCount = 0) => ({ status: "active", included: true, complete: true, currency: "CNY", nativeAmount: value, baseAmount: value, quantity: null, assetClass: "fund", manual: false, valueSourceAt: "", valueSourceId: "", price: null, fx: null, missing: [], missingDetails: [], displayValuation: { currency: "CNY", total: amount(value), knownSubtotal: amount(value), quality: quality(coverage, carriedCount) } });
  const row = (leftCoverage: string, carriedCount = 0): HistoricalOverviewRow => ({
    key: "broker", accountId: "", holdingId: "", instrumentId: "", parentKey: "", kind: "account", name: "Broker", role: "asset",
    left: valued("100", leftCoverage, carriedCount) as HistoricalOverviewCell, right: valued("120", "verified") as HistoricalOverviewCell,
    baseChange: "20", nativeChange: "20", quantityChange: null, changed: true, estimatedBaseChange: "20",
  });
  const data = (item: HistoricalOverviewRow): HistoricalOverviewResult => ({ ...result(state("2026-08-09", "20")), rows: [item] });
  const note = "Estimated net-worth change, including cash flows";
  const shown = (item: HistoricalOverviewRow) => {
    const view = render(<HistoricalAccountDeltaView data={data(item)} onOpen={vi.fn()} />);
    return { view, region: screen.getByRole("region", { name: "Account balance changes" }) };
  };
  const verified = shown(row("verified"));
  expect(verified.region).not.toHaveTextContent(note);
  verified.view.unmount();
  const carried = shown(row("verified", 1));
  expect(carried.region).toHaveTextContent(note);
  carried.view.unmount();
  const provisional = shown(row("provisional"));
  expect(provisional.region).toHaveTextContent(note);
  provisional.view.unmount();
  const nonContract = shown(row("estimated"));
  expect(nonContract.region).not.toHaveTextContent(note);
});

it("keeps negative sub-cent allocation tooltip values exact", () => {
  chart.options.length = 0;
  const data = result(null);
  data.left = { ...state("2026-08-01", "-0.00012"), byClass: [{ key: "cash", amount: "-0.00012", share: "-100" }] };
  render(<HistoricalAllocationView data={data} onFilter={vi.fn()} />);
  const option = chart.options[chart.options.length - 1] as { tooltip: { formatter: (point: unknown) => string } };
  expect(option.tooltip.formatter({ name: "cash", data: { name: "cash" } })).toContain(formatAmount("-0.00012", "CNY"));
});

import { useRef, useState } from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import type { HistoricalOverviewResult, HistoricalOverviewRow, HistoricalOverviewState } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import { HistoricalAccountDeltaView, HistoricalAllocationView } from "./HistoricalPanorama";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";

const chart = vi.hoisted(() => ({ options: [] as unknown[] }));
vi.mock("@/components/charts/EChart", () => ({
  EChart: ({ option, ariaLabel, onSelectName }: {
    option: { series?: Array<{ data?: Array<{ name?: string } | null> }> };
    ariaLabel: string; onSelectName?: (name: string) => void;
  }) => {
    chart.options.push(option);
    return <div role="img" aria-label={ariaLabel}>{option.series?.[0]?.data?.map(item => item?.name &&
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
  expect(option.tooltip.formatter({ name: "cash", data: { name: "cash" }, seriesName: "A" })).toContain("10 CNY · 100%");
  expect(option.tooltip.formatter({ name: "cash", data: { name: "cash" }, seriesName: "B" })).toContain("20 CNY · 100%");
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

it("keeps negative sub-cent allocation tooltip values exact", () => {
  chart.options.length = 0;
  const data = result(null);
  data.left = { ...state("2026-08-01", "-0.00012"), byClass: [{ key: "cash", amount: "-0.00012", share: "-100" }] };
  render(<HistoricalAllocationView data={data} onFilter={vi.fn()} />);
  const option = chart.options[chart.options.length - 1] as { tooltip: { formatter: (point: unknown) => string } };
  expect(option.tooltip.formatter({ name: "cash", data: { name: "cash" } })).toContain("-0.00012 CNY");
});

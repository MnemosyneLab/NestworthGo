import { expect, it } from "vitest";
import type { HistoricalOverviewAllocation, HistoricalOverviewResult, HistoricalOverviewRow, HistoricalOverviewState } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import { accountChanges, alignedAllocations, allocationMode, comparableAccount, geometry, missingAssetLeaves, validShare } from "./historicalChartModel";

const allocation = (key: string, amount: string, share = "50"): HistoricalOverviewAllocation => ({ key, amount, share });
const state = (patch: Partial<HistoricalOverviewState> = {}): HistoricalOverviewState => ({
  date: "2026-08-01", cutoffAt: "2026-08-01T23:59:59Z", current: false, currency: "CNY", complete: true,
  assets: "100", liabilities: "0", netWorth: "100", knownAssets: "100", knownLiabilities: "0", byClass: [], byCurrency: [], ...patch,
});
const cell = (baseAmount: string | null, patch: Record<string, unknown> = {}) => ({
  status: "active", included: true, complete: baseAmount !== null, currency: "CNY", nativeAmount: baseAmount,
  baseAmount, quantity: null, assetClass: "cash", manual: false, valueSourceAt: "", valueSourceId: "", price: null, fx: null, missing: [], ...patch,
});
const row = (key: string, role: string, baseChange: string | null, patch: Partial<HistoricalOverviewRow> = {}): HistoricalOverviewRow => ({
  key, parentKey: "", kind: "account", name: key, role, left: cell("100"), right: cell("100"),
  baseChange, nativeChange: null, quantityChange: null, changed: true, ...patch,
});

it("uses exact decimal eligibility for donut, zero and signed bars", () => {
  expect(allocationMode(state(), [allocation("cash", "100", "100")])).toBe("donut");
  expect(allocationMode(state({ complete: false, liabilities: null, netWorth: null }), [allocation("cash", "100", "100")])).toBe("donut");
  expect(allocationMode(state(), [allocation("cash", "-1", "-1"), allocation("stock", "101", "101")])).toBe("bars");
  expect(allocationMode(state({ knownAssets: "0" }), [allocation("cash", "0", "0")])).toBe("zero");
  expect(allocationMode(state({ knownAssets: "100" }), [])).toBe("bars");
  expect(allocationMode(state(), [allocation("cash", "100", "not-a-percent")])).toBe("bars");
  expect(validShare("100")).toBe(true);
  expect(validShare("100.00001")).toBe(false);
  expect(geometry("0.00012")).toBe(0.00012);
  expect(geometry(`0.${"0".repeat(400)}1`)).toBeNull();
  expect(geometry("9".repeat(400))).toBeNull();
  expect(geometry(null)).toBeNull();
});

it("aligns one-sided categories without inventing zero, preserving a common order", () => {
  const aligned = alignedAllocations(
    [allocation("A-only", "4"), allocation("both", "2")],
    [allocation("B-only", "0.00012"), allocation("both", "3")],
  );
  expect(aligned.map(item => item.key)).toEqual(["both", "B-only", "A-only"]);
  expect(aligned[1].left).toBeUndefined();
  expect(aligned[2].right).toBeUndefined();
});

it("counts only included, missing asset leaves on each side, not parents or debts", () => {
  const rows = [
    row("parent", "asset", null, { left: cell(null) }),
    row("missing-asset", "asset", null, { kind: "holding", parentKey: "parent", left: cell(null, { missing: ["fx_rate"] }) }),
    row("excluded", "asset", null, { kind: "cash", parentKey: "parent", left: cell(null, { included: false }) }),
    row("debt", "liability", null, { kind: "balance", parentKey: "debt-parent", left: cell(null) }),
  ];
  const data = { left: state({ assets: null }), right: state({ liabilities: null }), rows } as HistoricalOverviewResult;
  expect(missingAssetLeaves(data, "left")).toBe(1);
  expect(missingAssetLeaves(data, "right")).toBe(0);
});

it("sorts comparable changed accounts by exact absolute balance delta and separates debt and scope", () => {
  const rows = [
    row("tiny", "asset", "0.00012"), row("zero", "asset", "0", { changed: true }),
    row("large-negative", "asset", "-100"), row("large-positive", "asset", "100"),
    row("missing-fx", "asset", null, { left: cell(null), right: cell("12") }),
    row("new-account", "asset", null, { left: null }),
    row("scope", "asset", "5", { right: cell("105", { included: false }) }),
    row("status", "asset", "5", { right: cell("105", { status: "archived" }) }),
    row("debt", "liability", "-100"),
    row("same-evidence", "asset", "0", { changed: false }),
    row("unchanged-unknown", "asset", null, { left: cell(null), right: cell(null), changed: false }),
  ];
  expect(accountChanges(rows, "asset").comparable.map(item => item.key)).toEqual(["large-negative", "large-positive", "tiny", "zero"]);
  expect(accountChanges(rows, "asset").separate.map(item => item.key)).toEqual(["missing-fx", "new-account", "scope", "status", "unchanged-unknown"]);
  expect(accountChanges(rows, "liability").comparable.map(item => item.key)).toEqual(["debt"]);
  expect(comparableAccount(rows[6])).toBe(false);
});

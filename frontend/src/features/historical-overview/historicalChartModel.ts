import { fullDisplayAmount, hasDisplay } from "@/components/valuation/valuationQuality";
import { compareCanonical } from "@/lib/money";
import { chartNumber } from "@/components/charts/chartTheme";
import type { HistoricalOverviewAllocation, HistoricalOverviewResult, HistoricalOverviewRow, HistoricalOverviewState } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";

const DECIMAL = /^-?(?:0|[1-9]\d*)(?:\.\d+)?$/;
const magnitude = (value: string) => value.startsWith("-") ? value.slice(1) : value;
export const isZero = (value: string) => DECIMAL.test(value) && compareCanonical(value, "0") === 0;

// This model chooses geometry and order only. It never derives a financial amount.
export function geometry(value: string | null | undefined): number | null {
  if (value == null || !DECIMAL.test(value)) return null;
  const number = chartNumber(value);
  if (number === null || (number === 0 && !isZero(value))) return null;
  return number;
}

export function validShare(value: string): boolean {
  return DECIMAL.test(value) && compareCanonical(value, "0") >= 0 && compareCanonical(value, "100") <= 0;
}

export function allocationMode(state: HistoricalOverviewState, items: HistoricalOverviewAllocation[]): "donut" | "bars" | "zero" {
  if (items.length === 0) return isZero(state.knownAssets) ? "zero" : "bars";
  if (isZero(state.knownAssets) && items.every(item => isZero(item.amount))) return "zero";
  const safe = compareCanonical(state.knownAssets, "0") > 0 && items.every(item =>
    validShare(item.share) && compareCanonical(item.amount, "0") >= 0 && geometry(item.amount) !== null && geometry(item.share) !== null,
  );
  return safe ? "donut" : "bars";
}

export function missingAssetLeaves(result: HistoricalOverviewResult, side: "left" | "right"): number {
  return new Set((result.rows ?? []).filter(row => {
    const cell = row[side];
    return row.kind !== "account" && Boolean(row.parentKey) && row.role === "asset" &&
      cell?.included && !cell.complete && cell.baseAmount == null;
  }).map(row => row.key)).size;
}

export function alignedAllocations(left: HistoricalOverviewAllocation[], right: HistoricalOverviewAllocation[]) {
  const a = new Map(left.map(item => [item.key, item]));
  const b = new Map(right.map(item => [item.key, item]));
  return [...new Set([...a.keys(), ...b.keys()])].sort((x, y) => {
    const bx = b.get(x)?.amount;
    const by = b.get(y)?.amount;
    if (bx && by) {
      const order = compareCanonical(by, bx);
      if (order) return order;
    } else if (bx) return -1;
    else if (by) return 1;
    return x.localeCompare(y);
  }).map(key => ({ key, left: a.get(key), right: b.get(key) }));
}

/** New display amounts use the backend's guarded estimate. Strict legacy
 * differences remain available only to legacy projections without quality. */
export function historicalRowChange(row: HistoricalOverviewRow): string | null {
  return hasDisplay(row.left?.displayValuation) && hasDisplay(row.right?.displayValuation)
    ? row.estimatedBaseChange ?? null : row.baseChange ?? null;
}

export function comparableAccount(row: HistoricalOverviewRow): boolean {
  return row.kind === "account" && Boolean(row.left && row.right && comparableStatuses(row.left.status, row.right.status) && row.left.included && row.right.included &&
    fullDisplayAmount(row.left.displayValuation,row.left.complete ? row.left.baseAmount : null) != null && fullDisplayAmount(row.right.displayValuation,row.right.complete ? row.right.baseAmount : null) != null && historicalRowChange(row) != null);
}

export function comparableStatuses(left: string, right: string): boolean {
  return left === right || ([left, right].every(status => status === "active" || status === "zero"));
}

export function accountChanges(rows: HistoricalOverviewRow[], role: "asset" | "liability") {
  const accounts = rows.filter(row => row.kind === "account" && row.role === role);
  const comparable = accounts.filter(row => row.changed && comparableAccount(row)).sort((x, y) =>
    compareCanonical(magnitude(historicalRowChange(y)!), magnitude(historicalRowChange(x)!)) || x.key.localeCompare(y.key),
  );
  return { comparable, separate: accounts.filter(row => !comparableAccount(row)) };
}

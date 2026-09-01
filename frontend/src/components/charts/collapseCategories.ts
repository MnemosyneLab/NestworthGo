import { addCanonical, isPositiveCanonical } from "@/lib/money";

export interface ChartCategory {
  key: string;
  label: string;
  amount: string;
  shareBps: number;
}

/**
 * Collapses already-aggregated Go categories for a pie chart. Totals stay
 * canonical strings; this does not re-derive amounts from holdings.
 */
export function collapseChartCategories(items: ChartCategory[], maxVisible = 5, otherKey = "other"): ChartCategory[] {
  const positive = items.filter((item) => isPositiveCanonical(item.amount));
  if (positive.length <= maxVisible + 1) {
    return positive;
  }
  const head = positive.slice(0, maxVisible);
  const rest = positive.slice(maxVisible);
  const amount = rest.reduce((sum, item) => addCanonical(sum, item.amount) || sum, "0");
  const shareBps = rest.reduce((sum, item) => sum + item.shareBps, 0);
  return [...head, { key: otherKey, label: otherKey, amount, shareBps }];
}

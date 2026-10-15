import { compareCanonical } from "@/lib/money";
import { instrumentDisplayLabel } from "@/lib/instrumentDisplay";
import type { HoldingAmountsDTO, HoldingSubtotalDTO, InstrumentHoldingGroupDTO, InstrumentHoldingsDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics/models";

/** Mirrors domain.HoldingGroupOrder; the backend already sends groups in this order. */
export const HOLDING_GROUP_ORDER = ["stock", "fund", "bond", "bank_wealth", "precious_metal", "crypto", "cash_like", "other"] as const;
export type HoldingGroupKey = typeof HOLDING_GROUP_ORDER[number];

export type HoldingView = "instrument" | "account";
export type HoldingSortKey = "name" | "quantity" | "totalCost" | "currentValue" | "unrealizedGain";
export type HoldingSort = { key: HoldingSortKey; desc: boolean };
export type HoldingRow = { id: string; instrument: InstrumentHoldingsDTO; amounts: HoldingAmountsDTO; accountId?: string; accountName?: string; holdingId?: string };

export type VisibleHoldingGroup = {
  key: HoldingGroupKey;
  rows: HoldingRow[];
  subtotals: HoldingSubtotalDTO[];
  /** Distinct instruments in the group, independent of the by-account view. */
  instrumentCount: number;
};

export function normalizeHoldingGroupKey(key: string): HoldingGroupKey {
  return (HOLDING_GROUP_ORDER as readonly string[]).includes(key) ? key as HoldingGroupKey : "other";
}

const hasQuantity = (amounts: HoldingAmountsDTO) => compareCanonical(amounts.quantity, "0") !== 0;

/**
 * Turns the backend groups into display rows. It only filters, orders and
 * re-labels; every amount and subtotal comes from the backend unchanged.
 */
export function visibleHoldingGroups(
  groups: readonly InstrumentHoldingGroupDTO[],
  options: { view: HoldingView; showZero: boolean; sort: HoldingSort; language: string },
): VisibleHoldingGroup[] {
  const merged = new Map<HoldingGroupKey, { instruments: InstrumentHoldingsDTO[]; subtotals: HoldingSubtotalDTO[] }>();
  for (const group of groups) {
    const key = normalizeHoldingGroupKey(group.key);
    const entry = merged.get(key) ?? { instruments: [], subtotals: [] };
    entry.instruments.push(...(group.instruments ?? []));
    entry.subtotals.push(...(group.subtotals ?? []));
    merged.set(key, entry);
  }
  const result: VisibleHoldingGroup[] = [];
  for (const key of HOLDING_GROUP_ORDER) {
    const entry = merged.get(key);
    if (!entry) continue;
    const instruments = entry.instruments
      .map((instrument) => ({ ...instrument, holdings: (instrument.holdings ?? []).filter((member) => options.showZero || hasQuantity(member.amounts)) }))
      .filter((instrument) => instrument.holdings.length > 0);
    const rows: HoldingRow[] = options.view === "instrument"
      ? instruments.map((instrument) => ({ id: `instrument:${instrument.instrumentId}`, instrument, amounts: instrument.amounts }))
      : instruments.flatMap((instrument) => instrument.holdings.map((member) => ({
        id: `holding:${member.holdingId}`, instrument, amounts: member.amounts, accountId: member.accountId, accountName: member.accountName, holdingId: member.holdingId,
      })));
    if (rows.length === 0) continue;
    result.push({ key, rows: sortHoldingRows(rows, options.sort, options.language), subtotals: entry.subtotals, instrumentCount: instruments.length });
  }
  return result;
}

export function sortHoldingRows(rows: readonly HoldingRow[], sort: HoldingSort, language: string): HoldingRow[] {
  const label = (row: HoldingRow) => instrumentDisplayLabel(row.instrument);
  return [...rows].sort((a, b) => {
    let cmp = 0;
    if (sort.key === "name") cmp = label(a).localeCompare(label(b), language);
    else if (sort.key === "quantity") {
      const unit = a.instrument.quantityUnit.localeCompare(b.instrument.quantityUnit);
      if (unit) return unit;
      cmp = compareCanonical(a.amounts.quantity, b.amounts.quantity);
    } else {
      const currency = a.instrument.quoteCurrency.localeCompare(b.instrument.quoteCurrency);
      if (currency) return currency;
      const left = a.amounts[sort.key]?.amount, right = b.amounts[sort.key]?.amount;
      if (left == null || right == null) {
        // Unknown values always sort last, whichever direction is chosen.
        if (left == null && right != null) return 1;
        if (right == null && left != null) return -1;
      } else cmp = compareCanonical(left, right);
    }
    return (sort.desc ? -cmp : cmp) || label(a).localeCompare(label(b), language) || a.instrument.name.localeCompare(b.instrument.name, language) || a.id.localeCompare(b.id);
  });
}

/**
 * Direction of a canonical signed decimal, used only to pick an arrow and tone.
 * Values that display as 0.00 are flat so the arrow never contradicts the text.
 */
export function amountDirection(amount: string | null | undefined): "up" | "down" | "flat" | undefined {
  if (amount == null) return undefined;
  if (compareCanonical(amount, "0.005") >= 0) return "up";
  if (compareCanonical(amount, "-0.005") <= 0) return "down";
  return "flat";
}

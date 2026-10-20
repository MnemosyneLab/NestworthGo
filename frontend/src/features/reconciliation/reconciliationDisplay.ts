import type { TFunction } from "i18next";
import { formatAmount, formatAuditMoney } from "@/lib/money";
import { metalUnitLabel } from "@/lib/preciousMetals";

type CheckpointItem = { kind?: string; value?: string; currency?: string; referenceValue?: string; detail?: unknown };

// Frozen checkpoints carry quantity units in detail; currency describes price/cost.
// Audit reviews (exact) keep digits hidden by currency rounding visible; cards use ordinary display formatting.
function presentCheckpointNumber(t: TFunction, item: CheckpointItem, raw: string, exact: boolean): string {
  const holding = item.kind === "holding";
  if (holding) {
    const detail = item.detail as { quantityUnit?: string } | undefined;
    return [exact ? raw : formatAmount(raw), metalUnitLabel(detail?.quantityUnit, t)].filter(Boolean).join(" ");
  }
  if (exact) return item.currency ? formatAuditMoney(raw, item.currency) : formatAmount(raw);
  return [formatAmount(raw, item.currency), item.currency].filter(Boolean).join(" ");
}

export function checkpointValue(t: TFunction, item: CheckpointItem, exact = false): string {
  if (item.kind === "scope") return "";
  if (!item.value) return t("reconciliation.unavailable");
  return presentCheckpointNumber(t, item, item.value, exact);
}

/** The user's source figure, shown in the same unit and currency format as the ledger value it is compared to. */
export function checkpointReference(t: TFunction, item: CheckpointItem, exact = false): string {
  if (item.kind === "scope" || !item.referenceValue) return "";
  return presentCheckpointNumber(t, item, item.referenceValue, exact);
}

export function sourceText(source?: { reference?: string; asOfDate?: string; asOfTime?: string; timezone?: string }): string {
  return [source?.reference, [source?.asOfDate, source?.asOfTime, source?.timezone].filter(Boolean).join(" ")].filter(Boolean).join(" · ");
}

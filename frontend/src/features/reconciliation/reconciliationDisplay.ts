import { signedAccountAmount } from "@/lib/liabilityDisplay";
import type { TFunction } from "i18next";
import { formatAmount, formatAnnualRatePercent, formatAuditMoney, formatQuantity } from "@/lib/money";
import { formatCivilDate } from "@/lib/time";
import { metalUnitLabel } from "@/lib/preciousMetals";

type CheckpointItem = { kind?: string; value?: string; currency?: string; referenceValue?: string; detail?: unknown };

// Frozen checkpoints carry quantity units in detail; currency describes price/cost.
// Audit reviews (exact) keep digits hidden by currency rounding visible; cards use ordinary display formatting.
function presentCheckpointNumber(t: TFunction, item: CheckpointItem, raw: string, exact: boolean): string {
  const holding = item.kind === "holding";
  if (holding) {
    const detail = item.detail as { quantityUnit?: string } | undefined;
    return [exact ? raw : formatQuantity(raw), metalUnitLabel(detail?.quantityUnit, t)].filter(Boolean).join(" ");
  }
  if (!item.currency) return exact ? raw : formatQuantity(raw);
  return exact ? formatAuditMoney(raw, item.currency) : formatAmount(raw, item.currency);
}

/** Review lines show money at the currency scale, and quantities without trailing zeros. */
export function formatRecordedAmount(amount?: string | null, currency?: string | null): string {
  if (!amount) return "";
  return currency ? formatAuditMoney(amount, currency) : formatQuantity(amount);
}

export function formatRecordedRate(rate?: string | null): string {
  if (!rate) return "";
  return formatAnnualRatePercent(rate);
}

export function formatRecordedDate(value?: string | null): string {
  if (!value) return "";
  return formatCivilDate(value);
}

export function checkpointValue(t: TFunction, item: CheckpointItem, exact = false, liability = false): string {
  if (item.kind === "scope") return "";
  if (!item.value) return t("reconciliation.unavailable");
  return presentCheckpointNumber(t, item, item.kind === "balance" ? signedAccountAmount(item.value, liability) : item.value, exact);
}

/** Preserve free-text scope evidence; format numeric sources like the ledger value they are compared to. */
export function checkpointReference(t: TFunction, item: CheckpointItem, exact = false, liability = false): string {
  if (!item.referenceValue) return "";
  if (item.kind === "scope") return item.referenceValue;
  return presentCheckpointNumber(t, item, item.kind === "balance" && /^\d+(\.\d+)?$/.test(item.referenceValue) ? signedAccountAmount(item.referenceValue, liability) : item.referenceValue, exact);
}

export function sourceText(source?: { reference?: string; asOfDate?: string; asOfTime?: string; timezone?: string }): string {
  return [source?.reference, [source?.asOfDate, source?.asOfTime, source?.timezone].filter(Boolean).join(" ")].filter(Boolean).join(" · ");
}

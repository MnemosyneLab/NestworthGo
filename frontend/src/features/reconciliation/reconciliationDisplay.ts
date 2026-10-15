import type { TFunction } from "i18next";
import { formatAmount } from "@/lib/money";
import { metalUnitLabel } from "@/lib/preciousMetals";

// Frozen checkpoints carry quantity units in detail; currency describes price/cost.
// Audit reviews retain canonical decimal text; cards use ordinary display formatting.
export function checkpointValue(t: TFunction, item: { kind?: string; value?: string; currency?: string; detail?: unknown }, exact = false): string {
  if (item.kind === "scope") return "";
  if (!item.value) return t("reconciliation.unavailable");
  const holding = item.kind === "holding";
  const detail = item.detail as { quantityUnit?: string } | undefined;
  const unit = holding ? metalUnitLabel(detail?.quantityUnit, t) : item.currency;
  return [exact ? item.value : formatAmount(item.value, holding ? undefined : item.currency), unit].filter(Boolean).join(" ");
}

export function sourceText(source?: { reference?: string; asOfDate?: string; asOfTime?: string; timezone?: string }): string {
  return [source?.reference, [source?.asOfDate, source?.asOfTime, source?.timezone].filter(Boolean).join(" ")].filter(Boolean).join(" · ");
}

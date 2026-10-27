import { useContext } from "react";
import { ReconciliationLedgerContext } from "./ReconciliationLedgerContext";
import { useTranslation } from "react-i18next";
import { checkpointValue, checkpointReference, formatRecordedAmount, formatRecordedDate } from "./reconciliationDisplay";
import { useReconciliationTimestamp } from "./useReconciliationTimestamp";
import { Notice } from "./ReconciliationParts";
import { compareCanonical } from "@/lib/money";
import type { ReconciliationItem } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";

export function LedgerCheckpoint({ item, evidence = false }: { item: ReconciliationItem; evidence?: boolean }) {
  const { t } = useTranslation();
  const liability = useContext(ReconciliationLedgerContext)(item.accountId);
  const timestamp = useReconciliationTimestamp();
  const label = item.kind === "scope" ? t("reconciliation.kind.scope") : item.label;
  const numeric = /^-?\d+(\.\d+)?$/;
  const mismatch = item.referenceComparable && numeric.test(item.value ?? "") && numeric.test(item.referenceValue ?? "") && compareCanonical(item.value!, item.referenceValue!) !== 0;
  return <section className="flex min-w-0 flex-col gap-2 rounded-xl border border-border bg-muted/30 p-3 text-sm" aria-label={t("reconciliation.ledgerCheckpoint")}>
    <p className="break-words font-semibold">{item.accountName} · {label} · {t(`reconciliation.kind.${item.kind}`)}</p>
    {item.kind === "scope" ? <p>{t("reconciliation.scopeHelp")}</p> : <p className="num break-words"><span className="font-medium">{t(item.kind === "holding" ? "reconciliation.ledgerQuantity" : item.kind === "product" ? "availableFunds.principal" : "reconciliation.ledgerBalance")}: </span>{checkpointValue(t, item, true, liability)}</p>}
    {item.cost && <p className="num break-words">{t("reconciliation.unitCost")}: {formatRecordedAmount(item.cost, item.currency)}</p>}
    <p className="text-xs text-muted-foreground">{t("reconciliation.snapshotHint")}</p>
    {item.checkedAt && <p className="num text-xs text-muted-foreground">{t("reconciliation.checkedAt")}: {timestamp(item.checkedAt)}</p>}
    {evidence && <>
      <p>{t("reconciliation.outcome")}: <strong>{t(`reconciliation.${item.outcome}`)}</strong></p>
      {item.source.asOfDate && <p>{t("reconciliation.sourceDate")}: {formatRecordedDate(item.source.asOfDate)} {item.source.asOfTime} {item.source.timezone}</p>}
      {item.source.reference && <p className="break-words">{t("reconciliation.source")}: {item.source.reference}</p>}
      {item.referenceValue && <p className="num break-words">{t("reconciliation.referenceValue")}: {checkpointReference(t, item, true, liability)}</p>}
      <p className="text-xs text-muted-foreground">{t(item.referenceComparable ? "reconciliation.comparable" : "reconciliation.notCompared")}</p>
      {item.note && <p className="break-words">{t("reconciliation.note")}: {item.note}</p>}
      {mismatch && <Notice tone="warning" role="alert">{t("reconciliation.mismatch")}</Notice>}
    </>}
    {item.recheckRequired && <Notice tone="warning" role="status">{t("reconciliation.pendingReview")}</Notice>}
  </section>;
}

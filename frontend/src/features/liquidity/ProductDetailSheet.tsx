import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { displayEnum, displayError } from "@/lib/display";
import { formatAmount, multiplyCanonical } from "@/lib/money";
import { formatTimestamp, localDateTimeInTimeZone } from "@/lib/time";
import { useHistoryOrigin } from "@/queries/history";
import {
  useAppendProductValuation,
  usePreviewProductOperation,
  useProduct,
  useProductOperations,
  useRecordProductOperation,
  useReleaseReservation
} from "@/queries/liquidity";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { SettleProductCommand } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import type { ProductCommandRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import { moneyText, productStateLabel } from "./liquidityDisplay";
import { ProductEditSheet } from "./ProductEditSheet";
import { ProductPreview } from "./ProductPreview";
import { ProductTimeFields, type ProductLocalTime } from "./ProductTimeFields";
import { useReviewedCommand } from "./useReviewedCommand";


export function ProductDetailSheet({
  productId,
  open,
  onOpenChange,
}: {
  productId: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const detail = useProduct(productId ?? "", Boolean(productId) && open);
  const history = useHistoryOrigin();
  const [localTime, setLocalTime] = useState<ProductLocalTime>({});
  const operations = useProductOperations(productId ?? "", Boolean(productId) && open);
  const preview = usePreviewProductOperation();
  const record = useRecordProductOperation();
  const valuation = useAppendProductValuation();
  const release = useReleaseReservation();
  const isSaving = record.isPending || valuation.isPending || release.isPending;
  const [action, setAction] = useState<"settle" | "interest" | "value" | "undo" | null>(null);
  const [amount, setAmount] = useState("");
  const [interest, setInterest] = useState("0");
  const [fee, setFee] = useState("0");
  const [editTerms, setEditTerms] = useState(false);
  const [paidThrough, setPaidThrough] = useState("");
  const [remainingInterest, setRemainingInterest] = useState("");
  const [error, setError] = useState<string>();
  const product = detail.data?.product;
  const today = history.data?.timezone ? localDateTimeInTimeZone(history.data.timezone)?.date : undefined;
  const paidThroughMax = [today, product?.maturityOn].filter((date): date is string => Boolean(date)).sort()[0];
  const operationHistory = operations.data?.operations ?? [];
  const reversedOperationIds = new Set(operationHistory.map((operation) => operation.reversesOperationId).filter(Boolean));
  const latestOperation = operationHistory.find((operation) => !operation.reversesOperationId && !reversedOperationIds.has(operation.id));
  const undoTarget = latestOperation?.kind === "value_observation" ? undefined : latestOperation;
  const releaseIds = useMemo(
    () => (detail.data?.reservations ?? []).filter((reservation) => !reservation.releasedAt).map((reservation) => reservation.id),
    [detail.data?.reservations],
  );
  const command = useMemo((): ProductCommandRequest | null => {
    if (!product || !action) {
      return null;
    }
    if (action === "settle") {
      const settle: SettleProductCommand = product.kind === "term_deposit"
        ? { productId: product.id, returnedPrincipal: amount, interest, fee, grossProceeds: null, effectiveAt: "", ...localTime, releaseReservationIds: releaseIds }
        : { productId: product.id, grossProceeds: amount, fee, effectiveAt: "", ...localTime, releaseReservationIds: releaseIds, returnedPrincipal: null, interest: null };
      return { kind: "settle", settle };
    }
    if (action === "interest") {
      return { kind: "receive_interest", receiveInterest: { productId: product.id, amount, effectiveAt: "", ...localTime, interestPaidThroughOn: paidThrough || null, remainingInterest: remainingInterest || null } };
    }
    if (action === "undo") {
      if (!undoTarget) {
        return null;
      }
      return { kind: "undo", undo: { operationId: undoTarget.id } };
    }
    return null;
  }, [action, amount, fee, interest, localTime, paidThrough, product, releaseIds, remainingInterest, undoTarget]);
  const { reviewed, setReviewed } = useReviewedCommand(command);
  function beginAction(next: typeof action) {
    setAction(next);
    setReviewed(null);
    setError(undefined);
    setAmount(next === "settle" && product?.kind === "term_deposit" ? product.principal.amount : "");
    setInterest("0");
    setFee("0");
    setLocalTime({});
    setPaidThrough("");
    setRemainingInterest("");
  }
  if (!productId) {
    return null;
  }
  if (editTerms) return <ProductEditSheet productId={productId} open={open} onOpenChange={(next) => { if (!next) setEditTerms(false); }} />;
  return (
    <Sheet open={open} onOpenChange={(next) => { if (!isSaving) onOpenChange(next); }}>
      <SheetContent size="lg" onChangeCapture={() => setReviewed(null)}>
        <SheetHeader>
          <SheetTitle>{product?.name ?? t("availableFunds.products")}</SheetTitle>
        </SheetHeader>
        {detail.isLoading && <p>{t("ui.state.loadingPage")}</p>}
        {detail.isError && <p role="alert">{t("availableFunds.loadError")}</p>}
        {product && (
          <div className="flex flex-col gap-3 text-sm">
            <div className="rounded-lg border border-border bg-muted/20 p-4">
              <p className="mb-3 text-muted-foreground">{displayEnum(t, "availableFunds", product.kind === "term_deposit" ? "termDeposit" : "lockedProduct")} · {productStateLabel(t, product.displayState)}</p>
              <dl className="grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-2">
                <dt className="text-muted-foreground">{t("availableFunds.principal")}</dt><dd className="text-right tabular-nums">{formatAmount(product.principal.amount, product.principal.currency)}</dd>
                <dt className="text-muted-foreground">{t("availableFunds.currentContractValue")}</dt><dd className="text-right font-medium tabular-nums">{moneyText(product.currentValue, t("availableFunds.unknownAmount"))}</dd>
                <dt className="text-muted-foreground">{t("availableFunds.startOn")}</dt><dd className="text-right">{product.startOn}</dd>
                {product.maturityOn && <><dt className="text-muted-foreground">{t("availableFunds.maturityOn")}</dt><dd className="text-right">{product.maturityOn}</dd></>}
                {product.annualRate != null && <><dt className="text-muted-foreground">{t("availableFunds.annualRatePercent")}</dt><dd className="text-right">{multiplyCanonical(product.annualRate, "100")}</dd></>}
                {product.maturityInterest && <><dt className="text-muted-foreground">{t("availableFunds.maturityInterest")}</dt><dd className="text-right tabular-nums">{moneyText(product.maturityInterest, t("availableFunds.unknownAmount"))}</dd></>}
                {product.interestPaidThroughOn && <><dt className="text-muted-foreground">{t("availableFunds.interestPaidThrough")}</dt><dd className="text-right">{product.interestPaidThroughOn}</dd></>}
              </dl>
              {product.note && <p className="mt-3 whitespace-pre-wrap break-words text-muted-foreground">{product.note}</p>}
            </div>
            <fieldset disabled={isSaving} className="flex flex-wrap gap-2">
              <Button type="button" variant="outline" onClick={() => { beginAction(null); setEditTerms(true); }}>{t("availableFunds.editTerms")}</Button>
              {(detail.data?.permittedActions ?? []).includes("settle") && <Button type="button" onClick={() => { beginAction("settle"); }}>{t("availableFunds.confirmReceipt")}</Button>}
              {(detail.data?.permittedActions ?? []).includes("receive_interest") && <Button type="button" variant="outline" onClick={() => { beginAction("interest"); }}>{t("availableFunds.receiveInterest")}</Button>}
              {product.kind === "locked_product" && product.state === "open" && <Button type="button" variant="outline" onClick={() => { beginAction("value"); }}>{t("availableFunds.updateValuation")}</Button>}
              {undoTarget && !operations.isError && <Button type="button" variant="ghost" disabled={operations.isFetching} onClick={() => { beginAction("undo"); }}>{t("availableFunds.groupedUndo")}</Button>}
            </fieldset>
            {action && <fieldset disabled={isSaving} className="flex min-w-0 flex-col gap-3 rounded-lg border border-border p-4">
              <h3 className="font-medium">{t(action === "settle" ? "availableFunds.confirmReceipt" : action === "interest" ? "availableFunds.receiveInterest" : action === "value" ? "availableFunds.updateValuation" : "availableFunds.groupedUndo")}</h3>
            {action === "settle" || action === "interest" ? (
              <div className="flex flex-col gap-2">
                {action === "settle" && <p className="text-xs text-muted-foreground">{t("availableFunds.receiptHelp")}</p>}
                <Label htmlFor="actual-amount">{action === "interest" ? t("availableFunds.interestReceived") : product.kind === "term_deposit" ? t("availableFunds.returnedPrincipal") : t("availableFunds.grossProceeds")}</Label>
                <Input id="actual-amount" inputMode="decimal" value={amount} onChange={(event) => setAmount(event.target.value)} />
                {action !== "interest" && product.kind === "term_deposit" && (
                  <>
                    <Label htmlFor="actual-interest">{t("availableFunds.interestReceived")}</Label>
                    <Input id="actual-interest" inputMode="decimal" value={interest} onChange={(event) => setInterest(event.target.value)} />
                  </>
                )}
                {action !== "interest" && (
                  <>
                    <Label htmlFor="actual-fee">{t("availableFunds.fee")}</Label>
                    <Input id="actual-fee" inputMode="decimal" value={fee} onChange={(event) => setFee(event.target.value)} />
                  </>
                )}
                {action === "interest" && (product.interestMode === "simple_act_365" || product.interestMode === "simple_act_360") && (
                  <>
                    <Label htmlFor="paid-through">{t("availableFunds.interestPaidThrough")}</Label>
                    <DatePicker clearable id="paid-through" min={product.interestPaidThroughOn ?? product.startOn} max={paidThroughMax} value={paidThrough} onChange={(next) => { setPaidThrough(next); setReviewed(null); }} />
                  </>
                )}
                {action === "interest" && product.interestMode === "manual_maturity_amount" && (
                  <>
                    <Label htmlFor="remaining-interest">{t("availableFunds.remainingInterest")}</Label>
                    <Input id="remaining-interest" inputMode="decimal" value={remainingInterest} onChange={(event) => setRemainingInterest(event.target.value)} />
                  </>
                )}
                {action !== "interest" && releaseIds.length > 0 && (
                  <p className="text-sm text-muted-foreground">{t("availableFunds.settleReleasesReservations")}</p>
                )}
                <details key={action} className="mt-2 rounded-lg border border-border p-3">
                  <summary className="cursor-pointer text-sm font-medium">{localTime.effectiveLocalDate || localTime.effectiveLocalTime ? `${t("availableFunds.actualTime")}: ${localTime.effectiveLocalDate ?? ""} ${localTime.effectiveLocalTime ?? ""}` : t("availableFunds.transactionTimeSettings")}</summary>
                  <div className="mt-3"><ProductTimeFields value={localTime} onChange={(next) => { setLocalTime(next); setReviewed(null); }} timezone={history.data?.timezone} /></div>
                </details>
              </div>
            ) : null}
            {action === "value" && (
              <>
                <p className="text-xs text-muted-foreground">{t("availableFunds.valuationHelp")}</p>
                <Label htmlFor="new-value">{t("availableFunds.currentContractValue")}</Label>
                <Input id="new-value" inputMode="decimal" placeholder={product.currentValue?.amount} value={amount} onChange={(event) => setAmount(event.target.value)} />
              </>
            )}
            {action === "undo" && undoTarget && <>
              <p>{t("availableFunds.undoTarget", { operation: displayEnum(t, "availableFunds", undoTarget.kind), date: formatTimestamp(undoTarget.effectiveAt, history.data?.timezone) })}</p>
              <p className="text-xs text-muted-foreground">{t("availableFunds.undoHelp")}</p>
            </>}
            {reviewed && <ProductPreview preview={reviewed} />}
            {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
            {(action === "settle" || action === "interest" || action === "undo") && command && (
              <div className="flex flex-wrap justify-end gap-2">
                <Button type="button" variant="ghost" onClick={() => beginAction(null)}>{t("availableFunds.cancel")}</Button>
                <Button type="button" variant="outline" disabled={preview.isPending || record.isPending} onClick={() => {
                  setError(undefined);
                  void preview.mutateAsync(command).then(setReviewed).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
                }}>{t("availableFunds.preview")}</Button>
                <Button type="button" disabled={!reviewed || record.isPending} onClick={() => {
                  if (!reviewed || record.isPending) {
                    return;
                  }
                  setError(undefined);
                  void record.mutateAsync({ command, mutationId: "", reviewedStateHash: reviewed.reviewedStateHash }).then(() => { setAction(null); setReviewed(null); }).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
                }}>{t("availableFunds.confirm")}</Button>
              </div>
            )}
            {action === "value" && (
              <div className="flex justify-end gap-2">
              <Button type="button" variant="ghost" onClick={() => beginAction(null)}>{t("availableFunds.cancel")}</Button>
              <Button type="button" disabled={valuation.isPending || !amount.trim()} onClick={() => {
                setError(undefined);
                void valuation.mutateAsync({ productId: product.id, amount, observedAt: "", mutationId: "" }).then(() => setAction(null)).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
              }}>{t("availableFunds.confirm")}</Button>
              </div>
            )}
            </fieldset>}
            {!action && error && <p role="alert" className="text-sm text-destructive">{error}</p>}
            {(detail.data?.reservations?.length ?? 0) > 0 && <h3 className="font-medium">{t("availableFunds.reservations")}</h3>}
            {(detail.data?.reservations ?? []).map((reservation) => (
              <div key={reservation.id} className="flex items-center justify-between">
                <span>{reservation.label}: {formatAmount(reservation.amount.amount, reservation.amount.currency)}</span>
                {!reservation.releasedAt && (
                  <Button type="button" variant="ghost" disabled={isSaving} onClick={() => { setError(undefined); void release.mutateAsync({ id: reservation.id, expectedRevision: reservation.revision }).catch((cause) => setError(displayError(cause, t("availableFunds.loadError")))); }}>{t("availableFunds.releaseReservation")}</Button>
                )}
              </div>
            ))}
            <h3 className="font-medium">{t("availableFunds.history")}</h3>
            {operations.isLoading && <p className="text-muted-foreground">{t("ui.state.loadingPage")}</p>}
            {operations.isError && <div role="alert" className="flex items-center justify-between gap-2 text-destructive"><span>{t("availableFunds.loadError")}</span><Button type="button" variant="outline" onClick={() => { void operations.refetch(); }}>{t("common.retry")}</Button></div>}
            {operations.isSuccess && operationHistory.length === 0 && <p className="text-muted-foreground">{t("availableFunds.noOperations")}</p>}
            {latestOperation?.kind === "value_observation" && <p className="text-xs text-muted-foreground">{t("availableFunds.undoAfterValuation")}</p>}
            <ul className="divide-y divide-border">
              {operationHistory.map((operation) => (
                <li key={operation.id} className="flex flex-col gap-1 py-3"><div className="flex items-center justify-between gap-2"><span>{displayEnum(t, "availableFunds", operation.kind)}</span>{reversedOperationIds.has(operation.id) && <span className="text-xs text-muted-foreground">{t("availableFunds.operationReversed")}</span>}</div><span className="text-xs text-muted-foreground">{formatTimestamp(operation.effectiveAt, history.data?.timezone)}</span></li>
              ))}
            </ul>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}

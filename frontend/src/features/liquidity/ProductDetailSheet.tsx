import { instrumentDisplayLabel, instrumentSecondaryName } from "@/lib/instrumentDisplay";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { displayEnum, displayError } from "@/lib/display";
import { formatAmount, formatAnnualRatePercent } from "@/lib/money";
import { formatCivilDate, formatTimestamp, localDateTimeInTimeZone } from "@/lib/time";
import { useHistoryOrigin } from "@/queries/history";
import {
  useAppendProductValuation,
  usePreviewProductOperation,
  useProduct,
  useProductOperations,
  useRecordProductOperation,
  useReleaseReservation
} from "@/queries/liquidity";
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { SettleProductCommand } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import type { ProductCommandRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import { moneyText, productStateLabel } from "./liquidityDisplay";
import { ProductEditSheet } from "./ProductEditSheet";
import { ProductPreview } from "./ProductPreview";
import { ProductTimeFields, type ProductLocalTime } from "./ProductTimeFields";
import { useReviewedCommand } from "./useReviewedCommand";


type ProductDetailSheetProps = { productId: string | null; receiptRequestId?: number; open: boolean; onOpenChange: (open: boolean) => void };
export function ProductDetailSheet(props: ProductDetailSheetProps) {
  return <ProductDetailSession key={`${props.productId}:${props.open}`} {...props} />;
}

function ProductDetailSession({
  productId,
  receiptRequestId,
  open,
  onOpenChange,
}: {
  productId: string | null;
  receiptRequestId?: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const detail = useProduct(productId ?? "", Boolean(productId) && open);
  const refetchDetail = detail.refetch;
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
  const [receiptMessage, setReceiptMessage] = useState<string>();
  const [receiptLoadError, setReceiptLoadError] = useState(false);
  const [receiptRetry, setReceiptRetry] = useState(0);
  const handledReceiptRequests = useRef(new Set<string>());
  const receiptIntentEpoch = useRef(0);
  const product = detail.data?.product;
  const today = history.data?.timezone ? localDateTimeInTimeZone(history.data.timezone)?.date : undefined;
  const paidThroughMax = [today, product?.maturityOn].filter((date): date is string => Boolean(date)).sort()[0];
  const operationHistory = operations.data?.pages.flatMap(page => page.operations ?? []) ?? [];
  const undo = operations.data?.pages[operations.data.pages.length - 1]?.undo;
  const undoFresh = operations.isSuccess && !operations.isFetching && !operations.isError;
  const undoTarget = undo?.operation;
  const [selectedUndoId, setSelectedUndoId] = useState<string | null>(null);
  const loadMoreRef = useRef<HTMLButtonElement>(null);
  const historyStatusRef = useRef<HTMLParagraphElement>(null);
  const loadingTrigger = useRef<HTMLButtonElement | null>(null);
  const loadMore = async (trigger: HTMLButtonElement) => {
    loadingTrigger.current = document.activeElement === trigger ? trigger : null;
    await operations.loadMore();
  };
  useLayoutEffect(() => {
    const trigger = loadingTrigger.current;
    if (!operations.isFetching && trigger) {
      if (document.activeElement === trigger || document.activeElement === document.body) {
        if (trigger.isConnected) trigger.focus();
        else if (loadMoreRef.current) loadMoreRef.current.focus();
        else historyStatusRef.current?.focus();
      }
      loadingTrigger.current = null;
    }
  }, [operations.isFetching, operations.hasNextPage]);
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
      if (!undoTarget || selectedUndoId !== undoTarget.id || !undo?.allowed || !undoFresh) {
        return null;
      }
      return { kind: "undo", undo: { operationId: undoTarget.id } };
    }
    return null;
  }, [action, amount, fee, interest, localTime, paidThrough, product, releaseIds, remainingInterest, undoTarget, selectedUndoId, undo?.allowed, undoFresh]);
  const { reviewed, setReviewed, clearReviewed } = useReviewedCommand(command, action === "undo"
    ? JSON.stringify([operations.sessionKey, operations.dataUpdatedAt, operations.data?.pages.length, undoFresh])
    : "");
  useEffect(() => {
    if (action === "undo") clearReviewed();
  }, [action, undo?.operation?.id, undo?.allowed, undoFresh, clearReviewed]);

  const initializeDraft = useCallback((next: typeof action, sourceProduct: typeof product) => {
    setAction(next);
    clearReviewed();
    setError(undefined);
    setAmount(next === "settle" && sourceProduct?.kind === "term_deposit" ? sourceProduct.principal.amount : "");
    setInterest("0");
    setFee("0");
    setLocalTime({});
    setPaidThrough("");
    setRemainingInterest("");
  }, [clearReviewed]);
  const beginAction = useCallback((next: typeof action) => {
    receiptIntentEpoch.current += 1;
    setReceiptLoadError(false);
    setReceiptMessage(undefined);
    setSelectedUndoId(next === "undo" ? undoTarget?.id ?? null : null);
    initializeDraft(next, product);
  }, [initializeDraft, product, undoTarget?.id]);
  useEffect(() => {
    const requestKey = `${receiptRequestId}:${receiptRetry}`;
    const handled = handledReceiptRequests.current;
    if (!open || !productId || receiptRequestId === undefined || handled.has(requestKey)) return;
    let cancelled = false;
    let finished = false;
    const epoch = receiptIntentEpoch.current;
    handled.add(requestKey);
    void refetchDetail().then(result => {
      if (cancelled || epoch !== receiptIntentEpoch.current) return;
      finished = true;
      if (result.isError) {
        setReceiptLoadError(true);
        setReceiptMessage(t("availableFunds.loadError"));
      } else if (result.data?.product?.displayState === "due_unconfirmed" && result.data.permittedActions?.includes("settle")) {
        setReceiptLoadError(false);
        initializeDraft("settle", result.data.product);
        setReceiptMessage(undefined);
      } else {
        setReceiptLoadError(false);
        setReceiptMessage(t("upcomingActions.stateChanged"));
      }
    });
    return () => { cancelled = true; if (!finished && epoch === receiptIntentEpoch.current) handled.delete(requestKey); };
  // The request ID is the one-shot navigation intent; refetch after each new intent.
  }, [receiptRequestId, receiptRetry, open, productId, refetchDetail, initializeDraft, t]);
  if (!productId) {
    return null;
  }
  if (editTerms) return <ProductEditSheet productId={productId} open={open} onOpenChange={(next) => { if (!next) setEditTerms(false); }} />;
  return (
    <Sheet open={open} onOpenChange={(next) => { if (!isSaving) { if (!next) receiptIntentEpoch.current += 1; onOpenChange(next); } }}>
      <SheetContent size="lg" onChangeCapture={() => setReviewed(null)}>
        <SheetHeader>
          <SheetTitle>{product ? instrumentDisplayLabel(product) : t("availableFunds.products")}</SheetTitle>
          {product && instrumentSecondaryName(product) && <p className="text-xs text-muted-foreground">{instrumentSecondaryName(product)}</p>}
        </SheetHeader>
        {detail.isLoading && <p>{t("ui.state.loadingPage")}</p>}
        {detail.isError && <p role="alert">{t("availableFunds.loadError")}</p>}
        {receiptMessage && <p role="status">{receiptMessage}</p>}
        {receiptLoadError && <Button type="button" variant="outline" onClick={() => setReceiptRetry(value => value + 1)}>{t("common.retryAction")}</Button>}
        {product && (
          <div className="flex flex-col gap-3 text-sm">
            <div className="rounded-lg border border-border bg-muted/20 p-4">
              <p className="mb-3 text-muted-foreground">{displayEnum(t, "availableFunds", product.kind === "term_deposit" ? "termDeposit" : "lockedProduct")} · {productStateLabel(t, product.displayState)}</p>
              <dl className="grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-2">
                <dt className="text-muted-foreground">{t("availableFunds.principal")}</dt><dd className="text-right tabular-nums">{formatAmount(product.principal.amount, product.principal.currency)}</dd>
                <dt className="text-muted-foreground">{t("availableFunds.currentContractValue")}</dt><dd className="text-right font-medium tabular-nums">{moneyText(product.currentValue, t("availableFunds.unknownAmount"))}</dd>
                <dt className="text-muted-foreground">{t("availableFunds.startOn")}</dt><dd className="text-right">{formatCivilDate(product.startOn)}</dd>
                {product.maturityOn && <><dt className="text-muted-foreground">{t("availableFunds.maturityOn")}</dt><dd className="text-right">{formatCivilDate(product.maturityOn)}</dd></>}
                {product.annualRate != null && <><dt className="text-muted-foreground">{t("availableFunds.annualRatePercent")}</dt><dd className="text-right">{formatAnnualRatePercent(product.annualRate)}</dd></>}
                {product.maturityInterest && <><dt className="text-muted-foreground">{t("availableFunds.maturityInterest")}</dt><dd className="text-right tabular-nums">{moneyText(product.maturityInterest, t("availableFunds.unknownAmount"))}</dd></>}
                {product.interestPaidThroughOn && <><dt className="text-muted-foreground">{t("availableFunds.interestPaidThrough")}</dt><dd className="text-right">{formatCivilDate(product.interestPaidThroughOn)}</dd></>}
              </dl>
              {product.note && <p className="mt-3 whitespace-pre-wrap break-words text-muted-foreground">{product.note}</p>}
            </div>
            <fieldset disabled={isSaving} className="flex flex-wrap gap-2">
              <Button type="button" variant="outline" onClick={() => { beginAction(null); setEditTerms(true); }}>{t("availableFunds.editTerms")}</Button>
              {(detail.data?.permittedActions ?? []).includes("settle") && <Button type="button" onClick={() => { beginAction("settle"); }}>{t("availableFunds.confirmReceipt")}</Button>}
              {(detail.data?.permittedActions ?? []).includes("receive_interest") && <Button type="button" variant="outline" onClick={() => { beginAction("interest"); }}>{t("availableFunds.receiveInterest")}</Button>}
              {product.kind === "locked_product" && product.state === "open" && <Button type="button" variant="outline" onClick={() => { beginAction("value"); }}>{t("availableFunds.updateValuation")}</Button>}
              {undoTarget && undo?.allowed && !operations.isError && <Button type="button" variant="ghost" disabled={!undoFresh} onClick={() => { beginAction("undo"); }}>{t("availableFunds.groupedUndo")}</Button>}
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
            {action === "undo" && !command && <>
              <p role="status">{t("availableFunds.historyChanged")}</p>
              <Button type="button" variant="ghost" onClick={() => beginAction(null)}>{t("availableFunds.cancel")}</Button>
            </>}
            {action === "undo" && command && undoTarget && <>
              <p>{t("availableFunds.undoTarget", { operation: displayEnum(t, "availableFunds", undoTarget.kind), date: formatTimestamp(undoTarget.effectiveAt, history.data?.timezone) })}</p>
              <p className="text-xs text-muted-foreground">{t("availableFunds.undoHelp")}</p>
            </>}
            {reviewed && <ProductPreview preview={reviewed} />}
            {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
            {(action === "settle" || action === "interest" || action === "undo") && command && (
              <div className="flex flex-wrap justify-end gap-2">
                <Button type="button" variant="ghost" onClick={() => beginAction(null)}>{t("availableFunds.cancel")}</Button>
                <Button type="button" variant="outline" disabled={preview.isPending || record.isPending || (action === "undo" && !undoFresh)} onClick={() => {
                  setError(undefined);
                  void preview.mutateAsync(command).then(setReviewed).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
                }}>{t("availableFunds.preview")}</Button>
                <Button type="button" disabled={!reviewed || record.isPending || (action === "undo" && !undoFresh)} onClick={() => {
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
            {operations.isError && <div role="alert" className="flex items-center justify-between gap-2 text-destructive"><span>{t("availableFunds.loadError")}</span><Button type="button" variant="outline" onClick={(event) => { if (operations.isFetchNextPageError) void loadMore(event.currentTarget); else operations.refresh(); }}>{t("common.retry")}</Button></div>}
            {operations.isSuccess && operationHistory.length === 0 && <p className="text-muted-foreground">{t("availableFunds.noOperations")}</p>}
            {undo?.reason && undo.reason !== "no_active_operation" && <p className="text-xs text-muted-foreground">{t(`availableFunds.undoReason_${undo.reason}`, { defaultValue: t("availableFunds.undoReason_unsafe_undo") })}</p>}
            <Button type="button" variant="ghost" disabled={operations.isFetching || isSaving} onClick={operations.refresh}>{t("availableFunds.refreshHistory")}</Button>
            <ul className="divide-y divide-border">
              {operationHistory.map((operation) => (
                <li key={operation.id} className="flex flex-col gap-1 py-3"><div className="flex items-center justify-between gap-2"><span>{displayEnum(t, "availableFunds", operation.kind)}</span>{operation.reversedByOperationId && <span className="text-xs text-muted-foreground">{t("availableFunds.operationReversed")}</span>}</div><span className="text-xs text-muted-foreground">{formatTimestamp(operation.effectiveAt, history.data?.timezone)}</span></li>
              ))}
            </ul>
            <p ref={historyStatusRef} tabIndex={-1} aria-live="polite" className="text-xs text-muted-foreground">
              {operations.isFetching ? t("availableFunds.loadingHistory") : operations.isSuccess && !operations.hasNextPage && operationHistory.length > 0 ? t("availableFunds.historyComplete") : ""}
            </p>
            {operations.hasNextPage && <Button ref={loadMoreRef} type="button" variant="outline" disabled={operations.isFetching || isSaving} onClick={(event) => { void loadMore(event.currentTarget); }}>{t("availableFunds.loadMoreHistory")}</Button>}
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}

import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { defaultProductPolicy, emptyTerms, policyForTerms, termsFromProduct } from "@/features/liquidity/productPolicy";
import { displayEnum, displayError } from "@/lib/display";
import { formatAmount } from "@/lib/money";
import { formatTimestamp } from "@/lib/time";
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
import type { ProductPolicyInput, ProductTermsInput, SettleProductCommand } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import type { ProductCommandRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import { moneyText, productStateLabel } from "./liquidityDisplay";
import { ProductEditSheet } from "./ProductEditSheet";
import { ProductPolicyFields, ProductTermsFields } from "./ProductFields";
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
  const [action, setAction] = useState<"settle" | "renew" | "interest" | "value" | "undo" | null>(null);
  const [amount, setAmount] = useState("");
  const [interest, setInterest] = useState("0");
  const [fee, setFee] = useState("0");
  const [newPrincipal, setNewPrincipal] = useState("");
  const [newTerms, setNewTerms] = useState<ProductTermsInput>(() => emptyTerms("term_deposit"));
  const [newPolicy, setNewPolicy] = useState<ProductPolicyInput>(() => defaultProductPolicy(null));
  const [newOpeningFee, setNewOpeningFee] = useState("0");
  const [editTerms, setEditTerms] = useState(false);
  const [paidThrough, setPaidThrough] = useState("");
  const [remainingInterest, setRemainingInterest] = useState("");
  const [error, setError] = useState<string>();
  const product = detail.data?.product;
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
    if (action === "renew") {
      const settle: SettleProductCommand = product.kind === "term_deposit"
        ? { productId: product.id, returnedPrincipal: amount, interest, fee, grossProceeds: null, effectiveAt: "", ...localTime, releaseReservationIds: releaseIds }
        : { productId: product.id, grossProceeds: amount, fee, effectiveAt: "", ...localTime, releaseReservationIds: releaseIds, returnedPrincipal: null, interest: null };
      return {
        kind: "renew",
        renew: {
          settle,
          principal: newPrincipal,
          openingFee: newOpeningFee,
          terms: newTerms,
          policy: policyForTerms(newTerms, newPolicy),
        },
      };
    }
    if (action === "interest") {
      return { kind: "receive_interest", receiveInterest: { productId: product.id, amount, effectiveAt: "", ...localTime, interestPaidThroughOn: paidThrough || null, remainingInterest: remainingInterest || null } };
    }
    if (action === "undo") {
      const history = operations.data?.operations ?? [];
      const reversed = new Set(history.map((operation) => operation.reversesOperationId).filter(Boolean));
      const latest = history.find((operation) => !operation.reversesOperationId && !reversed.has(operation.id));
      if (!latest) {
        return null;
      }
      return { kind: "undo", undo: { operationId: latest.id } };
    }
    return null;
  }, [action, amount, fee, interest, localTime, newOpeningFee, newPolicy, newPrincipal, newTerms, operations.data?.operations, paidThrough, product, releaseIds, remainingInterest]);
  const { reviewed, setReviewed } = useReviewedCommand(command);
  function beginAction(next: typeof action) {
    setAction(next);
    setReviewed(null);
    setError(undefined);
    setAmount("");
    setInterest("0");
    setFee("0");
    setLocalTime({});
    setPaidThrough("");
    setRemainingInterest("");
    setNewPrincipal("");
  }
  if (!productId) {
    return null;
  }
  if (editTerms) return <ProductEditSheet productId={productId} open={open} onOpenChange={(next) => { if (!next) setEditTerms(false); }} />;
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="overflow-y-auto" onChangeCapture={() => setReviewed(null)}>
        <SheetHeader>
          <SheetTitle>{product?.name ?? t("availableFunds.products")}</SheetTitle>
        </SheetHeader>
        {detail.isLoading && <p>{t("ui.state.loadingPage")}</p>}
        {detail.isError && <p role="alert">{t("availableFunds.loadError")}</p>}
        {product && (
          <div className="flex flex-col gap-3 text-sm">
            <p>{displayEnum(t, "availableFunds", product.kind === "term_deposit" ? "termDeposit" : "lockedProduct")}</p>
            <p>{t("availableFunds.principal")}: {formatAmount(product.principal.amount, product.principal.currency)}</p>
            <p>{t("availableFunds.currentContractValue")}: {moneyText(product.currentValue, t("availableFunds.unknownAmount"))}</p>
            <p>{t("availableFunds.nextAction")}: {productStateLabel(t, product.displayState)}</p>
            <p className="text-muted-foreground">{t("availableFunds.zeroWriteOff")}</p>
            <fieldset disabled={record.isPending || valuation.isPending} className="flex flex-wrap gap-2">
              <Button type="button" variant="outline" onClick={() => setEditTerms(true)}>{t("availableFunds.editTerms")}</Button>
              {(detail.data?.permittedActions ?? []).includes("settle") && <Button type="button" onClick={() => { beginAction("settle"); }}>{t("availableFunds.confirmReceipt")}</Button>}
              {(detail.data?.permittedActions ?? []).includes("renew") && <Button type="button" variant="outline" onClick={() => {
                setNewTerms({ ...termsFromProduct(product), startOn: "", maturityOn: null, interestPaidThroughOn: null });
                setNewPolicy(defaultProductPolicy(null)); setNewOpeningFee("0");
                beginAction("renew");
              }}>{t("availableFunds.confirmAndRenew")}</Button>}
              {(detail.data?.permittedActions ?? []).includes("receive_interest") && <Button type="button" variant="outline" onClick={() => { beginAction("interest"); }}>{t("availableFunds.receiveInterest")}</Button>}
              {product.kind === "locked_product" && product.state === "open" && <Button type="button" variant="outline" onClick={() => { beginAction("value"); }}>{t("availableFunds.updateValuation")}</Button>}
              <Button type="button" variant="ghost" onClick={() => { beginAction("undo"); }}>{t("availableFunds.groupedUndo")}</Button>
            </fieldset>
            {action === "settle" || action === "renew" || action === "interest" ? (
              <div className="flex flex-col gap-2">
                <ProductTimeFields value={localTime} onChange={(next) => { setLocalTime(next); setReviewed(null); }} timezone={history.data?.timezone} />
                <Label htmlFor="actual-amount">{action === "interest" ? t("availableFunds.interestReceived") : product.kind === "term_deposit" ? t("availableFunds.returnedPrincipal") : t("availableFunds.grossProceeds")}</Label>
                <Input id="actual-amount" value={amount} onChange={(event) => setAmount(event.target.value)} />
                {action !== "interest" && product.kind === "term_deposit" && (
                  <>
                    <Label htmlFor="actual-interest">{t("availableFunds.interestReceived")}</Label>
                    <Input id="actual-interest" value={interest} onChange={(event) => setInterest(event.target.value)} />
                  </>
                )}
                {action !== "interest" && (
                  <>
                    <Label htmlFor="actual-fee">{t("availableFunds.fee")}</Label>
                    <Input id="actual-fee" value={fee} onChange={(event) => setFee(event.target.value)} />
                  </>
                )}
                {action === "interest" && (product.interestMode === "simple_act_365" || product.interestMode === "simple_act_360") && (
                  <>
                    <Label htmlFor="paid-through">{t("availableFunds.interestPaidThrough")}</Label>
                    <DatePicker clearable allowFuture id="paid-through" value={paidThrough} onChange={(next) => { setPaidThrough(next); setReviewed(null); }} />
                  </>
                )}
                {action === "interest" && product.interestMode === "manual_maturity_amount" && (
                  <>
                    <Label htmlFor="remaining-interest">{t("availableFunds.remainingInterest")}</Label>
                    <Input id="remaining-interest" value={remainingInterest} onChange={(event) => setRemainingInterest(event.target.value)} />
                  </>
                )}
                {action !== "interest" && releaseIds.length > 0 && (
                  <p className="text-sm text-muted-foreground">{t("availableFunds.settleReleasesReservations")}</p>
                )}
                {action === "renew" && (
                  <>
                    <Label htmlFor="new-principal">{t("availableFunds.newPrincipal")}</Label>
                    <Input id="new-principal" value={newPrincipal} onChange={(event) => setNewPrincipal(event.target.value)} />
                    <Label htmlFor="new-opening-fee">{t("availableFunds.openingFee")}</Label>
                    <Input id="new-opening-fee" value={newOpeningFee} onChange={(event) => setNewOpeningFee(event.target.value)} />
                    <ProductTermsFields value={newTerms} onChange={(next) => { setNewTerms(next); setReviewed(null); }} />
                    <ProductPolicyFields value={policyForTerms(newTerms, newPolicy)} onChange={(next) => { setNewPolicy(next); setReviewed(null); }} termDeposit={newTerms.kind === "term_deposit"} />
                  </>
                )}
              </div>
            ) : null}
            {action === "value" && (
              <>
                <Label htmlFor="new-value">{t("availableFunds.currentContractValue")}</Label>
                <Input id="new-value" value={amount} onChange={(event) => setAmount(event.target.value)} />
              </>
            )}
            {reviewed && <ProductPreview preview={reviewed} />}
            {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
            {(action === "settle" || action === "renew" || action === "interest" || action === "undo") && command && (
              <div className="flex gap-2">
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
              <Button type="button" disabled={valuation.isPending} onClick={() => {
                setError(undefined);
                void valuation.mutateAsync({ productId: product.id, amount, observedAt: "", mutationId: "" }).then(() => setAction(null)).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
              }}>{t("availableFunds.confirm")}</Button>
            )}
            <h3 className="font-medium">{t("availableFunds.reservations")}</h3>
            {(detail.data?.reservations ?? []).map((reservation) => (
              <div key={reservation.id} className="flex items-center justify-between">
                <span>{reservation.label}: {formatAmount(reservation.amount.amount, reservation.amount.currency)}</span>
                {!reservation.releasedAt && (
                  <Button type="button" variant="ghost" disabled={release.isPending} onClick={() => { setError(undefined); void release.mutateAsync({ id: reservation.id, expectedRevision: reservation.revision }).catch((cause) => setError(displayError(cause, t("availableFunds.loadError")))); }}>{t("availableFunds.releaseReservation")}</Button>
                )}
              </div>
            ))}
            <h3 className="font-medium">{t("availableFunds.history")}</h3>
            <ul>
              {(operations.data?.operations ?? []).map((operation) => (
                <li key={operation.id}>{displayEnum(t, "availableFunds", operation.kind)} · {formatTimestamp(operation.effectiveAt, history.data?.timezone)}</li>
              ))}
            </ul>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}

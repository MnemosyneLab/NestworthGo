import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Sheet, SheetContent, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { formatAmount } from "@/lib/money";
import { displayEnum, displayError } from "@/lib/display";
import {
  useAppendProductValuation,
  usePreviewProductOperation,
  useProduct,
  useProductOperations,
  useRecordProductOperation,
  useReleaseReservation,
  useResetPolicy,
  useSavePolicy,
  useSaveReservation,
} from "@/queries/liquidity";
import { defaultProductPolicy, emptyTerms } from "@/features/liquidity/productPolicy";
import type { LiquiditySourceDTO, ProductCommandRequest, ProductOperationPreviewDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import type { ProductPolicyInput, ProductTermsInput, SettleProductCommand } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import type { MoneyView } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export function moneyText(value: MoneyView | null | undefined, unknown: string): string {
  if (!value) {
    return unknown;
  }
  return formatAmount(value.amount, value.currency);
}

export function PolicySheet({
  source,
  open,
  onOpenChange,
}: {
  source: LiquiditySourceDTO | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const save = useSavePolicy();
  const reset = useResetPolicy();
  const [accessKind, setAccessKind] = useState(source?.policy?.accessKind ?? "on_request");
  const [unlockOn, setUnlockOn] = useState(source?.policy?.unlockOn ?? "");
  const [settlementDays, setSettlementDays] = useState(String(source?.policy?.settlementDays ?? 0));
  const [dayBasis, setDayBasis] = useState(source?.policy?.dayBasis ?? "calendar");
  const [normalExitFee, setNormalExitFee] = useState(source?.policy?.normalExitFee?.amount ?? "0");
  const [earlyKind, setEarlyKind] = useState(source?.policy?.earlyKind ?? "not_allowed");
  const [error, setError] = useState<string>();
  if (!source) {
    return null;
  }
  const managed = Boolean(source.productId);
  const policy: ProductPolicyInput = {
    accessKind,
    unlockOn: unlockOn || null,
    settlementDays: Number(settlementDays),
    dayBasis,
    receiptOnOverride: source.policy?.receiptOnOverride ?? null,
    normalExitFee,
    earlyKind,
    earlySettlementDays: source.policy?.earlySettlementDays ?? null,
    earlyDayBasis: source.policy?.earlyDayBasis ?? null,
    earlyFee: source.policy?.earlyFee?.amount ?? null,
    earlyAmountMode: source.policy?.earlyAmountMode ?? null,
    earlyGrossAmount: source.policy?.earlyGrossAmount?.amount ?? null,
    note: source.policy?.note ?? null,
  };
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="overflow-y-auto">
        <SheetHeader>
          <SheetTitle>{t("availableFunds.policy")}</SheetTitle>
        </SheetHeader>
        <p className="text-sm text-muted-foreground">{source.displayName}</p>
        <p className="text-sm">{t(`availableFunds.policyOrigin${source.policyOrigin === "assumed" ? "Assumed" : source.policyOrigin === "explicit" ? "Explicit" : "Contract"}`)}</p>
        <Label htmlFor="access-kind">{t("availableFunds.accessKind")}</Label>
        <NativeSelect id="access-kind" value={accessKind} onChange={(event) => setAccessKind(event.target.value)}>
          <option value="on_request">{t("availableFunds.accessOnRequest")}</option>
          <option value="on_date">{t("availableFunds.accessOnDate")}</option>
          <option value="unknown">{t("availableFunds.accessUnknown")}</option>
          <option value="excluded">{t("availableFunds.accessExcluded")}</option>
        </NativeSelect>
        {accessKind === "on_date" && (
          <>
            <Label htmlFor="unlock-on">{t("availableFunds.unlockOn")}</Label>
            <Input id="unlock-on" type="date" value={unlockOn} onChange={(event) => setUnlockOn(event.target.value)} />
          </>
        )}
        <Label htmlFor="settlement-days">{t("availableFunds.settlementDays")}</Label>
        <Input id="settlement-days" inputMode="numeric" value={settlementDays} onChange={(event) => setSettlementDays(event.target.value)} />
        <Label htmlFor="day-basis">{t("availableFunds.dayBasis")}</Label>
        <NativeSelect id="day-basis" value={dayBasis} onChange={(event) => setDayBasis(event.target.value)}>
          <option value="calendar">{t("availableFunds.calendar")}</option>
          <option value="weekdays">{t("availableFunds.weekdays")}</option>
        </NativeSelect>
        {dayBasis === "weekdays" && <p className="text-sm text-muted-foreground">{t("availableFunds.weekdaysNote")}</p>}
        <Label htmlFor="exit-fee">{t("availableFunds.normalExitFee")}</Label>
        <Input id="exit-fee" value={normalExitFee} onChange={(event) => setNormalExitFee(event.target.value)} />
        <Label htmlFor="early-kind">{t("availableFunds.earlyKind")}</Label>
        <NativeSelect id="early-kind" value={earlyKind} onChange={(event) => setEarlyKind(event.target.value)}>
          <option value="not_allowed">{t("availableFunds.earlyNotAllowed")}</option>
          <option value="allowed">{t("availableFunds.earlyAllowed")}</option>
          <option value="unknown">{t("availableFunds.earlyUnknown")}</option>
        </NativeSelect>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <SheetFooter>
          {!managed && source.policy && source.policy.revision > 0 && (
            <Button type="button" variant="outline" onClick={() => {
              setError(undefined);
              void reset.mutateAsync({ sourceRef: source.sourceRef, expectedRevision: source.policy?.revision ?? 0 }).then(() => onOpenChange(false)).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
            }}>{t("availableFunds.resetPolicy")}</Button>
          )}
          <Button type="button" onClick={() => {
            setError(undefined);
            void save.mutateAsync({ sourceRef: source.sourceRef, expectedRevision: source.policy?.revision ?? 0, policy }).then(() => onOpenChange(false)).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
          }}>{t("availableFunds.savePolicy")}</Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

export function ReservationSheet({
  source,
  open,
  onOpenChange,
}: {
  source: LiquiditySourceDTO | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const save = useSaveReservation();
  const [label, setLabel] = useState("");
  const [amount, setAmount] = useState("");
  const [error, setError] = useState<string>();
  if (!source) {
    return null;
  }
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{t("availableFunds.addReservation")}</SheetTitle>
        </SheetHeader>
        <Label htmlFor="reserve-label">{t("availableFunds.reservationLabel")}</Label>
        <Input id="reserve-label" value={label} onChange={(event) => setLabel(event.target.value)} />
        <Label htmlFor="reserve-amount">{t("availableFunds.reservationAmount")}</Label>
        <Input id="reserve-amount" value={amount} onChange={(event) => setAmount(event.target.value)} />
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <SheetFooter>
          <Button type="button" onClick={() => {
            setError(undefined);
            void save.mutateAsync({ sourceRef: source.sourceRef, expectedRevision: 0, label, amount }).then(() => onOpenChange(false)).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
          }}>{t("availableFunds.confirm")}</Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

export function ProductFormSheet({
  accountId,
  currency,
  cashAmount,
  open,
  onOpenChange,
}: {
  accountId: string;
  currency: string;
  cashAmount?: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const preview = usePreviewProductOperation();
  const record = useRecordProductOperation();
  const [mode, setMode] = useState<"open" | "record_existing">("open");
  const [kind, setKind] = useState<"term_deposit" | "locked_product">("term_deposit");
  const [name, setName] = useState("");
  const [principal, setPrincipal] = useState("");
  const [currentValue, setCurrentValue] = useState("");
  const [totalCostBasis, setTotalCostBasis] = useState("");
  const [startOn, setStartOn] = useState("");
  const [maturityOn, setMaturityOn] = useState("");
  const [interestMode, setInterestMode] = useState("none");
  const [annualRatePercent, setAnnualRatePercent] = useState("");
  const [maturityInterest, setMaturityInterest] = useState("");
  const [openingFee, setOpeningFee] = useState("0");
  const [cashExcludes, setCashExcludes] = useState(false);
  const [reviewed, setReviewed] = useState<ProductOperationPreviewDTO | null>(null);
  const [error, setError] = useState<string>();
  const terms: ProductTermsInput = {
    ...emptyTerms(kind),
    name,
    startOn,
    maturityOn: maturityOn || null,
    interestMode,
    annualRatePercent: annualRatePercent || null,
    maturityInterest: maturityInterest || null,
  };
  const policy = defaultProductPolicy(maturityOn || null);
  const command: ProductCommandRequest = mode === "open"
    ? { kind: "open", open: { accountId, currency, principal, openingFee, effectiveAt: "", terms, policy } }
    : { kind: "record_existing", recordExisting: { accountId, currency, principal, totalCostBasis, currentValue: currentValue || principal, cashExcludesProduct: cashExcludes, effectiveAt: "", terms, policy } };
  return (
    <Sheet open={open} onOpenChange={(next) => { if (!next) setReviewed(null); onOpenChange(next); }}>
      <SheetContent className="overflow-y-auto">
        <SheetHeader>
          <SheetTitle>{t("availableFunds.addProduct")}</SheetTitle>
        </SheetHeader>
        <p className="text-sm text-muted-foreground">{t("availableFunds.unsupportedPartial")}</p>
        <p className="text-sm text-muted-foreground">{t("availableFunds.noFxConversion")}</p>
        {cashAmount && <p className="text-sm">{t("availableFunds.cashAvailable", { currency, amount: formatAmount(cashAmount, currency) })}</p>}
        <Label htmlFor="product-mode">{t("availableFunds.addProduct")}</Label>
        <NativeSelect id="product-mode" value={mode} onChange={(event) => setMode(event.target.value as "open" | "record_existing")}>
          <option value="open">{t("availableFunds.buyOpen")}</option>
          <option value="record_existing">{t("availableFunds.recordExisting")}</option>
        </NativeSelect>
        <Label htmlFor="product-kind">{t("availableFunds.asset")}</Label>
        <NativeSelect id="product-kind" value={kind} onChange={(event) => setKind(event.target.value as "term_deposit" | "locked_product")}>
          <option value="term_deposit">{t("availableFunds.termDeposit")}</option>
          <option value="locked_product">{t("availableFunds.lockedProduct")}</option>
        </NativeSelect>
        <Label htmlFor="product-name">{t("availableFunds.name")}</Label>
        <Input id="product-name" value={name} onChange={(event) => setName(event.target.value)} />
        <Label htmlFor="product-principal">{t("availableFunds.principal")}</Label>
        <Input id="product-principal" value={principal} onChange={(event) => setPrincipal(event.target.value)} />
        {mode === "record_existing" && (
          <>
            <p className="text-sm text-muted-foreground">{t("availableFunds.recordExistingHelp")}</p>
            <Label htmlFor="product-value">{t("availableFunds.currentContractValue")}</Label>
            <Input id="product-value" value={currentValue} onChange={(event) => setCurrentValue(event.target.value)} />
            <Label htmlFor="product-cost">{t("availableFunds.cost")}</Label>
            <Input id="product-cost" value={totalCostBasis} onChange={(event) => setTotalCostBasis(event.target.value)} />
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={cashExcludes} onChange={(event) => setCashExcludes(event.target.checked)} />
              {t("availableFunds.cashExcludes")}
            </label>
          </>
        )}
        {mode === "open" && (
          <>
            <Label htmlFor="opening-fee">{t("availableFunds.openingFee")}</Label>
            <Input id="opening-fee" value={openingFee} onChange={(event) => setOpeningFee(event.target.value)} />
          </>
        )}
        <Label htmlFor="start-on">{t("availableFunds.startOn")}</Label>
        <Input id="start-on" type="date" value={startOn} onChange={(event) => setStartOn(event.target.value)} />
        <Label htmlFor="maturity-on">{t("availableFunds.maturityOn")}</Label>
        <Input id="maturity-on" type="date" value={maturityOn} onChange={(event) => setMaturityOn(event.target.value)} />
        {kind === "term_deposit" && (
          <>
            <Label htmlFor="interest-mode">{t("availableFunds.interestMode")}</Label>
            <NativeSelect id="interest-mode" value={interestMode} onChange={(event) => setInterestMode(event.target.value)}>
              <option value="none">{t("availableFunds.interestNone")}</option>
              <option value="simple_act_365">{t("availableFunds.interestSimple365")}</option>
              <option value="simple_act_360">{t("availableFunds.interestSimple360")}</option>
              <option value="manual_maturity_amount">{t("availableFunds.interestManual")}</option>
            </NativeSelect>
            {(interestMode === "simple_act_365" || interestMode === "simple_act_360") && (
              <>
                <Label htmlFor="annual-rate">{t("availableFunds.annualRatePercent")}</Label>
                <Input id="annual-rate" value={annualRatePercent} onChange={(event) => setAnnualRatePercent(event.target.value)} />
              </>
            )}
            {interestMode === "manual_maturity_amount" && (
              <>
                <Label htmlFor="maturity-interest">{t("availableFunds.maturityInterest")}</Label>
                <Input id="maturity-interest" value={maturityInterest} onChange={(event) => setMaturityInterest(event.target.value)} />
              </>
            )}
          </>
        )}
        {reviewed && (
          <div className="rounded-md border border-border p-3 text-sm" data-testid="product-preview">
            <p>{t("availableFunds.previewCashBefore")}: {reviewed.cashBefore?.map((item) => formatAmount(item.amount, item.currency)).join(" · ") || t("availableFunds.unknownAmount")}</p>
            <p>{t("availableFunds.previewCashAfter")}: {reviewed.cashAfter?.map((item) => formatAmount(item.amount, item.currency)).join(" · ") || t("availableFunds.unknownAmount")}</p>
            <p>{t("availableFunds.previewProductAfter")}: {moneyText(reviewed.productAfter, t("availableFunds.unknownAmount"))}</p>
            {(reviewed.warnings ?? []).map((warning) => <p key={warning}>{warning}</p>)}
          </div>
        )}
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <SheetFooter>
          <Button type="button" variant="outline" onClick={() => {
            setError(undefined);
            void preview.mutateAsync(command).then(setReviewed).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
          }}>{t("availableFunds.preview")}</Button>
          <Button type="button" disabled={!reviewed || record.isPending} onClick={() => {
            if (!reviewed || record.isPending) {
              return;
            }
            setError(undefined);
            void record.mutateAsync({ command, mutationId: "", reviewedStateHash: reviewed.reviewedStateHash }).then(() => onOpenChange(false)).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
          }}>{t("availableFunds.confirm")}</Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

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
  const [newName, setNewName] = useState("");
  const [newMaturity, setNewMaturity] = useState("");
  const [paidThrough, setPaidThrough] = useState("");
  const [remainingInterest, setRemainingInterest] = useState("");
  const [reviewed, setReviewed] = useState<ProductOperationPreviewDTO | null>(null);
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
        ? { productId: product.id, returnedPrincipal: amount, interest, fee, grossProceeds: null, effectiveAt: "", releaseReservationIds: releaseIds }
        : { productId: product.id, grossProceeds: amount, fee, effectiveAt: "", releaseReservationIds: releaseIds, returnedPrincipal: null, interest: null };
      return { kind: "settle", settle };
    }
    if (action === "renew") {
      const terms = { ...emptyTerms(product.kind as "term_deposit" | "locked_product"), name: newName || product.name, startOn: product.startOn, maturityOn: newMaturity || null, interestMode: product.interestMode };
      const settle: SettleProductCommand = product.kind === "term_deposit"
        ? { productId: product.id, returnedPrincipal: amount, interest, fee, grossProceeds: null, effectiveAt: "", releaseReservationIds: releaseIds }
        : { productId: product.id, grossProceeds: amount, fee, effectiveAt: "", releaseReservationIds: releaseIds, returnedPrincipal: null, interest: null };
      return {
        kind: "renew",
        renew: {
          settle,
          principal: newPrincipal,
          openingFee: "0",
          terms,
          policy: defaultProductPolicy(newMaturity || null),
        },
      };
    }
    if (action === "interest") {
      return { kind: "receive_interest", receiveInterest: { productId: product.id, amount, effectiveAt: "", interestPaidThroughOn: paidThrough || null, remainingInterest: remainingInterest || null } };
    }
    if (action === "undo") {
      const latest = operations.data?.operations?.[0];
      if (!latest) {
        return null;
      }
      return { kind: "undo", undo: { operationId: latest.id } };
    }
    return null;
  }, [action, amount, fee, interest, newMaturity, newName, newPrincipal, operations.data?.operations, paidThrough, product, releaseIds, remainingInterest]);
  if (!productId) {
    return null;
  }
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="overflow-y-auto">
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
            <p>{t("availableFunds.nextAction")}: {displayEnum(t, "availableFunds", product.displayState === "due_unconfirmed" ? "stateDue" : product.displayState === "locked" ? "stateLocked" : product.displayState === "redeemable" ? "stateRedeemable" : product.displayState === "settled" ? "stateSettled" : "stateCancelled")}</p>
            <p className="text-muted-foreground">{t("availableFunds.zeroWriteOff")}</p>
            <div className="flex flex-wrap gap-2">
              {(detail.data?.permittedActions ?? []).includes("settle") && <Button type="button" onClick={() => { setAction("settle"); setReviewed(null); }}>{t("availableFunds.confirmReceipt")}</Button>}
              {(detail.data?.permittedActions ?? []).includes("renew") && <Button type="button" variant="outline" onClick={() => { setAction("renew"); setReviewed(null); }}>{t("availableFunds.confirmAndRenew")}</Button>}
              {(detail.data?.permittedActions ?? []).includes("receive_interest") && <Button type="button" variant="outline" onClick={() => { setAction("interest"); setReviewed(null); }}>{t("availableFunds.receiveInterest")}</Button>}
              {product.kind === "locked_product" && product.state === "open" && <Button type="button" variant="outline" onClick={() => { setAction("value"); setReviewed(null); }}>{t("availableFunds.updateValuation")}</Button>}
              <Button type="button" variant="ghost" onClick={() => { setAction("undo"); setReviewed(null); }}>{t("availableFunds.groupedUndo")}</Button>
            </div>
            {action === "settle" || action === "renew" || action === "interest" ? (
              <div className="flex flex-col gap-2">
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
                    <Input id="paid-through" type="date" value={paidThrough} onChange={(event) => setPaidThrough(event.target.value)} />
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
                    <Label htmlFor="new-name">{t("availableFunds.newName")}</Label>
                    <Input id="new-name" value={newName} onChange={(event) => setNewName(event.target.value)} />
                    <Label htmlFor="new-maturity">{t("availableFunds.newMaturity")}</Label>
                    <Input id="new-maturity" type="date" value={newMaturity} onChange={(event) => setNewMaturity(event.target.value)} />
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
            {reviewed && (
              <div className="rounded-md border border-border p-3">
                <p>{t("availableFunds.previewCashAfter")}: {reviewed.cashAfter?.map((item) => formatAmount(item.amount, item.currency)).join(" · ") || t("availableFunds.unknownAmount")}</p>
              </div>
            )}
            {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
            {(action === "settle" || action === "renew" || action === "interest" || action === "undo") && command && (
              <div className="flex gap-2">
                <Button type="button" variant="outline" onClick={() => {
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
                  <Button type="button" variant="ghost" onClick={() => void release.mutateAsync({ id: reservation.id, expectedRevision: reservation.revision })}>{t("availableFunds.releaseReservation")}</Button>
                )}
              </div>
            ))}
            <h3 className="font-medium">{t("availableFunds.history")}</h3>
            <ul>
              {(operations.data?.operations ?? []).map((operation) => (
                <li key={operation.id}>{displayEnum(t, "availableFunds", operation.kind)} · {operation.effectiveAt}</li>
              ))}
            </ul>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}

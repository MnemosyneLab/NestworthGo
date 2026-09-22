import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Sheet, SheetContent, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { formatAmount } from "@/lib/money";
import { useHistoryOrigin } from "@/queries/history";
import { useAccountValuations } from "@/queries/accounts";
import { useSupportedCurrencies } from "@/queries/settings";
import { ProductTimeFields, type ProductLocalTime } from "./ProductTimeFields";
import { ProductPreview } from "./ProductPreview";
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
import { defaultProductPolicy, emptyTerms, policyForTerms, policyFromDTO, termsFromProduct } from "@/features/liquidity/productPolicy";
import { ProductTermsFields, ProductPolicyFields } from "./ProductFields";
import { ProductEditSheet } from "./ProductEditSheet";
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
  if (!source) return null;
  if (source.productId) return <ProductEditSheet productId={source.productId} open={open} onOpenChange={onOpenChange} />;
  return <OrdinaryPolicySheet source={source} open={open} onOpenChange={onOpenChange} />;
}

function OrdinaryPolicySheet({ source, open, onOpenChange }: {
  source: LiquiditySourceDTO; open: boolean; onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const save = useSavePolicy();
  const reset = useResetPolicy();
  const [policy, setPolicy] = useState<ProductPolicyInput>(() => source.policy ? policyFromDTO(source.policy) : { ...defaultProductPolicy(null), accessKind: "unknown", settlementDays: null, normalExitFee: null });
  const [error, setError] = useState<string>();
  return <Sheet open={open} onOpenChange={onOpenChange}>
    <SheetContent className="overflow-y-auto">
      <SheetHeader><SheetTitle>{t("availableFunds.policy")}</SheetTitle></SheetHeader>
      <p className="text-sm text-muted-foreground">{source.displayName}</p>
      <ProductPolicyFields value={policy} onChange={setPolicy} ordinary />
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <SheetFooter>
        {source.policy && source.policy.revision > 0 && <Button type="button" variant="outline" disabled={save.isPending || reset.isPending} onClick={() => {
          setError(undefined);
          void reset.mutateAsync({ sourceRef: source.sourceRef, expectedRevision: source.policy?.revision ?? 0 }).then(() => onOpenChange(false)).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
        }}>{t("availableFunds.resetPolicy")}</Button>}
        <Button type="button" disabled={save.isPending || reset.isPending} onClick={() => {
          setError(undefined);
          void save.mutateAsync({ sourceRef: source.sourceRef, expectedRevision: source.policy?.revision ?? 0, policy }).then(() => onOpenChange(false)).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
        }}>{t("availableFunds.savePolicy")}</Button>
      </SheetFooter>
    </SheetContent>
  </Sheet>;
}

export function ReservationSheet({
  source, reservation,
  open,
  onOpenChange,
}: {
  source: LiquiditySourceDTO | null;
  reservation?: import("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models").ReservationDTO;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const save = useSaveReservation();
  const [label, setLabel] = useState(reservation?.label ?? "");
  const [amount, setAmount] = useState(reservation?.amount.amount ?? "");
  const [error, setError] = useState<string>();
  if (!source && !reservation) {
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
            void save.mutateAsync({ id: reservation?.id, sourceRef: reservation?.sourceRef ?? source!.sourceRef, expectedRevision: reservation?.revision ?? 0, label, amount }).then(() => onOpenChange(false)).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
          }}>{t("availableFunds.confirm")}</Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

export function ProductFormSheet({
  accountId,
  currency: defaultCurrency,
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
  const [currency, setCurrency] = useState(defaultCurrency);
  const currencies = useSupportedCurrencies();
  const valuations = useAccountValuations();
  const accountValue = valuations.data?.find((item) => item.account.id === accountId);
  const cash = accountValue?.components?.filter((item) => !item.holdingId) ?? [];
  const selectedCash = accountValue ? (cash.find((item) => item.nativeCurrency === currency)?.nativeAmount ?? "0") : (currency === defaultCurrency ? cashAmount : undefined);
  const history = useHistoryOrigin();
  const [localTime, setLocalTime] = useState<ProductLocalTime>({});
  const [mode, setMode] = useState<"open" | "record_existing">("open");
  const [terms, setTerms] = useState<ProductTermsInput>(() => emptyTerms("term_deposit"));
  const [rules, setRules] = useState<ProductPolicyInput>(() => defaultProductPolicy(null));
  const [principal, setPrincipal] = useState("");
  const [currentValue, setCurrentValue] = useState("");
  const [totalCostBasis, setTotalCostBasis] = useState("");
  const [openingFee, setOpeningFee] = useState("0");
  const [cashExcludes, setCashExcludes] = useState(false);
  const [reviewed, setReviewed] = useState<ProductOperationPreviewDTO | null>(null);
  const [error, setError] = useState<string>();
  const policy = policyForTerms(terms, rules);
  const command: ProductCommandRequest = mode === "open"
    ? { kind: "open", open: { accountId, currency, principal, openingFee, effectiveAt: "", ...localTime, terms, policy } }
    : { kind: "record_existing", recordExisting: { accountId, currency, principal, totalCostBasis, currentValue: currentValue || principal, cashExcludesProduct: cashExcludes, effectiveAt: "", terms, policy } };
  return (
    <Sheet open={open} onOpenChange={(next) => { if (!next) setReviewed(null); onOpenChange(next); }}>
      <SheetContent className="overflow-y-auto" onChangeCapture={() => setReviewed(null)}>
        <SheetHeader>
          <SheetTitle>{t("availableFunds.addProduct")}</SheetTitle>
        </SheetHeader>
        <p className="text-sm text-muted-foreground">{t("availableFunds.unsupportedPartial")}</p>
        <p className="text-sm text-muted-foreground">{t("availableFunds.noFxConversion")}</p>
        <Label htmlFor="product-currency">{t("availableFunds.contractCurrency")}</Label>
        <NativeSelect id="product-currency" value={currency} onChange={(e) => setCurrency(e.target.value)}>
          {[...new Set([defaultCurrency, ...(currencies.data ?? []), ...cash.map((item) => item.nativeCurrency)])].sort().map((code) => <option key={code} value={code}>{code}</option>)}
        </NativeSelect>
        <p className="text-sm">{t("availableFunds.cashAvailable", { currency, amount: selectedCash === undefined ? t("availableFunds.unknownAmount") : formatAmount(selectedCash, currency) })}</p>
        {mode === "open" && <ProductTimeFields value={localTime} onChange={setLocalTime} timezone={history.data?.timezone} />}
        <Label htmlFor="product-mode">{t("availableFunds.addProduct")}</Label>
        <NativeSelect id="product-mode" value={mode} onChange={(event) => setMode(event.target.value as "open" | "record_existing")}>
          <option value="open">{t("availableFunds.buyOpen")}</option>
          <option value="record_existing">{t("availableFunds.recordExisting")}</option>
        </NativeSelect>
        <Label htmlFor="product-kind">{t("availableFunds.asset")}</Label>
        <NativeSelect id="product-kind" value={terms.kind} onChange={(event) => { setTerms({ ...emptyTerms(event.target.value as "term_deposit" | "locked_product"), name: terms.name, startOn: terms.startOn }); setRules(defaultProductPolicy(null)); }}>
          <option value="term_deposit">{t("availableFunds.termDeposit")}</option>
          <option value="locked_product">{t("availableFunds.lockedProduct")}</option>
        </NativeSelect>
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
        <ProductTermsFields value={terms} onChange={setTerms} />
        <ProductPolicyFields value={policy} onChange={setRules} termDeposit={terms.kind === "term_deposit"} />
        {reviewed && <ProductPreview preview={reviewed} />}
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
            <p>{t("availableFunds.nextAction")}: {displayEnum(t, "availableFunds", product.displayState === "due_unconfirmed" ? "stateDue" : product.displayState === "locked" ? "stateLocked" : product.displayState === "redeemable" ? "stateRedeemable" : product.displayState === "settled" ? "stateSettled" : "stateCancelled")}</p>
            <p className="text-muted-foreground">{t("availableFunds.zeroWriteOff")}</p>
            <div className="flex flex-wrap gap-2">
              <Button type="button" variant="outline" onClick={() => setEditTerms(true)}>{t("availableFunds.editTerms")}</Button>
              {(detail.data?.permittedActions ?? []).includes("settle") && <Button type="button" onClick={() => { setAction("settle"); setReviewed(null); }}>{t("availableFunds.confirmReceipt")}</Button>}
              {(detail.data?.permittedActions ?? []).includes("renew") && <Button type="button" variant="outline" onClick={() => {
                setNewTerms({ ...termsFromProduct(product), startOn: "", maturityOn: null, interestPaidThroughOn: null });
                setNewPolicy(defaultProductPolicy(null)); setNewOpeningFee("0");
                setAction("renew"); setReviewed(null);
              }}>{t("availableFunds.confirmAndRenew")}</Button>}
              {(detail.data?.permittedActions ?? []).includes("receive_interest") && <Button type="button" variant="outline" onClick={() => { setAction("interest"); setReviewed(null); }}>{t("availableFunds.receiveInterest")}</Button>}
              {product.kind === "locked_product" && product.state === "open" && <Button type="button" variant="outline" onClick={() => { setAction("value"); setReviewed(null); }}>{t("availableFunds.updateValuation")}</Button>}
              <Button type="button" variant="ghost" onClick={() => { setAction("undo"); setReviewed(null); }}>{t("availableFunds.groupedUndo")}</Button>
            </div>
            {action === "settle" || action === "renew" || action === "interest" ? (
              <div className="flex flex-col gap-2">
                <ProductTimeFields value={localTime} onChange={setLocalTime} timezone={history.data?.timezone} />
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
                    <Label htmlFor="new-opening-fee">{t("availableFunds.openingFee")}</Label>
                    <Input id="new-opening-fee" value={newOpeningFee} onChange={(event) => setNewOpeningFee(event.target.value)} />
                    <ProductTermsFields value={newTerms} onChange={setNewTerms} />
                    <ProductPolicyFields value={policyForTerms(newTerms, newPolicy)} onChange={setNewPolicy} termDeposit={newTerms.kind === "term_deposit"} />
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

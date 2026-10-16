import {
  knownReconciliationConflict,
  completedReconciliation,
} from "./reconciliationErrors";
import { useQueryClient } from "@tanstack/react-query";
import type { WireError } from "@/lib/wails";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import {
  ProductTermsFields,
  ProductPolicyFields,
} from "@/features/liquidity/ProductFields";
import { DepositFields } from "@/features/liquidity/DepositFields";
import { ProductTimeFields } from "@/features/liquidity/ProductTimeFields";
import {
  policyForTerms,
  policyFromDTO,
  termsFromProduct,
} from "@/features/liquidity/productPolicy";
import { useProduct } from "@/queries/liquidity";
import { useHistoryOrigin } from "@/queries/history";
import { ReconciliationService } from "@/queries/reconciliation";
import { callService } from "@/lib/wails";
import { displayError } from "@/lib/display";
import type { ReconciliationLink } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import type { ProductPreview } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation/models";
import type { ProductDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import type { ProductLocalTime } from "@/features/liquidity/ProductTimeFields";
export function ProductCorrection({
  productId,
  link,
  onPreview,
}: {
  productId: string;
  link: ReconciliationLink;
  onPreview: (value: ProductPreview) => void;
}) {
  const { t } = useTranslation();
  const product = useProduct(productId, true);
  return product.data ? (
    <ProductCorrectionForm
      product={product.data.product}
      link={link}
      onPreview={onPreview}
    />
  ) : (
    <div role={product.isError ? "alert" : "status"}>
      <p>
        {t(product.isError ? "reconciliation.error" : "reconciliation.loading")}
      </p>
      {product.isError && (
        <Button onClick={() => void product.refetch()}>
          {t("reconciliation.retry")}
        </Button>
      )}
    </div>
  );
}
function ProductCorrectionForm({
  product,
  link,
  onPreview,
}: {
  product: ProductDTO;
  link: ReconciliationLink;
  onPreview: (value: ProductPreview) => void;
}) {
  const { t } = useTranslation();
  const history = useHistoryOrigin();
  const client = useQueryClient();
  const [mode, setMode] = useState("terms");
  const [terms, setTerms] = useState(() => termsFromProduct(product));
  const [rules, setRules] = useState(() => policyFromDTO(product.policy));
  const [amount, setAmount] = useState("");
  const [interest, setInterest] = useState("");
  const [principal, setPrincipal] = useState("");
  const [openingFee, setOpeningFee] = useState("");
  const [fee, setFee] = useState("");
  const [paidThrough, setPaidThrough] = useState("");
  const [remaining, setRemaining] = useState("");
  const [localTime, setLocalTime] = useState<ProductLocalTime>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const policy =
    product.state === "open" ? policyForTerms(terms, rules) : rules;
  const preview = async () => {
    setBusy(true);
    setError("");
    try {
      const value =
        mode === "terms"
          ? await callService(() =>
              ReconciliationService.PreviewProductTerms({
                productId: product.id,
                expectedRevision: product.revision,
                terms,
                policy,
                reconciliationLink: link,
              }),
            )
          : mode === "value"
            ? await callService(() =>
                ReconciliationService.PreviewProductValuation(
                  link,
                  product.id,
                  amount,
                  "",
                ),
              )
            : await callService(() =>
                ReconciliationService.PreviewProductOperation({
                  kind: mode,
                  reconciliationLink: link,
                  ...(mode === "receive_interest"
                    ? {
                        receiveInterest: {
                          productId: product.id,
                          amount,
                          effectiveAt: "",
                          ...localTime,
                          interestPaidThroughOn: paidThrough || null,
                          remainingInterest: remaining || null,
                        },
                      }
                    : mode === "renew"
                      ? {
                          renew: {
                            settle: {
                              productId: product.id,
                              returnedPrincipal:
                                product.kind === "term_deposit" ? amount : null,
                              grossProceeds:
                                product.kind === "locked_product"
                                  ? amount
                                  : null,
                              interest: interest || null,
                              fee: fee || null,
                              effectiveAt: "",
                              ...localTime,
                              releaseReservationIds: [],
                            },
                            principal,
                            openingFee: openingFee || null,
                            terms,
                            policy,
                          },
                        }
                      : {
                          settle: {
                            productId: product.id,
                            returnedPrincipal:
                              product.kind === "term_deposit" ? amount : null,
                            grossProceeds:
                              product.kind === "locked_product" ? amount : null,
                            interest: interest || null,
                            fee: fee || null,
                            effectiveAt: "",
                            ...localTime,
                            releaseReservationIds: [],
                          },
                        }),
                }),
              );
      onPreview(value);
    } catch (cause) {
      const wire = (cause as { wireError?: WireError })?.wireError;
      if (knownReconciliationConflict(wire))
        void client.invalidateQueries({ queryKey: ["reconciliation"] });
      setError(
        completedReconciliation(wire)
          ? t("reconciliation.readOnly")
          : knownReconciliationConflict(wire)
            ? t("reconciliation.changedReview")
            : displayError(cause, t("reconciliation.error")),
      );
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="flex flex-col gap-4">
      <p>{t("reconciliation.maturityHelp")}</p>
      <p>
        {t("reconciliation.current")}:{" "}
        {product.currentValue?.amount ?? product.principal.amount}{" "}
        {product.principal.currency}
      </p>
      <Label htmlFor="product-correction-mode">
        {t("reconciliation.productRoute")}
      </Label>
      <NativeSelect
        id="product-correction-mode"
        value={mode}
        onChange={(e) => setMode(e.target.value)}
      >
        <option value="terms">{t("availableFunds.editTerms")}</option>
        {product.state === "open" && (
          <>
            <option value="receive_interest">
              {t("availableFunds.receiveInterest")}
            </option>
            <option value="renew">{t("reconciliation.renew")}</option>
            <option value="settle">{t("availableFunds.confirmReceipt")}</option>
            {product.kind === "locked_product" && (
              <option value="value">
                {t("availableFunds.updateValuation")}
              </option>
            )}
          </>
        )}
      </NativeSelect>
      {mode === "terms" ? (
        product.state !== "open" ? (
          <>
            <p>{t("availableFunds.closedMetadataOnly")}</p>
            <Label htmlFor="closed-product-name">
              {t("availableFunds.name")}
            </Label>
            <Input
              id="closed-product-name"
              value={terms.name}
              onChange={(e) => setTerms({ ...terms, name: e.target.value })}
            />
            <Label htmlFor="closed-product-note">
              {t("availableFunds.contractNote")}
            </Label>
            <Input
              id="closed-product-note"
              value={terms.note ?? ""}
              onChange={(e) =>
                setTerms({ ...terms, note: e.target.value || null })
              }
            />
          </>
        ) : (
          <>
            {product.kind === "term_deposit" ? (
              <DepositFields
                terms={terms}
                policy={policy}
                principal={product.principal.amount}
                onTermsChange={setTerms}
                onPolicyChange={setRules}
                editing
              />
            ) : (
              <>
                <ProductTermsFields value={terms} onChange={setTerms} editing />
                <ProductPolicyFields value={policy} onChange={setRules} />
              </>
            )}
          </>
        )
      ) : (
        <>
          <Label htmlFor="product-correction-amount">
            {t(
              mode === "value"
                ? "availableFunds.currentValue"
                : mode === "receive_interest"
                  ? "availableFunds.interestReceived"
                  : product.kind === "term_deposit"
                    ? "availableFunds.returnedPrincipal"
                    : "availableFunds.grossProceeds",
            )}
          </Label>
          <Input
            id="product-correction-amount"
            inputMode="decimal"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
          />
          {(mode === "settle" || mode === "renew") && (
            <>
              {product.kind === "term_deposit" && (
                <>
                  <Label htmlFor="product-correction-interest">
                    {t("availableFunds.interestReceived")}
                  </Label>
                  <Input
                    id="product-correction-interest"
                    inputMode="decimal"
                    value={interest}
                    onChange={(e) => setInterest(e.target.value)}
                  />
                </>
              )}
              <Label htmlFor="product-correction-fee">
                {t("availableFunds.fee")}
              </Label>
              <Input
                id="product-correction-fee"
                inputMode="decimal"
                value={fee}
                onChange={(e) => setFee(e.target.value)}
              />
            </>
          )}
          {mode !== "value" && (
            <ProductTimeFields
              value={localTime}
              onChange={setLocalTime}
              timezone={history.data?.timezone}
            />
          )}
        </>
      )}
      {mode === "receive_interest" && (
        <>
          <Label htmlFor="receipt-paid-through">
            {t("availableFunds.interestPaidThrough")}
          </Label>
          <DatePicker
            id="receipt-paid-through"
            value={paidThrough}
            onChange={setPaidThrough}
            clearable
          />
          <Label htmlFor="receipt-remaining">
            {t("availableFunds.remainingInterest")}
          </Label>
          <Input
            id="receipt-remaining"
            value={remaining}
            onChange={(e) => setRemaining(e.target.value)}
          />
        </>
      )}
      {mode === "renew" && (
        <>
          <Label htmlFor="renew-principal">
            {t("reconciliation.newPrincipal")}
          </Label>
          <Input
            id="renew-principal"
            inputMode="decimal"
            value={principal}
            onChange={(e) => setPrincipal(e.target.value)}
          />
          <Label htmlFor="renew-fee">{t("reconciliation.openingFee")}</Label>
          <Input
            id="renew-fee"
            inputMode="decimal"
            value={openingFee}
            onChange={(e) => setOpeningFee(e.target.value)}
          />
          <ProductTermsFields value={terms} onChange={setTerms} />
          <ProductPolicyFields value={policy} onChange={setRules} />
        </>
      )}
      {error && <p role="alert">{error}</p>}
      <Button disabled={busy} onClick={() => void preview()}>
        {t("reconciliation.preview")}
      </Button>
    </div>
  );
}

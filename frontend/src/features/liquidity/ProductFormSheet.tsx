import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Sheet, SheetContent, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { defaultProductPolicy, emptyTerms, policyForTerms } from "@/features/liquidity/productPolicy";
import { displayError } from "@/lib/display";
import { formatAmount } from "@/lib/money";
import { useAccountValuations } from "@/queries/accounts";
import { useHistoryOrigin } from "@/queries/history";
import {
  usePreviewProductOperation,
  useRecordProductOperation
} from "@/queries/liquidity";
import { useSupportedCurrencies } from "@/queries/settings";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { ProductPolicyInput, ProductTermsInput } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import type { ProductCommandRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import { ProductPolicyFields, ProductTermsFields } from "./ProductFields";
import { ProductPreview } from "./ProductPreview";
import { ProductTimeFields, type ProductLocalTime } from "./ProductTimeFields";
import { useReviewedCommand } from "./useReviewedCommand";


export function ProductFormSheet({
  accountId,
  currency: defaultCurrency,
  accountName,
  cashAmount,
  open,
  onOpenChange,
}: {
  accountId: string;
  currency: string;
  cashAmount?: string;
  accountName?: string;
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
  const [error, setError] = useState<string>();
  const [invalidField, setInvalidField] = useState<string>();
  const formError = (cause: unknown) => {
    setInvalidField((cause as { wireError?: { field?: string } })?.wireError?.field);
    setError(displayError(cause, t("availableFunds.loadError")));
  };
  const policy = policyForTerms(terms, rules);
  const command: ProductCommandRequest = mode === "open"
    ? { kind: "open", open: { accountId, currency, principal, openingFee, effectiveAt: "", ...localTime, terms, policy } }
    : { kind: "record_existing", recordExisting: { accountId, currency, principal, totalCostBasis, currentValue: currentValue || principal, cashExcludesProduct: cashExcludes, effectiveAt: "", terms, policy } };
  const { reviewed, setReviewed } = useReviewedCommand(command);
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
        <p className="text-sm">{t("availableFunds.targetAccount")}: {accountValue?.account.name ?? accountName ?? accountId}</p>
        <p className="text-sm">{t("availableFunds.cashAvailable", { currency, amount: selectedCash === undefined ? t("availableFunds.unknownAmount") : formatAmount(selectedCash, currency) })}</p>
        {mode === "open" && <ProductTimeFields value={localTime} onChange={(next) => { setLocalTime(next); setReviewed(null); }} timezone={history.data?.timezone} />}
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
        <Input aria-invalid={invalidField === "principal"} aria-describedby={invalidField === "principal" ? "product-form-error" : undefined} id="product-principal" value={principal} onChange={(event) => setPrincipal(event.target.value)} />
        {mode === "record_existing" && (
          <>
            <p className="text-sm text-muted-foreground">{t("availableFunds.recordExistingHelp")}</p>
            <Label htmlFor="product-value">{t("availableFunds.currentContractValue")}</Label>
            <Input id="product-value" value={currentValue} onChange={(event) => setCurrentValue(event.target.value)} />
            <Label htmlFor="product-cost">{t("availableFunds.cost")}</Label>
            <Input id="product-cost" value={totalCostBasis} onChange={(event) => setTotalCostBasis(event.target.value)} />
            <label className="flex items-center gap-2 text-sm">
              <input aria-invalid={invalidField === "cashExcludesProduct"} aria-describedby={invalidField === "cashExcludesProduct" ? "product-form-error" : undefined} type="checkbox" checked={cashExcludes} onChange={(event) => setCashExcludes(event.target.checked)} />
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
        <ProductTermsFields invalidField={invalidField} errorId="product-form-error" value={terms} onChange={(next) => { setTerms(next); setReviewed(null); }} />
        <ProductPolicyFields value={policy} onChange={(next) => { setRules(next); setReviewed(null); }} termDeposit={terms.kind === "term_deposit"} />
        {reviewed && <ProductPreview preview={reviewed} />}
        {error && <p id="product-form-error" role="alert" className="text-sm text-destructive">{error}</p>}
        <SheetFooter>
          <Button type="button" variant="outline" disabled={preview.isPending || record.isPending} onClick={() => {
            setError(undefined);
            setInvalidField(undefined);
            const required = [
              ["principal", principal, "principal"], ["name", terms.name, "name"],
              ["startOn", terms.startOn, "startOn"],
              ...(terms.kind === "term_deposit" ? [["maturityOn", terms.maturityOn, "maturityOn"]] : []),
            ].find(([, value]) => !value?.trim());
            if (required) {
              setInvalidField(required[0]!);
              setError(t("availableFunds.requiredField", { field: t(`availableFunds.${required[2]}`) }));
              return;
            }
            if (mode === "record_existing" && !cashExcludes) {
              setInvalidField("cashExcludesProduct");
              setError(t("availableFunds.confirmCashExcludes"));
              return;
            }
            void preview.mutateAsync(command).then(setReviewed).catch(formError);
          }}>{t("availableFunds.preview")}</Button>
          <Button type="button" disabled={!reviewed || record.isPending} onClick={() => {
            if (!reviewed || record.isPending) {
              return;
            }
            setError(undefined);
            void record.mutateAsync({ command, mutationId: "", reviewedStateHash: reviewed.reviewedStateHash }).then(() => onOpenChange(false)).catch(formError);
          }}>{t("availableFunds.confirm")}</Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

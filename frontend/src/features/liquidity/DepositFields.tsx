import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { DatePicker } from "@/components/ui/date-picker";
import { ProductPolicyFields } from "./ProductFields";
import type { ProductPolicyInput, ProductTermsInput } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";

export function DepositFields({ terms, policy, principal, onTermsChange, onPolicyChange, editing = false, invalidField, errorId }: {
  terms: ProductTermsInput; policy: ProductPolicyInput; principal: string;
  onTermsChange: (value: ProductTermsInput) => void; onPolicyChange: (value: ProductPolicyInput) => void;
  editing?: boolean; invalidField?: string; errorId?: string;
}) {
  const { t } = useTranslation();
  const id = useId();
  const simple = terms.interestMode.startsWith("simple_act_");
  const invalid = (field: string) => ({ "aria-invalid": invalidField === field, "aria-describedby": invalidField === field ? errorId : undefined });
  return <div className="flex flex-col gap-3">
    <div className="grid grid-cols-2 gap-3">
      <div className="flex flex-col gap-2">
        <Label htmlFor={`${id}-start`}>{t("availableFunds.startOn")}</Label>
        <DatePicker clearable allowFuture id={`${id}-start`} disabled={editing} value={terms.startOn} onChange={(startOn) => onTermsChange({ ...terms, startOn })} {...invalid("startOn")} />
      </div>
      <div className="flex flex-col gap-2">
        <Label htmlFor={`${id}-end`}>{t("availableFunds.maturityOn")}</Label>
        <DatePicker clearable allowFuture id={`${id}-end`} value={terms.maturityOn ?? ""} onChange={(maturityOn) => onTermsChange({ ...terms, maturityOn: maturityOn || null })} {...invalid("maturityOn")} />
      </div>
    </div>
    <Label htmlFor={`${id}-interest`}>{t("availableFunds.depositInterest")}</Label>
    <NativeSelect id={`${id}-interest`} value={simple ? "rate" : terms.interestMode} onChange={(e) => onTermsChange({ ...terms, interestMode: e.target.value === "rate" ? "simple_act_365" : e.target.value, annualRate: null, annualRatePercent: null, maturityInterest: null, interestPaidThroughOn: null })}>
      <option value="none">{t("availableFunds.depositInterestLater")}</option>
      <option value="rate">{t("availableFunds.annualRatePercent")}</option>
      <option value="manual_maturity_amount">{t("availableFunds.maturityInterest")}</option>
    </NativeSelect>
    {simple && <>
      <Label htmlFor={`${id}-rate`}>{t("availableFunds.annualRatePercent")}</Label>
      <Input id={`${id}-rate`} inputMode="decimal" value={terms.annualRatePercent ?? ""} onChange={(e) => onTermsChange({ ...terms, annualRate: null, annualRatePercent: e.target.value || null })} />
    </>}
    {terms.interestMode === "manual_maturity_amount" && <>
      <Label htmlFor={`${id}-interest-amount`}>{t("availableFunds.maturityInterest")}</Label>
      <Input id={`${id}-interest-amount`} inputMode="decimal" value={terms.maturityInterest ?? ""} onChange={(e) => onTermsChange({ ...terms, maturityInterest: e.target.value || null })} />
    </>}
    <label className="flex items-center gap-2 text-sm">
      <input type="checkbox" checked={policy.earlyKind === "allowed"} onChange={(e) => onPolicyChange({ ...policy, earlyKind: e.target.checked ? "allowed" : "not_allowed", ...(e.target.checked ? { earlyAmountMode: "fixed_gross", earlyGrossAmount: principal || null, earlyFee: "0", earlySettlementDays: 0, earlyDayBasis: "calendar" } : {}) })} />
      {t("availableFunds.depositEarly")}
    </label>
    {policy.earlyKind === "allowed" && <>
      {policy.earlyAmountMode === "fixed_gross" && <>
        <Label htmlFor={`${id}-early-gross`}>{t("availableFunds.earlyGross")}</Label>
        <Input id={`${id}-early-gross`} inputMode="decimal" value={policy.earlyGrossAmount ?? ""} onChange={(e) => onPolicyChange({ ...policy, earlyGrossAmount: e.target.value || null })} />
      </>}
      <p className="text-xs text-muted-foreground">{t("availableFunds.depositEarlyHelp")}</p>
    </>}
    <details className="rounded-lg border border-border p-3">
      <summary className="cursor-pointer text-sm font-medium">{t("availableFunds.depositAdvanced")}</summary>
      <div className="mt-3 flex flex-col gap-3">
        <Label htmlFor={`${id}-name`}>{t("availableFunds.name")}</Label>
        <Input id={`${id}-name`} value={terms.name} onChange={(e) => onTermsChange({ ...terms, name: e.target.value })} {...invalid("name")} />
        {simple && <>
          <Label htmlFor={`${id}-basis`}>{t("availableFunds.depositYearBasis")}</Label>
          <NativeSelect id={`${id}-basis`} value={terms.interestMode} onChange={(e) => onTermsChange({ ...terms, interestMode: e.target.value })}>
            <option value="simple_act_365">365</option><option value="simple_act_360">360</option>
          </NativeSelect>
          {editing && <>
            <Label htmlFor={`${id}-paid`}>{t("availableFunds.interestPaidThrough")}</Label>
            <DatePicker clearable allowFuture id={`${id}-paid`} value={terms.interestPaidThroughOn ?? ""} onChange={(interestPaidThroughOn) => onTermsChange({ ...terms, interestPaidThroughOn: interestPaidThroughOn || null })} />
          </>}
        </>}
        <Label htmlFor={`${id}-note`}>{t("availableFunds.contractNote")}</Label>
        <Input id={`${id}-note`} value={terms.note ?? ""} onChange={(e) => onTermsChange({ ...terms, note: e.target.value || null })} />
        <ProductPolicyFields value={policy} onChange={onPolicyChange} termDeposit hideEarlyGross />
      </div>
    </details>
  </div>;
}

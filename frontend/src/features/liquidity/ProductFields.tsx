import { DatePicker } from "@/components/ui/date-picker";
import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import type { ProductPolicyInput, ProductTermsInput } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";

export function ProductTermsFields({ value, onChange, editing = false, invalidField, errorId }: {
  value: ProductTermsInput; onChange: (value: ProductTermsInput) => void; editing?: boolean; invalidField?: string; errorId?: string;
}) {
  const { t } = useTranslation();
  const id = useId();
  const simple = value.interestMode === "simple_act_365" || value.interestMode === "simple_act_360";
  return <fieldset className="flex flex-col gap-2">
    <legend className="mb-2 font-medium">{t("availableFunds.contractTerms")}</legend>
    <Label htmlFor={`${id}-name`}>{t("availableFunds.name")}</Label>
    <Input aria-invalid={invalidField === "name"} aria-describedby={invalidField === "name" ? errorId : undefined} id={`${id}-name`} value={value.name} onChange={(e) => onChange({ ...value, name: e.target.value })} />
    <Label htmlFor={`${id}-start`}>{t("availableFunds.startOn")}</Label>
    <DatePicker clearable allowFuture aria-invalid={invalidField === "startOn"} aria-describedby={invalidField === "startOn" ? errorId : undefined} id={`${id}-start`} disabled={editing} value={value.startOn} onChange={(next) => onChange({ ...value, startOn: next })} />
    <Label htmlFor={`${id}-maturity`}>{t("availableFunds.maturityOn")}</Label>
    <DatePicker clearable allowFuture aria-invalid={invalidField === "maturityOn"} aria-describedby={invalidField === "maturityOn" ? errorId : undefined} id={`${id}-maturity`} value={value.maturityOn ?? ""} onChange={(next) => onChange({ ...value, maturityOn: next || null })} />
    {value.kind === "term_deposit" && <>
      <Label htmlFor={`${id}-interest`}>{t("availableFunds.interestMode")}</Label>
      <NativeSelect id={`${id}-interest`} value={value.interestMode} onChange={(e) => onChange({ ...value, interestMode: e.target.value, annualRate: null, annualRatePercent: null, maturityInterest: null, interestPaidThroughOn: null })}>
        <option value="none">{t("availableFunds.interestNone")}</option>
        <option value="simple_act_365">{t("availableFunds.interestSimple365")}</option>
        <option value="simple_act_360">{t("availableFunds.interestSimple360")}</option>
        <option value="manual_maturity_amount">{t("availableFunds.interestManual")}</option>
      </NativeSelect>
      {simple && <>
        <Label htmlFor={`${id}-rate`}>{t("availableFunds.annualRatePercent")}</Label>
        <Input id={`${id}-rate`} inputMode="decimal" value={value.annualRatePercent ?? ""} onChange={(e) => onChange({ ...value, annualRate: null, annualRatePercent: e.target.value || null })} />
        {editing && <>
          <Label htmlFor={`${id}-paid`}>{t("availableFunds.interestPaidThrough")}</Label>
          <DatePicker clearable allowFuture id={`${id}-paid`} value={value.interestPaidThroughOn ?? ""} onChange={(next) => onChange({ ...value, interestPaidThroughOn: next || null })} />
        </>}
      </>}
      {value.interestMode === "manual_maturity_amount" && <>
        <Label htmlFor={`${id}-unpaid`}>{t("availableFunds.maturityInterest")}</Label>
        <Input id={`${id}-unpaid`} inputMode="decimal" value={value.maturityInterest ?? ""} onChange={(e) => onChange({ ...value, maturityInterest: e.target.value || null })} />
      </>}
    </>}
    <Label htmlFor={`${id}-note`}>{t("availableFunds.contractNote")}</Label>
    <Input id={`${id}-note`} value={value.note ?? ""} onChange={(e) => onChange({ ...value, note: e.target.value || null })} />
  </fieldset>;
}

export function ProductPolicyFields({ value, onChange, termDeposit = false, ordinary = false }: {
  value: ProductPolicyInput; onChange: (value: ProductPolicyInput) => void; termDeposit?: boolean; ordinary?: boolean;
}) {
  const { t } = useTranslation();
  const id = useId();
  return <fieldset className="flex flex-col gap-2">
    <legend className="mb-2 font-medium">{t("availableFunds.policy")}</legend>
    {ordinary && <>
      <Label htmlFor={`${id}-cap`}>{t("availableFunds.accessibleCap")}</Label>
      <Input id={`${id}-cap`} inputMode="decimal" value={value.accessibleAmountCap ?? ""} onChange={(e) => onChange({ ...value, accessibleAmountCap: e.target.value || null })} />
      <p className="text-xs text-muted-foreground">{t("availableFunds.capHelp")}</p>
    </>}
    <Label htmlFor={`${id}-access`}>{t("availableFunds.accessKind")}</Label>
    <NativeSelect id={`${id}-access`} value={value.accessKind} onChange={(e) => onChange({ ...value, accessKind: e.target.value })}>
      {!termDeposit && <option value="on_request">{t("availableFunds.accessOnRequest")}</option>}
      <option value="on_date">{t("availableFunds.accessOnDate")}</option>
      <option value="unknown">{t("availableFunds.accessUnknown")}</option>
      <option value="excluded">{t("availableFunds.accessExcluded")}</option>
    </NativeSelect>
    {(value.accessKind === "on_date" || value.accessKind === "on_request") && <>
      <Label htmlFor={`${id}-unlock`}>{t("availableFunds.unlockOn")}</Label>
      <DatePicker clearable allowFuture id={`${id}-unlock`} disabled={termDeposit} value={value.unlockOn ?? ""} onChange={(next) => onChange({ ...value, unlockOn: next || null })} />
    </>}
    <Label htmlFor={`${id}-days`}>{t("availableFunds.settlementDays")}</Label>
    <Input id={`${id}-days`} type="number" min={0} max={365} value={value.settlementDays ?? ""} onChange={(e) => onChange({ ...value, settlementDays: e.target.value === "" ? null : Number(e.target.value), dayBasis: value.dayBasis ?? "calendar" })} />
    <Label htmlFor={`${id}-basis`}>{t("availableFunds.dayBasis")}</Label>
    <NativeSelect id={`${id}-basis`} value={value.dayBasis ?? "calendar"} onChange={(e) => onChange({ ...value, dayBasis: e.target.value })}>
      <option value="calendar">{t("availableFunds.calendar")}</option><option value="weekdays">{t("availableFunds.weekdays")}</option>
    </NativeSelect>
    <Label htmlFor={`${id}-receipt`}>{t("availableFunds.receiptOverride")}</Label>
    <DatePicker clearable allowFuture id={`${id}-receipt`} value={value.receiptOnOverride ?? ""} onChange={(next) => onChange({ ...value, receiptOnOverride: next || null })} />
    <Label htmlFor={`${id}-fee`}>{t("availableFunds.normalExitFee")}</Label>
    <Input id={`${id}-fee`} inputMode="decimal" value={value.normalExitFee ?? ""} onChange={(e) => onChange({ ...value, normalExitFee: e.target.value || null })} />
    <p className="text-xs text-muted-foreground">{t("availableFunds.unknownPolicyInputs")}</p>
    <Label htmlFor={`${id}-early`}>{t("availableFunds.earlyKind")}</Label>
    <NativeSelect id={`${id}-early`} value={value.earlyKind} onChange={(e) => onChange({ ...value, earlyKind: e.target.value, earlyDayBasis: value.earlyDayBasis ?? "calendar", earlyAmountMode: value.earlyAmountMode ?? "current_value" })}>
      <option value="not_allowed">{t("availableFunds.earlyNotAllowed")}</option><option value="allowed">{t("availableFunds.earlyAllowed")}</option><option value="unknown">{t("availableFunds.earlyUnknown")}</option>
    </NativeSelect>
    {value.earlyKind === "allowed" && <>
      <Label htmlFor={`${id}-early-days`}>{t("availableFunds.earlySettlementDays")}</Label>
      <Input id={`${id}-early-days`} type="number" min={0} max={365} value={value.earlySettlementDays ?? ""} onChange={(e) => onChange({ ...value, earlySettlementDays: e.target.value === "" ? null : Number(e.target.value) })} />
      <Label htmlFor={`${id}-early-basis`}>{t("availableFunds.earlyDayBasis")}</Label>
      <NativeSelect id={`${id}-early-basis`} value={value.earlyDayBasis ?? "calendar"} onChange={(e) => onChange({ ...value, earlyDayBasis: e.target.value })}>
        <option value="calendar">{t("availableFunds.calendar")}</option><option value="weekdays">{t("availableFunds.weekdays")}</option>
      </NativeSelect>
      <Label htmlFor={`${id}-early-fee`}>{t("availableFunds.earlyFee")}</Label>
      <Input id={`${id}-early-fee`} inputMode="decimal" value={value.earlyFee ?? ""} onChange={(e) => onChange({ ...value, earlyFee: e.target.value || null })} />
      <Label htmlFor={`${id}-early-mode`}>{t("availableFunds.earlyAmountMode")}</Label>
      <NativeSelect id={`${id}-early-mode`} value={value.earlyAmountMode ?? "current_value"} onChange={(e) => onChange({ ...value, earlyAmountMode: e.target.value, earlyGrossAmount: null })}>
        <option value="current_value">{t("availableFunds.currentContractValue")}</option><option value="fixed_gross">{t("availableFunds.earlyGross")}</option>
      </NativeSelect>
      {value.earlyAmountMode === "fixed_gross" && <>
        <Label htmlFor={`${id}-gross`}>{t("availableFunds.earlyGross")}</Label>
        <Input id={`${id}-gross`} inputMode="decimal" value={value.earlyGrossAmount ?? ""} onChange={(e) => onChange({ ...value, earlyGrossAmount: e.target.value || null })} />
      </>}
    </>}
    {(value.dayBasis === "weekdays" || (value.earlyKind === "allowed" && value.earlyDayBasis === "weekdays")) && <p className="text-xs text-muted-foreground">{t("availableFunds.weekdaysNote")}</p>}
  </fieldset>;
}

import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { DatePicker } from "@/components/ui/date-picker";
import { defaultProductPolicy } from "./productPolicy";
import { ProductPolicyFields } from "./ProductFields";
import type { ProductPolicyInput } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";

type Preset = "now" | "soon" | "date" | "excluded";
export function simplePolicy(preset: Preset, current: ProductPolicyInput): ProductPolicyInput {
  return { ...defaultProductPolicy(null), note: current.note,
    accessKind: preset === "excluded" ? "excluded" : preset === "date" ? "on_date" : "on_request",
    unlockOn: preset === "date" ? current.unlockOn : null,
    settlementDays: preset === "soon" ? 1 : 0,
    dayBasis: preset === "soon" ? "weekdays" : "calendar",
  };
}
export function policyPreset(value: ProductPolicyInput): Preset | null {
  for (const preset of ["now", "soon", "date", "excluded"] as const) {
    const candidate = simplePolicy(preset, value);
    if (Object.keys(candidate).every((key) => (value[key as keyof ProductPolicyInput] ?? null) === (candidate[key as keyof ProductPolicyInput] ?? null))) return preset;
  }
  return null;
}
export function SimplePolicyFields({value, onChange}: {value: ProductPolicyInput; onChange: (value: ProductPolicyInput) => void}) {
  const {t} = useTranslation();
  const id = useId();
  const selected = policyPreset(value);
  const custom = !selected && value.accessKind !== "unknown";
  const [advanced, setAdvanced] = useState(custom);
  return <div className="flex flex-col gap-4">
    <fieldset className="grid gap-2">
      <legend className="mb-2 font-medium">{t("availableFunds.quickRule")}</legend>
      {(["now", "soon", "date", "excluded"] as const).map((preset) => <label key={preset} className="flex cursor-pointer items-start gap-3 rounded-lg border border-border p-3 has-[:checked]:border-primary has-[:checked]:bg-primary/5">
        <input type="radio" name={id} checked={selected === preset} onChange={() => onChange(simplePolicy(preset, value))} className="mt-1 accent-primary" />
        <span><span className="block font-medium">{t(`availableFunds.quick_${preset}`)}</span><span className="text-xs text-muted-foreground">{t(`availableFunds.quick_${preset}_hint`)}</span></span>
      </label>)}
    </fieldset>
    {selected === "date" && <div className="flex flex-col gap-2"><label htmlFor={`${id}-date`}>{t("availableFunds.quickDate")}</label><DatePicker id={`${id}-date`} allowFuture clearable value={value.unlockOn ?? ""} onChange={(date) => onChange({...value, unlockOn: date || null})} /></div>}
    <p className="text-xs text-muted-foreground">{t("availableFunds.quickEstimate")}</p>
    <details open={advanced} onToggle={(event) => setAdvanced(event.currentTarget.open)} className="rounded-lg border border-border p-3">
      <summary className="cursor-pointer text-sm font-medium">{t(custom ? "availableFunds.customRule" : "availableFunds.advancedRule")}</summary>
      {advanced && <div className="mt-4"><ProductPolicyFields ordinary value={value} onChange={onChange} /></div>}
    </details>
  </div>;
}

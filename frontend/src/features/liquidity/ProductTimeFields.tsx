import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
export type ProductLocalTime = { effectiveLocalDate?: string; effectiveLocalTime?: string };
export function ProductTimeFields({ value, onChange, timezone }: { value: ProductLocalTime; onChange: (value: ProductLocalTime) => void; timezone?: string }) {
  const { t } = useTranslation(); const id = useId();
  return <fieldset className="flex flex-col gap-2">
    <legend>{t("availableFunds.actualTime")}</legend>
    <Label htmlFor={`${id}-date`}>{t("history.effectiveDate")}</Label>
    <Input id={`${id}-date`} type="date" value={value.effectiveLocalDate ?? ""} disabled={!timezone} onChange={(e) => onChange({ ...value, effectiveLocalDate: e.target.value })} />
    <Label htmlFor={`${id}-time`}>{t("history.effectiveTime")}</Label>
    <Input id={`${id}-time`} type="time" value={value.effectiveLocalTime ?? ""} disabled={!timezone} onChange={(e) => onChange({ ...value, effectiveLocalTime: e.target.value })} />
    <p className="text-xs text-muted-foreground">{t("availableFunds.actualTimeHelp", { timezone: timezone ?? t("availableFunds.unknownAmount") })}</p>
  </fieldset>;
}

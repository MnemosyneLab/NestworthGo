import { TimePicker } from "@/components/ui/time-picker";
import { DatePicker } from "@/components/ui/date-picker";
import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@/components/ui/label";
import { localDateTimeInTimeZone } from "@/lib/time";
export type ProductLocalTime = { effectiveLocalDate?: string; effectiveLocalTime?: string };
export function ProductTimeFields({ value, onChange, timezone }: { value: ProductLocalTime; onChange: (value: ProductLocalTime) => void; timezone?: string }) {
  const { t } = useTranslation(); const id = useId();
  const today = timezone ? localDateTimeInTimeZone(timezone)?.date : undefined;
  return <fieldset className="flex flex-col gap-2">
    <legend>{t("availableFunds.actualTime")}</legend>
    <Label htmlFor={`${id}-date`}>{t("history.effectiveDate")}</Label>
    <DatePicker clearable id={`${id}-date`} max={today} value={value.effectiveLocalDate ?? ""} disabled={!timezone} onChange={(next) => onChange({ ...value, effectiveLocalDate: next })} />
    <Label htmlFor={`${id}-time`}>{t("history.effectiveTime")}</Label>
    <TimePicker clearable id={`${id}-time`} value={value.effectiveLocalTime ?? ""} disabled={!timezone} onChange={(next) => onChange({ ...value, effectiveLocalTime: next })} />
    <p className="text-xs text-muted-foreground">{t("availableFunds.actualTimeHelp", { timezone: timezone ?? t("availableFunds.unknownAmount") })}</p>
  </fieldset>;
}

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { RangeToggle } from "./RangeToggle";
import { DATE_PRESETS, presetRange, shiftRange, type DatePreset, type DateRange } from "./dateRanges";

export function DateRangeControl({ value, min, max, onChange, disabled = false }: {
  value: DateRange; min?: string; max: string; onChange: (range: DateRange) => void; disabled?: boolean;
}) {
  const { t } = useTranslation();
  const [selection, setSelection] = useState<{ preset: DatePreset; range: DateRange } | null>(null);
  const active = selection && selection.range.from === value.from && selection.range.to === value.to ? selection.preset : DATE_PRESETS.find(preset => {
    const range = presetRange(preset, value.to || max, min);
    return range.from === value.from && range.to === value.to;
  }) ?? null;
  const valid = Boolean(value.from && value.to && value.from <= value.to);
  const change = (range: DateRange, preset: DatePreset | null) => {
    const clamped = { from: min && range.from < min ? min : range.from, to: range.to > max ? max : range.to };
    setSelection(preset ? { preset, range: clamped } : null);
    onChange(clamped);
  };
  const previous = valid ? shiftRange(value, -1, active) : value;
  const next = valid ? shiftRange(value, 1, active) : value;
  return <div className="flex flex-wrap items-center justify-between gap-3">
    <RangeToggle ranges={DATE_PRESETS} value={active ?? ""} disabled={disabled || Boolean(min && min > max)} onChange={preset => change(presetRange(preset as DatePreset, max, min), preset as DatePreset)} label={t("rangeShortcuts.label")} />
    <div className="flex items-center gap-1 rounded-lg border border-border bg-card p-1" role="group" aria-label={t("rangeShortcuts.navigate")}>
      <Button size="icon" variant="ghost" className="h-8 w-8" aria-label={t("rangeShortcuts.previous")} disabled={disabled || !valid || active === "all" || Boolean(min && previous.to < min)} onClick={() => change(previous, active)}><ChevronLeft className="h-4 w-4" /></Button>
      <span className="px-2 text-xs tabular-nums text-muted-foreground">{!valid ? t("common.selectOption") : active === "all" ? t("analytics.rangeAll") : value.from === value.to ? value.from : `${value.from} – ${value.to}`}</span>
      <Button size="icon" variant="ghost" className="h-8 w-8" aria-label={t("rangeShortcuts.next")} disabled={disabled || !valid || active === "all" || next.from > max} onClick={() => change(next, active)}><ChevronRight className="h-4 w-4" /></Button>
    </div>
  </div>;
}

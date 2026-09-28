import { addDays, addMonths, differenceInCalendarDays } from "date-fns";
import { parseYmd, ymd } from "@/features/insights/calendar";

export const DATE_PRESETS = ["1d", "1w", "1m", "3m", "6m", "1y", "all"] as const;
export type DatePreset = typeof DATE_PRESETS[number];
export type DateRange = { from: string; to: string };

export function presetRange(preset: DatePreset, to: string, min?: string): DateRange {
  const end = parseYmd(to);
  const from = preset === "all" ? min || to : ymd(preset === "1d" ? end : preset === "1w" ? addDays(end, -6) : addDays(addMonths(end, -(preset === "1y" ? 12 : Number(preset.slice(0, -1)))), 1));
  return { from: min && from < min ? min : from, to };
}

export function shiftRange(range: DateRange, direction: -1 | 1, preset?: DatePreset | null): DateRange {
  if (preset && preset !== "all") {
    // Anchor at the boundary so adjacent periods never overlap or skip days.
    if (direction === -1) {
      const to = ymd(addDays(parseYmd(range.from), -1));
      if (preset === "1d" || preset === "1w") return presetRange(preset, to);
      return { from: ymd(addMonths(parseYmd(range.from), -(preset === "1y" ? 12 : Number(preset.slice(0, -1))))), to };
    }
    const from = ymd(addDays(parseYmd(range.to), 1));
    const start = parseYmd(from);
    const to = ymd(preset === "1d" ? start : preset === "1w" ? addDays(start, 6) : addDays(addMonths(start, preset === "1y" ? 12 : Number(preset.slice(0, -1))), -1));
    return { from, to };
  }
  const days = differenceInCalendarDays(parseYmd(range.to), parseYmd(range.from)) + 1;
  return { from: ymd(addDays(parseYmd(range.from), days * direction)), to: ymd(addDays(parseYmd(range.to), days * direction)) };
}

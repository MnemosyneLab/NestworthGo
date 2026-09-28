import { useState } from "react";
import { useHistoryOrigin } from "@/queries/history";
import { useSettings } from "@/queries/settings";
import { originLocalDate } from "@/features/insights/analysisRequest";
import { ymdInTimeZone } from "@/features/insights/calendar";
import { presetRange, type DateRange } from "./dateRanges";

export function useTrendDateRange(initial?: DateRange, marketData = false) {
  const origin = useHistoryOrigin();
  const settings = useSettings();
  const timezone = origin.data?.timezone || (settings.data?.timezone === "system" ? undefined : settings.data?.timezone);
  const max = ymdInTimeZone(new Date(), timezone);
  // Market quotes may predate the household's history.
  const min = !marketData && origin.data?.startedAt ? originLocalDate(origin.data.startedAt, timezone) : "1970-01-01";
  const [selected, onChange] = useState<DateRange | undefined>(initial);
  const value = selected ?? presetRange("1m", max, min);
  const queryRange: "all" | `${string}:${string}` = value.from === min && value.to === max ? "all" : `${value.from}:${value.to}`;
  return { value, min, max, onChange, queryRange };
}

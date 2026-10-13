import { useEffect, useSyncExternalStore } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useHistoryOrigin } from "./history";
import { useSettings } from "./settings";
import { invalidateCurrentValuation } from "./invalidation";
import { ymdInTimeZone } from "@/features/insights/calendar";

let generation = 0;
const listeners = new Set<() => void>();
const subscribe = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; };
export function useValuationDateGeneration() { return useSyncExternalStore(subscribe, () => generation); }

/** One mounted ledger clock. Timer/resume checks are local; only a civil-day
 * change invalidates valuation reads. No provider refresh is dispatched. */
export function useValuationDayBoundary() {
  const client = useQueryClient();
  const origin = useHistoryOrigin();
  const settings = useSettings();
  const timezone = origin.data?.timezone || (settings.data?.timezone === "system" ? undefined : settings.data?.timezone);
  useEffect(() => {
    let previous = ymdInTimeZone(new Date(), timezone);
    const check = () => {
      const next = ymdInTimeZone(new Date(), timezone);
      if (next === previous) return;
      previous = next; generation += 1;
      listeners.forEach(listener => listener());
      invalidateCurrentValuation(client);
    };
    const timer = window.setInterval(check, 30_000);
    window.addEventListener("focus", check);
    document.addEventListener("visibilitychange", check);
    return () => { window.clearInterval(timer); window.removeEventListener("focus", check); document.removeEventListener("visibilitychange", check); };
  }, [client, timezone]);
}

import type { PagePublication } from "./contracts";
import { hasDisplay, type DisplayValuation } from "@/components/valuation/valuationQuality";
import { signedAccountAmount } from "@/lib/liabilityDisplay";

// AccountDisplayValueDTO describes account currency independently of the
// household projection. Keep the selected amount, completeness and gaps together.
export type AccountDisplayProjection = {
  currency: string; value?: { amount: string; currency: string } | null;
  complete: boolean; missingInputs?: readonly { kind: string }[] | null;
};
export function nativeAccountMetric(projection: AccountDisplayProjection | null | undefined, label: string,
  displayedMoney = projection?.value): PagePublication["metrics"][number] {
  return { label, value: displayedMoney?.amount ?? null, unit: displayedMoney?.currency ?? projection?.currency ?? "",
    basis: "account_value", status: !displayedMoney ? "unavailable" : projection?.complete === true ? "available" : "incomplete",
    missingReasons: projection?.missingInputs?.map(input => input.kind) ?? [] };
}

type Overview = { currency: string; complete: boolean; netWorth: string; assets: string; liabilities: string;
  displayValuation?: { netWorth: DisplayValuation; assets: DisplayValuation; liabilities: DisplayValuation };
  missingInputs?: { kind: string }[] | null };
export function overviewPublication(data: Overview | undefined, options: {
  observedAt: number; loading: boolean; error: boolean; timezone: string; label: string; labels: string[];
}): PagePublication {
  return { viewKey: "overview:", page: "overview", scope: { kind: "household", label: options.label },
    timezone: options.timezone, basis: "base", currency: data?.currency, observedAt: options.observedAt,
    state: options.error ? "unavailable" : options.loading ? "loading" : "available", targets: [],
    metrics: data ? (["netWorth", "assets", "liabilities"] as const).map((key, i) => {
      const display = data.displayValuation?.[key];
      // Same canonical display authority as OverviewPage. A partial subtotal
      // stays partial; absence never falls back to a fabricated full value.
      const money = hasDisplay(display) ? display.total ?? display.knownSubtotal : { amount: data[key], currency: data.currency };
      return { label: options.labels[i], value: money ? signedAccountAmount(money.amount, key === "liabilities") : null,
        unit: money?.currency ?? data.currency, basis: key === "netWorth" ? "net_worth" : key,
        status: money ? (hasDisplay(display) ? display.total ? "available" : "incomplete" : data.complete ? "available" : "incomplete") : "unavailable",
        missingReasons: data.missingInputs?.map(input => input.kind) ?? [] };
    }) : [] };
}

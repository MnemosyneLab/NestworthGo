import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Check, Zap } from "lucide-react";
import { formatAmount } from "@/lib/money";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { ChangeText } from "@/components/ui/change-text";
import { InfoPopover } from "@/components/ui/info-popover";
import { cashVersusProceeds, compareHorizonTotals, horizonUsesEarly } from "./liquidityDisplay";
import type { LiquidityBucketDTO, LiquiditySourceDTO, NativeCurrencyGroupDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import type { MoneyView } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

type Horizon = { label: string; bucket: LiquidityBucketDTO };
type Total = LiquidityBucketDTO | NativeCurrencyGroupDTO;

export function FundsComparison({ horizons, selected, onSelect, sources, baseCurrency, includeEarly = false, busy = false, notes }: {
  horizons: Horizon[]; selected: string; onSelect: (date: string) => void;
  sources: LiquiditySourceDTO[]; baseCurrency: string;
  includeEarly?: boolean; busy?: boolean; notes?: ReactNode;
}) {
  const { t } = useTranslation();
  const unknown = t("availableFunds.unknownAmount");
  const nativeCurrencies = [...new Set(horizons.flatMap(({ bucket }) => (bucket.nativeCurrencyGroups ?? []).map(group => group.currency)))].sort();
  const selectedHorizon = horizons.find(({ bucket }) => bucket.horizonOn === selected);
  const amount = (value: MoneyView | null | undefined) => !value ? unknown : formatAmount(value.amount, value.currency);
  const total = (bucket: Total | undefined) => bucket?.fullUnreserved ?? bucket?.knownUnreservedSubtotal;
  const spendable = (bucket: Total | undefined) => <>
    <span className="font-semibold tabular-nums">{amount(total(bucket))}</span>
    {!bucket?.fullUnreserved && <Badge variant="warning" className="ml-2 align-middle" title={t("availableFunds.incompleteAmount")}>{t("availableFunds.partialShort")}</Badge>}
  </>;
  const earlier = (horizon: Horizon) => horizons
    .filter(other => other.bucket.horizonOn < horizon.bucket.horizonOn)
    .sort((a, b) => b.bucket.horizonOn.localeCompare(a.bucket.horizonOn))[0];
  const breakdown = () => {
    if (!selectedHorizon) return null;
    const bucket = selectedHorizon.bucket;
    const labels = [t("availableFunds.cashAccessible"), t("availableFunds.estimatedProceeds"), t("availableFunds.appliedReserve"), t("availableFunds.availableAfterReserve")];
    const rows = nativeCurrencies.map(currency => {
      const group = bucket.nativeCurrencyGroups?.find(item => item.currency === currency);
      const matchingSources = sources.filter(source => source.nativeCurrency === currency);
      const parts = cashVersusProceeds(matchingSources, selected, unknown, currency, "native");
      // Missing currency groups remain unknown, even if no source was returned.
      const values = [group ? parts.cash : unknown, group ? parts.proceeds : unknown, amount(group?.appliedReserveSubtotal), spendable(group)];
      return { currency, values };
    });
    return <>
      <table className="hidden w-full text-sm md:table">
        <caption className="sr-only">{t("availableFunds.breakdownTitle")}</caption>
        <thead><tr className="border-b border-border text-xs text-muted-foreground"><th scope="col" className="px-3 py-2 text-left font-medium">{t("availableFunds.currencyColumn")}</th>{labels.map(label => <th scope="col" key={label} className="px-3 py-2 text-right font-medium">{label}</th>)}</tr></thead>
        <tbody>{rows.map(row => <tr key={row.currency} className="border-b border-border/60 last:border-0"><th scope="row" className="px-3 py-2.5 text-left font-semibold">{row.currency}</th>{row.values.map((value, index) => <td key={labels[index]} className={cn("px-3 py-2.5 text-right tabular-nums", index === 3 && "bg-primary/8 font-semibold")}>{value}</td>)}</tr>)}</tbody>
      </table>
      <div className="divide-y divide-border md:hidden">{rows.map(row => <div key={row.currency} className="py-3 first:pt-0 last:pb-0"><h4 className="mb-2 font-semibold">{row.currency}</h4><dl className="grid grid-cols-[1fr_auto] gap-x-3 gap-y-1.5 text-sm">{row.values.map((value, index) => <div key={labels[index]} className={cn("contents", index === 3 && "font-semibold")}><dt className="text-muted-foreground">{labels[index]}</dt><dd className="text-right tabular-nums">{value}</dd></div>)}</dl></div>)}</div>
    </>;
  };
  return <section className="space-y-3" aria-label={t("availableFunds.comparisonTitle")} aria-busy={busy}>
    <div className="flex items-center gap-2">
      <h2 className="font-semibold">{t("availableFunds.comparisonTitle")}</h2>
      <Badge variant="secondary">{baseCurrency}</Badge>
      <InfoPopover label={t("availableFunds.aboutAmounts")}>
        <div className="flex flex-col gap-2">
          <p>{t("availableFunds.cumulativeHint")}</p>
          <p>{t("availableFunds.currentFxEstimate")}</p>
            <p>{t("moneyDisplay.roundingHelp")}</p>
          {notes}
        </div>
      </InfoPopover>
    </div>
    <div className={cn("grid gap-3 transition-opacity [grid-template-columns:repeat(auto-fit,minmax(min(100%,13rem),1fr))]", busy && "opacity-70")} data-testid="funds-comparison" role="group" aria-label={t("availableFunds.selectHorizon")}>
      {horizons.map((horizon) => {
        const { label, bucket } = horizon;
        const active = selected === bucket.horizonOn;
        const previous = earlier(horizon);
        // A cue only compares complete totals; known subtotals can look equal while the real totals differ.
        const direction = previous && bucket.fullUnreserved && previous.bucket.fullUnreserved ? compareHorizonTotals(bucket.fullUnreserved, previous.bucket.fullUnreserved) : null;
        const early = includeEarly && horizonUsesEarly(sources, bucket.horizonOn);
        return <button
          key={bucket.horizonOn}
          type="button"
          aria-pressed={active}
          aria-controls="funds-currency-breakdown"
          onClick={() => onSelect(bucket.horizonOn)}
          data-testid={`horizon-${bucket.horizonOn}`}
          className={cn(
            "group flex min-w-0 flex-col gap-1 rounded-2xl border p-4 text-left shadow-sm transition-[border-color,background-color,box-shadow] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background",
            active ? "border-primary bg-primary/10 ring-1 ring-primary" : "border-border bg-card hover:border-primary/50 hover:bg-primary/5",
          )}
        >
          <span className="flex items-center justify-between gap-3">
            <span className="text-sm font-semibold">{label}</span>
            <span className={cn("flex size-5 shrink-0 items-center justify-center rounded-full", active ? "bg-primary text-primary-foreground" : "border border-border")}>
              {active && <Check className="size-3.5" aria-hidden="true" />}
            </span>
          </span>
          <span className="text-xs text-muted-foreground">{bucket.horizonOn}</span>
          <span className="mt-2 block break-words text-2xl font-bold tabular-nums tracking-tight" data-testid={`horizon-available-${bucket.horizonOn}`}>{amount(total(bucket))}</span>
          <span className="mt-1 flex min-h-5 flex-wrap items-center gap-x-2 gap-y-1 text-xs">
            {direction && previous && <ChangeText direction={direction} className="font-medium">
              {t(direction === "up" ? "availableFunds.moreThan" : direction === "down" ? "availableFunds.lessThan" : "availableFunds.sameAs", { label: previous.label })}
            </ChangeText>}
            {!bucket.fullUnreserved && <Badge variant="warning" title={t("availableFunds.incompleteAmount")}>{t("availableFunds.partialShort")}</Badge>}
            {early && <Badge variant="warning" className="gap-1"><Zap className="size-3" aria-hidden="true" />{t("availableFunds.earlyIncluded")}</Badge>}
          </span>
        </button>;
      })}
    </div>
    {selectedHorizon && <div id="funds-currency-breakdown" className="rounded-2xl border border-border bg-card p-4">
      <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
        <h3 className="text-sm font-semibold">{t("availableFunds.breakdownTitle")}</h3>
        <span className="text-xs text-muted-foreground">{selectedHorizon.label} · {selected}</span>
      </div>
      {nativeCurrencies.length ? breakdown() : <p className="text-sm text-muted-foreground">{unknown}</p>}
    </div>}
  </section>;
}

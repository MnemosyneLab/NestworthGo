import { useTranslation } from "react-i18next";
import { ArrowRight, Check } from "lucide-react";
import { formatAmount } from "@/lib/money";
import { cashVersusProceeds } from "./liquidityDisplay";
import type { LiquidityBucketDTO, LiquiditySourceDTO, NativeCurrencyGroupDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import type { MoneyView } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

type Horizon = { label: string; bucket: LiquidityBucketDTO };
type Total = LiquidityBucketDTO | NativeCurrencyGroupDTO;

export function FundsComparison({ horizons, selected, onSelect, sources, baseCurrency }: {
  horizons: Horizon[]; selected: string; onSelect: (date: string) => void;
  sources: LiquiditySourceDTO[]; baseCurrency: string;
}) {
  const { t } = useTranslation();
  const unknown = t("availableFunds.unknownAmount");
  const nativeCurrencies = [...new Set(horizons.flatMap(({ bucket }) => (bucket.nativeCurrencyGroups ?? []).map(group => group.currency)))].sort();
  const selectedHorizon = horizons.find(({ bucket }) => bucket.horizonOn === selected);
  const amount = (value: MoneyView | null | undefined, dashZero = false) => !value ? unknown : dashZero && /^0(?:\.0+)?$/.test(value.amount) ? "—" : formatAmount(value.amount, value.currency);
  const spendable = (total: Total | undefined) => <>
    <span className="font-semibold tabular-nums">{amount(total?.fullUnreserved ?? total?.knownUnreservedSubtotal)}</span>
    {!total?.fullUnreserved && <span className="mt-1 block text-xs font-normal text-muted-foreground">{t("availableFunds.incompleteAmount")}</span>}
  </>;
  const breakdown = () => {
    if (!selectedHorizon) return null;
    const bucket = selectedHorizon.bucket;
    const rowCurrencies = nativeCurrencies;
    const labels = [t("availableFunds.cashAccessible"), t("availableFunds.estimatedProceeds"), t("availableFunds.appliedReserve"), t("availableFunds.availableAfterReserve")];
    const rows = rowCurrencies.map(currency => {
      const total = bucket.nativeCurrencyGroups?.find(group => group.currency === currency);
      const matchingSources = sources.filter(source => source.nativeCurrency === currency);
      const parts = cashVersusProceeds(matchingSources, selected, unknown, currency, "native");
      const zero = formatAmount("0", currency);
      // Missing currency groups remain unknown, even if no source was returned.
      const values = [total ? parts.cash === zero ? "—" : parts.cash : unknown, total ? parts.proceeds === zero ? "—" : parts.proceeds : unknown, amount(total?.appliedReserveSubtotal, true), spendable(total)];
      return { currency, values };
    });
    return <>
      <table className="hidden w-full text-sm md:table">
        <caption className="sr-only">{t("availableFunds.breakdownTitle")}</caption>
        <thead><tr className="border-b border-border text-muted-foreground"><th scope="col" className="p-3 text-left font-medium">{t("availableFunds.currencyColumn")}</th>{labels.map(label => <th scope="col" key={label} className="p-3 text-right font-medium">{label}</th>)}</tr></thead>
        <tbody>{rows.map(row => <tr key={row.currency} className="border-b border-border/60 last:border-0"><th scope="row" className="p-3 text-left font-medium">{row.currency}</th>{row.values.map((value, index) => <td key={labels[index]} className={`p-3 text-right tabular-nums ${index === 3 ? "bg-primary/5" : ""}`}>{value}</td>)}</tr>)}</tbody>
      </table>
      <div className="divide-y divide-border md:hidden">{rows.map(row => <div key={row.currency} className="py-3"><h4 className="mb-2 font-semibold">{row.currency}</h4><dl className="grid grid-cols-2 gap-x-3 gap-y-2 text-sm">{row.values.map((value, index) => <div key={labels[index]} className="contents"><dt className="text-muted-foreground">{labels[index]}</dt><dd className="text-right tabular-nums">{value}</dd></div>)}</dl></div>)}</div>
    </>;
  };
  return <section className="space-y-5" aria-label={t("availableFunds.comparisonTitle")}>
    <div className="flex flex-wrap items-baseline justify-between gap-2">
      <h2 className="font-semibold">{t("availableFunds.comparisonTitle")} <span className="ml-2 text-sm font-normal text-muted-foreground">{baseCurrency}</span></h2>
      <p className="text-xs text-muted-foreground">{t("availableFunds.currentFxEstimate")}</p>
    </div>
    <div className="grid gap-3 sm:grid-cols-3" data-testid="funds-comparison" role="group" aria-label={t("availableFunds.selectHorizon")}>
      {horizons.map(({ label, bucket }) => {
        const active = selected === bucket.horizonOn;
        return <button
          key={bucket.horizonOn}
          type="button"
          aria-pressed={active}
          aria-controls="funds-currency-breakdown"
          onClick={() => onSelect(bucket.horizonOn)}
          data-testid={`horizon-${bucket.horizonOn}`}
          className={`group rounded-xl border p-5 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 ${active ? "border-primary bg-primary/10 ring-1 ring-primary" : "border-border bg-card hover:border-primary/50 hover:bg-primary/5"}`}
        >
          <span className="flex items-center justify-between gap-3">
            <span className="font-semibold">{label}</span>
            <span className={`flex h-6 w-6 items-center justify-center rounded-full ${active ? "bg-primary text-primary-foreground" : "border border-border text-muted-foreground group-hover:border-primary group-hover:text-primary"}`}>
              {active ? <Check className="h-4 w-4" aria-hidden="true" /> : <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />}
            </span>
          </span>
          <span className="mt-1 block text-xs text-muted-foreground">{bucket.horizonOn}</span>
          <span className="mt-4 block break-words text-2xl" data-testid={`horizon-available-${bucket.horizonOn}`}>{spendable(bucket)}</span>
          <span className={`mt-3 block text-xs ${active ? "font-medium text-primary" : "text-muted-foreground"}`}>
            {t(active ? "availableFunds.selectedHorizon" : "availableFunds.viewBreakdown")}
          </span>
        </button>;
      })}
    </div>
    <p className="text-xs text-muted-foreground">{t("availableFunds.cumulativeHint")}</p>
    {selectedHorizon && <div id="funds-currency-breakdown" className="rounded-xl border border-border bg-card p-4 md:p-5">
      <div className="mb-4 flex flex-wrap items-baseline justify-between gap-2">
        <h3 className="font-semibold">{t("availableFunds.breakdownTitle")}</h3>
        <span className="text-sm text-muted-foreground">{selectedHorizon.label} · {selected}</span>
      </div>
      {nativeCurrencies.length ? breakdown() : <p className="text-sm text-muted-foreground">{unknown}</p>}
    </div>}
  </section>;
}

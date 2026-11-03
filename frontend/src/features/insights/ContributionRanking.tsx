import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { analysisReason } from "./analysisText";
import { chartNumber } from "@/components/charts/chartTheme";
import { ChangeText } from "@/components/ui/change-text";
import { compareCanonical, formatSignedMoney } from "@/lib/money";
import type { ContributionRowDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export function rankedContributions(rows: ContributionRowDTO[], direction: string, limit: string): ContributionRowDTO[] {
  const ranked = rows.filter((row) => row.amount && (direction === "all" || (direction === "positive" ? compareCanonical(row.amount.amount, "0") > 0 : compareCanonical(row.amount.amount, "0") < 0)));
  ranked.sort((a, b) => compareCanonical(b.amount!.amount.replace(/^-/, ""), a.amount!.amount.replace(/^-/, "")) || a.key.localeCompare(b.key));
  return limit === "all" ? ranked : ranked.slice(0, Number(limit));
}

/** Numbers are geometry only; labels retain the authoritative canonical amounts. */
export function ContributionRanking({ rows, labelFor, onSelect, label }: { rows: ContributionRowDTO[]; labelFor: (row: ContributionRowDTO) => string; onSelect: (key: string) => void; label: string }) {
  const { t } = useTranslation();
  const maximum = Math.max(...rows.map((row) => Math.abs(chartNumber(row.amount?.amount) ?? 0)), 0);
  const maximumAmount = rows[0]?.amount?.amount.replace(/^-/, "") ?? "0";
  return <div role="list" aria-label={label} data-testid="contribution-ranking" className="divide-y divide-border/50">
    {rows.map((row) => {
      const value = chartNumber(row.amount?.amount) ?? 0;
      const width = maximum ? Math.abs(value) / maximum * 50 : 0;
      const negative = compareCanonical(row.amount!.amount, "0") < 0;
      return <div key={row.key} role="listitem"><button type="button" onClick={() => onSelect(row.key)} className="grid w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-1 rounded-sm py-2 text-left hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:grid-cols-[minmax(8rem,1fr)_minmax(0,3fr)_minmax(8rem,1fr)]">
        <span className="min-w-0 text-sm"><span className="block truncate" title={labelFor(row)}>{labelFor(row)}</span>{row.status === "partial" && <Badge variant="warning">{t("insights.partial")}</Badge>}{row.status === "unavailable" && <Badge variant="warning">{t("insights.status.unavailable")}</Badge>}{row.status !== "ok" && row.missingReason && <span className="block text-xs text-warning-foreground">{analysisReason(t, row.missingReason)}</span>}</span>
        <span aria-hidden="true" className="relative col-span-2 row-start-2 h-5 sm:col-span-1 sm:row-start-auto">
          <span className="absolute inset-y-0 left-1/2 w-px bg-muted-foreground/60" />
          <span data-testid="contribution-bar" data-amount={row.amount!.amount} className={`absolute top-0.5 h-4 rounded-sm ${negative ? "bg-gain-negative/75" : "bg-gain-positive/75"}`} style={{ width: `${width}%`, left: negative ? `${50 - width}%` : "50%" }} />
        </span>
        <ChangeText direction={negative ? "down" : value > 0 ? "up" : "flat"} className="col-start-2 row-start-1 justify-end whitespace-nowrap text-sm font-semibold num sm:col-start-3">{formatSignedMoney(row.amount!.amount, row.amount!.currency)}</ChangeText>
      </button></div>;
    })}
    <div aria-hidden="true" className="hidden grid-cols-[minmax(8rem,1fr)_minmax(0,3fr)_minmax(8rem,1fr)] gap-4 pt-2 text-xs text-muted-foreground sm:grid"><span /><span className="flex justify-between num"><span>{maximum ? formatSignedMoney(`-${maximumAmount}`, rows[0]!.amount!.currency) : "0"}</span><span>0</span><span>{maximum ? formatSignedMoney(maximumAmount, rows[0]!.amount!.currency) : "0"}</span></span></div>
  </div>;
}

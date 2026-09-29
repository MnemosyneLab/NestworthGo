import { useTranslation } from "react-i18next";
import type { AssetChangeSummaryDTO, SignedMoneyView } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { formatAmount } from "@/lib/money";
import { cn } from "@/lib/utils";
import { signToneClass } from "@/components/ui/money-text";

export function AssetValueSummary({ summary }: { summary: AssetChangeSummaryDTO }) {
  const { t } = useTranslation();
  const amount = (value?: SignedMoneyView | null, signed = false) => value ? `${signed && Number(value.amount) > 0 ? "+" : ""}${formatAmount(value.amount, value.currency)}` : "—";
  const rate = summary.changeRate == null ? null : Number(summary.changeRate) * 100;
  const rateText = rate != null && Number.isFinite(rate) ? `${rate > 0 ? "+" : ""}${rate.toFixed(2)}%` : "—";
  const reason = summary.changeRateMissingReason === "nonpositive_beginning" ? "changeRateNonpositive" : "changeRateIncomplete";
  const fields: Array<[string, string, string | undefined]> = [
    [t("insights.beginningValue"), amount(summary.beginningValue), undefined],
    [t("insights.endingValue"), amount(summary.endingValue), undefined],
    [t("insights.change"), amount(summary.change, true), signToneClass(summary.change?.amount)],
    [t("insights.changeRate"), rateText, signToneClass(summary.changeRate)],
  ];
  return <div className="space-y-3">
    <dl className="grid grid-cols-2 gap-4 lg:grid-cols-4">{fields.map(([label, value, tone]) => <div key={label}><dt className="text-sm text-muted-foreground">{label}</dt><dd className={cn("mt-1 text-xl font-semibold tabular-nums break-words", tone)}>{value}</dd></div>)}</dl>
    <p className="text-xs text-muted-foreground">{t("insights.changeRateHint")}{rate == null && <> {t(`insights.${reason}`)}</>}</p>
  </div>;
}

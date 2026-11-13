import { useTranslation } from "react-i18next";
import { AnalysisHealthLink } from "./AnalysisHealthLink";

type Coverage = {
  ratedDays: number;
  totalDays: number;
  missingValuationDays?: number;
  amountWithoutRateDays?: number;
};

export function ReturnCoverage({ data, rate, from, to }: {
  data: Coverage;
  rate?: string | null;
  from: string;
  to: string;
}) {
  const { t } = useTranslation();
  const { ratedDays, totalDays, missingValuationDays = 0, amountWithoutRateDays = 0 } = data;
  if (!(totalDays > 0)) return null;
  const otherDays = totalDays - ratedDays - missingValuationDays - amountWithoutRateDays;
  return <div className="flex flex-col items-start gap-1 text-sm text-muted-foreground">
    <p>{t(rate == null ? "insights.rateDaysUnavailable" : "insights.rateDaysUsed", { rated: ratedDays, total: totalDays })}</p>
    {(missingValuationDays > 0 || amountWithoutRateDays > 0 || otherDays > 0) && <ul className="list-disc space-y-0.5 pl-4">
      {missingValuationDays > 0 && <li>{t("insights.rateDaysMissingValuation", { count: missingValuationDays })}</li>}
      {amountWithoutRateDays > 0 && <li>{t("insights.rateDaysAmountOnly", { count: amountWithoutRateDays })}</li>}
      {otherDays > 0 && <li>{t("insights.rateDaysOtherGaps", { count: otherDays })}</li>}
    </ul>}
    {(missingValuationDays > 0 || otherDays > 0) && <AnalysisHealthLink from={from} to={to} />}
  </div>;
}

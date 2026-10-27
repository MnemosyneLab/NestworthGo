import { ReviewTruth } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { formatCivilDate } from "@/lib/time";
import { reviewLeafActual, reviewReasonLabel } from "./reviewEvidenceDisplay";
import type { ReviewDailyResult, ReviewDailyLeafEvidence, ReviewPredicateNode, ReviewRuleLeaf } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";

/** The persisted leaf order is depth first, with no boolean short circuit. The
 * backend attaches the exact condition version to reads, including old votes. */
export function ReviewEvidenceTree({ result }: { result: ReviewDailyResult }) {
  const { t } = useTranslation();
  let index = 0;
  const condition = (evidence: ReviewDailyLeafEvidence | undefined, definition?: ReviewRuleLeaf, ordinal = 0) => {
    const leaf = evidence ?? { metric: definition?.metric ?? "", truth: ReviewTruth.ReviewUnknown };
    const threshold = definition?.value ?? leaf.threshold;
    const operator = definition?.operator;
    const title = t(`reviewV2.metrics.${leaf.metric}`, { defaultValue: t("reviewV2.unknown") });
    const days = definition?.lookbackDays;
    const expected = definition?.types?.length ? definition.types.map(type => t(`reviewV2.types.${type}`)).join(", ")
      : definition?.instrumentIds?.length ? t("reviewV2.instrumentCount", { total: definition.instrumentIds.length })
      : threshold ? `${threshold}${leaf.metric === "annualized" || leaf.metric === "price_change" ? "%" : leaf.currency ? ` ${leaf.currency}` : ""}` : "";
    const truth = leaf.truth === "true" ? "True" : leaf.truth === "false" ? "False" : "Unknown";
    return <li key={ordinal} className="flex min-w-0 flex-col gap-1 rounded-lg bg-muted/40 p-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <p className="min-w-0 break-words font-medium"><span className="text-muted-foreground">{t("reviewV2.conditionNumber", { ordinal: ordinal + 1 })} · </span>{title}{days ? ` · ${t("reviewV2.conditionWindow", { days })}` : ""}</p>
        <Badge variant={leaf.truth === "true" ? "success" : "outline"}>{t(`reviewV2.condition${truth}`)}</Badge>
      </div>
      <p className="num break-words text-foreground">{operator && <span>{operator === "in" || operator === "not_in" ? t(operator === "in" ? "reviewV2.includesAny" : "reviewV2.excludesAll") : operator} {expected} · </span>}{t("reviewV2.observedValue")}: {reviewLeafActual(t, leaf)}</p>
      {(leaf.baselineOn || leaf.effectiveOn) && <p className="text-xs text-muted-foreground">{leaf.baselineOn ? `${formatCivilDate(leaf.baselineOn)} → ` : ""}{leaf.effectiveOn ? formatCivilDate(leaf.effectiveOn) : ""}</p>}
      {leaf.reason && <p className="text-xs text-muted-foreground">{reviewReasonLabel(t, leaf.reason)}</p>}
    </li>;
  };
  const tree = (node: ReviewPredicateNode, path: string): React.ReactNode => {
    if (node.leaf) {
      const ordinal = index++;
      return condition(result.leaves?.[ordinal], node.leaf, ordinal);
    }
    const all = Boolean(node.all);
    return <li key={path} className="min-w-0 border-l-2 border-border pl-3"><p className="mb-2 text-xs font-semibold text-muted-foreground">{t(all ? "reviewV2.allConditions" : "reviewV2.anyCondition")}</p><ul className="flex flex-col gap-2">{(node.all ?? node.any ?? []).map((child, n) => tree(child, `${path}.${n}`))}</ul></li>;
  };
  return <ul className="flex min-w-0 flex-col gap-2" aria-label={t("reviewV2.conditionEvidence")}>
    {result.predicate ? tree(result.predicate, "root") : (result.leaves ?? []).map((leaf, n) => condition(leaf, undefined, n))}
  </ul>;
}

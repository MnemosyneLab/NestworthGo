import { useId, useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/select";
import { Notice } from "@/components/ui/notice";
import { formatCivilDate, formatCivilDateRange } from "@/lib/time";
import { formatMoney } from "@/lib/money";
import { useReviewDailyEvidencePage } from "@/queries/investmentReviewV2";
import { ReviewEvidenceTree } from "./ReviewEvidenceTree";
import { reviewLeafActual, reviewReasonLabel } from "./reviewEvidenceDisplay";
import type { ReviewAlertRule, ReviewDailyResult, ReviewPredicateNode, ReviewRuleLeaf } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";

const isUnknown = (truth?: string) => truth !== "true" && truth !== "false";
const truthKey = (truth?: string) => `reviewV2.daily${truth === "true" ? "True" : truth === "false" ? "False" : "Unknown"}`;

function definitions(node?: ReviewPredicateNode | null): ReviewRuleLeaf[] {
  return node?.leaf ? [node.leaf] : (node?.all ?? node?.any ?? []).flatMap(definitions);
}

/** Format the engine's evidence; handling and daily predicate truth are independent. */
function ResultSummary({ result }: { result: ReviewDailyResult }) {
  const { t } = useTranslation();
  const leaves = definitions(result.predicate);
  const metrics = (result.leaves ?? []).map((leaf, index) => ({ leaf, definition: leaves[index] }))
    .filter(({ leaf }) => ["amount", "cost", "price_change", "annualized"].includes(leaf.metric));
  const reasons = [...new Set((result.leaves ?? []).flatMap(leaf => leaf.reason ? [leaf.reason] : []))];
  return <div className="min-w-0 space-y-2">
    <p className="num flex flex-wrap items-center gap-2"><span>{formatCivilDate(result.on)}</span><Badge variant={isUnknown(result.truth) ? "outline" : "secondary"}>{t(truthKey(result.truth))}</Badge></p>
    <dl className="space-y-1.5 text-xs">{metrics.map(({ leaf, definition }, index) => <div key={index} className="min-w-0">
      <dt className="break-words text-muted-foreground">{t(`reviewV2.metrics.${leaf.metric}`)}{definition?.lookbackDays ? ` · ${t("reviewV2.conditionWindow", { days: definition.lookbackDays })}` : ""}{definition?.value != null && definition.operator ? ` · ${definition.operator} ${reviewLeafActual(t, { ...leaf, actual: definition.value })}` : ""}</dt>
      <dd className="num break-words font-medium" title={leaf.actual}>{reviewLeafActual(t, leaf)}</dd>
      {leaf.baselineOn && leaf.effectiveOn && <dd className="num break-words text-muted-foreground">{formatCivilDateRange(leaf.baselineOn, leaf.effectiveOn)}</dd>}
    </div>)}</dl>
    {!metrics.length && <p className="text-xs text-muted-foreground">{t("reviewV2.noMetricValues")}</p>}
    {isUnknown(result.truth) && reasons.map(reason => <p key={reason} className="text-xs text-muted-foreground">{reviewReasonLabel(t, reason)}</p>)}
  </div>;
}

function DayDetails({ result }: { result: ReviewDailyResult }) {
  const { t } = useTranslation();
  return <section className="min-w-0 space-y-3 rounded-xl border border-border bg-card p-3" aria-label={t("reviewV2.selectedDayDetails")}>
    <p className="num flex flex-wrap items-center gap-2 font-semibold">{formatCivilDate(result.on)}<Badge variant="outline">{t(truthKey(result.truth))}</Badge></p>
    <dl className="num grid min-w-0 grid-cols-1 gap-2 text-xs sm:grid-cols-2">
      <div><dt className="text-muted-foreground">{t("reviewV2.amount")}</dt><dd className="break-words text-foreground" title={result.amountExact ?? undefined}>{result.amountExact != null && result.currency ? formatMoney(result.amountExact, result.currency) : t("reviewV2.unknown")}</dd></div>
      <div><dt className="text-muted-foreground">{t("reviewV2.cost")}</dt><dd className="break-words text-foreground" title={result.costExact ?? undefined}>{result.costExact != null && result.currency ? formatMoney(result.costExact, result.currency) : t("reviewV2.unknown")}</dd></div>
    </dl>
    <ReviewEvidenceTree result={result} />
  </section>;
}

export function ReviewEvidenceDetails({ rule, isActive, publication = "", onRefresh }: { rule: ReviewAlertRule; isActive: boolean; publication?: string; onRefresh?: () => void }) {
  const { t } = useTranslation();
  const id = useId();
  const [expanded, setExpanded] = useState(false);
  const [cursor, setCursor] = useState("");
  const [history, setHistory] = useState<string[]>([]);
  const [selectedOn, setSelectedOn] = useState("");
  const latest = rule.latestResult;
  const needsKnown = !latest || isUnknown(latest.truth);
  const scope = `${rule.episodeRevision}:${publication}`;
  // Known results are selected by the backend independently of the page cursor.
  // A valid latest assessment needs no extra read until history is opened.
  const first = useReviewDailyEvidencePage(rule.episodeId, "", isActive && (needsKnown || expanded), scope);
  const older = useReviewDailyEvidencePage(rule.episodeId, cursor, isActive && expanded && cursor !== "", scope);
  const page = cursor ? older : first;
  const matchesEpisode = first.data?.episodeId === rule.episodeId && first.data?.episodeRevision === rule.episodeRevision;
  const firstFailed = first.isError || Boolean(first.data && !matchesEpisode);
  const pageFailed = firstFailed || page.isError || Boolean(page.data && (page.data.episodeId !== rule.episodeId || page.data.episodeRevision !== rule.episodeRevision || page.data.reportRevision !== first.data?.reportRevision));
  const known = !firstFailed && matchesEpisode ? first.data?.latestKnownResult : undefined;
  const choices = !pageFailed ? [...(page.data?.items ?? [])] : [];
  if (!cursor && known && !choices.some(day => day.on === known.on)) choices.push(known);
  choices.sort((a, b) => b.on.localeCompare(a.on));
  const selected = choices.find(day => day.on === selectedOn) ?? choices[0];
  const retry = () => { setSelectedOn(""); setCursor(""); setHistory([]); onRefresh?.(); void first.refetch(); };
  return <div className="min-w-0 space-y-3">
    <div className="space-y-1 text-xs">
      <p className="font-semibold">{t("reviewV2.why")}</p>
      <p className="num break-words">{rule.triggerWindow ? t("reviewV2.triggerSummary", { triggered: formatCivilDate(rule.triggeredOn), hits: rule.triggerWindow.hitCount, range: formatCivilDateRange(rule.triggerWindow.windowStart, rule.triggerWindow.windowEnd) }) : t("reviewV2.alertOrigin", { triggered: formatCivilDate(rule.triggeredOn), first: formatCivilDate(rule.firstHitOn) })}</p>
    </div>
    <div className={`grid min-w-0 gap-3 rounded-xl bg-muted/30 p-3 ${needsKnown ? "sm:grid-cols-2" : ""}`}>
      <div className="min-w-0 space-y-2"><p className="text-xs font-semibold text-muted-foreground">{t("reviewV2.latestAssessment")}</p>{latest ? <ResultSummary result={latest} /> : <p className="text-xs">{t("reviewV2.assessmentUnavailable")}</p>}</div>
      {needsKnown && <div className="min-w-0 space-y-2"><p className="text-xs font-semibold text-muted-foreground">{t("reviewV2.latestKnown")}</p>{firstFailed ? <div className="space-y-2"><p role="alert" className="text-xs">{t("reviewV2.evidenceLoadError")}</p><Button size="sm" variant="outline" disabled={first.isFetching} onClick={retry}>{t("reviewV2.refresh")}</Button></div> : known ? <ResultSummary result={known} /> : <p role={first.isPending ? "status" : undefined} className="text-xs">{t(first.isPending ? "ui.state.loadingPage" : "reviewV2.noKnownResult")}</p>}</div>}
    </div>
    {needsKnown && <p className="text-xs text-muted-foreground">{t("reviewV2.unknownDoesNotRecover")}</p>}
    <div className="min-w-0 rounded-xl border border-border text-xs">
      <Button variant="ghost" size="sm" className="h-auto w-full justify-start whitespace-normal px-3 py-2 text-left" aria-expanded={expanded} aria-controls={`${id}-history`} onClick={() => setExpanded(value => !value)}>{expanded ? <ChevronDown aria-hidden="true" /> : <ChevronRight aria-hidden="true" />}{t("reviewV2.dayHistory")}</Button>
      {expanded && <div id={`${id}-history`} className="min-w-0 space-y-3 border-t border-border p-3">
        <p className="text-muted-foreground">{t("reviewV2.historySelectionHelp")}</p>
        <p className="num text-muted-foreground">{t("reviewV2.windowCounts", { hits: rule.window.hitCount, falseCount: rule.window.falseCount, unknowns: rule.window.unknownCount })}</p>
        {(page.isPending || page.isFetching) && <p role="status">{t("ui.state.loadingPage")}</p>}
        {pageFailed && <Notice tone="warning" role="alert"><p>{t("reviewV2.evidenceLoadError")}</p><Button size="sm" variant="outline" disabled={first.isFetching} onClick={retry}>{t("reviewV2.refresh")}</Button></Notice>}
        {!pageFailed && !!choices.length && <><label htmlFor={id} className="block font-medium">{t("reviewV2.selectDay")}</label><NativeSelect id={id} className="w-full max-w-full" disabled={page.isFetching} value={selected?.on ?? ""} onChange={event => setSelectedOn(event.target.value)}>{choices.map(day => <option key={day.on} value={day.on}>{formatCivilDate(day.on)} · {t(truthKey(day.truth))}</option>)}</NativeSelect>{selected && <DayDetails result={selected} />}</>}
        {!page.isPending && !pageFailed && !choices.length && <p>{t("reviewV2.noDailyEvidence")}</p>}
        {!pageFailed && page.data && <p className="text-muted-foreground">{t("reviewV2.conditionVersion", { version: page.data.conditionVersion })}</p>}
        <div className="flex flex-wrap justify-end gap-2"><Button size="sm" variant="outline" disabled={!history.length || page.isFetching} onClick={() => { setSelectedOn(""); setCursor(history[history.length - 1] ?? ""); setHistory(values => values.slice(0, -1)); }}>{t("reviewV2.previous")}</Button><Button size="sm" variant="outline" disabled={!page.data?.nextCursor || page.isFetching || pageFailed} onClick={() => { setSelectedOn(""); setHistory(values => [...values, cursor]); setCursor(page.data!.nextCursor ?? ""); }}>{t("reviewV2.next")}</Button></div>
        {(rule.coverage ?? []).map((code, i) => <p key={i} className="text-muted-foreground">{reviewReasonLabel(t, code)}</p>)}
      </div>}
    </div>
  </div>;
}

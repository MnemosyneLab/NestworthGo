import { useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { InvestmentReviewItem, InvestmentReviewPolicy } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import { useInvestmentReviewActions, useInvestmentReviews } from "@/queries/investmentReview";
import { useObjectNavigation } from "@/app/NavigationContext";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/select";

// The application report owns eligibility and episodes; this only orders its pending actions.
function isPendingReview(item: InvestmentReviewItem) {
  return Boolean(item.episodeId && !item.acknowledged && !item.snoozed);
}
function compareNames(a: InvestmentReviewItem, b: InvestmentReviewItem) {
  return a.displayName.localeCompare(b.displayName) || a.instrumentId.localeCompare(b.instrumentId);
}
function comparePending(a: InvestmentReviewItem, b: InvestmentReviewItem) {
  return Number(b.status === "loss") - Number(a.status === "loss") || compareNames(a, b);
}
const summaryClass = "cursor-pointer rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";
const actionClass = "h-auto min-h-9 max-w-full whitespace-normal text-left";

export function InvestmentReviewSummary({ isActive = true }: { isActive?: boolean }) {
  const { t } = useTranslation();
  const query = useInvestmentReviews(isActive);
  const navigation = useObjectNavigation();
  const items = query.data?.items ?? [];
  const count = items.filter(isPendingReview).length;
  return <section aria-label={t("investmentReview.title")} className="border-t pt-3 space-y-2">
    <h3 className="font-medium">{t("investmentReview.title")}</h3>
    {query.isPending && <p role="status">{t("ui.state.loadingPage")}</p>}
    {query.isError && <p role="alert">{t(query.data ? "investmentReview.staleRead" : "investmentReview.error")}</p>}
    {query.data && <>
      <p>{t("investmentReview.summary", { count })}</p>
      {!query.data.policy.enabled ? <p className="text-sm">{t("investmentReview.disabled")}</p>
        : items.length > 0 && !items.some(item => item.monitored) && <p className="text-sm">{t("investmentReview.setup")}</p>}
    </>}
    <p className="text-xs text-muted-foreground">{t("investmentReview.foreground")}</p>
    {navigation && <Button className={actionClass} variant="link" onClick={() => navigation.open({ page: "upcoming-actions" })}>{t("investmentReview.view")}</Button>}
  </section>;
}

export function InvestmentReviewPanel({ isActive = true }: { isActive?: boolean }) {
  const { t } = useTranslation();
  const query = useInvestmentReviews(isActive);
  const { acknowledge, enroll, snooze } = useInvestmentReviewActions();
  const navigation = useObjectNavigation();
  const [showAll, setShowAll] = useState(false);
  const allButton = useRef<HTMLButtonElement>(null);
  const data = query.data;
  const items = data?.items ?? [];
  const pending = items.filter(isPendingReview).sort(comparePending);
  const other = items.filter(item => !isPendingReview(item)).sort(compareNames);
  const monitored = items.filter(item => item.monitored);
  const insufficient = monitored.some(item => item.status === "unavailable" || item.status === "stale");
  const writesDisabled = query.isError || query.isFetching || acknowledge.isPending || enroll.isPending || snooze.isPending;
  const chooseInvestments = () => { setShowAll(true); allButton.current?.focus(); };
  const enrollment = (item: InvestmentReviewItem) => <label className="flex items-start gap-2 text-sm">
    <input type="checkbox" className="mt-1 shrink-0" checked={item.monitored} disabled={writesDisabled} onChange={event => enroll.mutate({ instrumentId: item.instrumentId, enabled: event.target.checked, expectedRevision: data!.revision })} />
    {t("investmentReview.enroll", { name: item.displayName })}
  </label>;
  const renderItem = (item: InvestmentReviewItem) => {
    if (!data) return null;
    // Keep the existing handling precedence separate from the authoritative condition.
    const handling = !item.monitored ? "not_enabled" : item.snoozed ? "snoozed" : !data.policy.enabled ? "paused" : item.acknowledged && item.episodeId ? "acknowledged" : "monitoring";
    return <li key={item.instrumentId} className="min-w-0 rounded-lg border border-border p-3 space-y-3 break-words">
      <h5 className="font-medium">{item.displayName}</h5>
      <div className="flex flex-wrap gap-2">
        <Badge className="max-w-full whitespace-normal" variant={item.status === "loss" ? "destructive" : item.status === "low_yield" || item.status === "stale" ? "warning" : "outline"}>
          <span className="sr-only">{t("investmentReview.condition")}: </span>{t(`investmentReview.status.${item.status}`)}
        </Badge>
        <Badge className="max-w-full whitespace-normal" variant="secondary">
          <span className="sr-only">{t("investmentReview.handling")}: </span>{t(`investmentReview.status.${handling}`)}
        </Badge>
      </div>
      {item.annualizedPercent != null && <p className="text-sm">{t("investmentReview.estimated", { value: item.annualizedPercent })}</p>}
      {item.reason && <p className="text-sm text-muted-foreground">{t(`investmentReview.reason.${item.reason}`)}</p>}
      {item.snoozed && <p className="text-sm">{t("investmentReview.resumeDate", { date: item.snoozedUntilOn, timezone: data.timezone })}</p>}
      <div className="flex flex-wrap gap-2">
        {navigation && <Button className={actionClass} variant="outline" aria-label={t("investmentReview.checkNav", { name: item.displayName })} onClick={() => navigation.open({ page: "market-data", focus: { instrumentId: item.instrumentId, label: item.displayName } })}>{t("investmentReview.navAction")}</Button>}
        {isPendingReview(item) && <Button className={actionClass} disabled={writesDisabled} aria-label={t("investmentReview.acknowledge", { name: item.displayName })} onClick={() => acknowledge.mutate(item.episodeId)}>{t("investmentReview.reviewedAction")}</Button>}
      </div>
      {showAll && enrollment(item)}
      <details>
        <summary className={summaryClass}>{t("investmentReview.controls", { name: item.displayName })}</summary>
        <div className="space-y-3 pt-3">
          {!showAll && enrollment(item)}
          <div className="flex flex-wrap gap-2">
            {item.monitored && [7, 30].map(days => <Button key={days} className={actionClass} variant="outline" disabled={writesDisabled} aria-label={t("investmentReview.snooze", { days, name: item.displayName })} onClick={() => snooze.mutate({ instrumentId: item.instrumentId, days, expectedRevision: data.revision })}>{t("investmentReview.snoozeAction", { days })}</Button>)}
            {item.snoozed && <Button className={actionClass} variant="outline" disabled={writesDisabled} aria-label={t("investmentReview.resumeNow", { name: item.displayName })} onClick={() => snooze.mutate({ instrumentId: item.instrumentId, days: 0, expectedRevision: data.revision })}>{t("investmentReview.resumeAction")}</Button>}
            {item.monitored && <Button className={actionClass} variant="outline" disabled={writesDisabled} aria-label={t("investmentReview.ignoreIndefinitely", { name: item.displayName })} onClick={() => enroll.mutate({ instrumentId: item.instrumentId, enabled: false, expectedRevision: data.revision })}>{t("investmentReview.disableAction")}</Button>}
          </div>
        </div>
      </details>
      <details>
        <summary className={summaryClass}>{t("investmentReview.calculation", { name: item.displayName })}</summary>
        <div className="space-y-2 pt-3 text-xs text-muted-foreground">
          {item.latestOn ? <p>{t("investmentReview.dates", { latest: item.latestOn, baseline: item.baselineOn || t("investmentReview.missing"), window: data.policy.windowDays, days: item.baselineOn ? item.elapsedDays : t("investmentReview.missing"), age: item.ageDays })}</p>
            : <p>{t("investmentReview.noDates", { window: data.policy.windowDays })}</p>}
          {item.targetOn && <p>{t("investmentReview.target", { date: item.targetOn })}</p>}
          {item.periodPercent != null && <p>{t("investmentReview.period", { value: item.periodPercent })}</p>}
          {item.observationCount > 0 && <p>{t("investmentReview.observations", { count: item.observationCount, missing: item.missingDays })}</p>}
          <p>{t("investmentReview.priceOnly")}</p>
        </div>
      </details>
    </li>;
  };
  return <section aria-label={t("investmentReview.title")} className="min-w-0 rounded-xl border border-border p-4 space-y-3 break-words" aria-busy={query.isFetching}>
    <div className="flex flex-wrap items-center justify-between gap-2">
      <h3 className="text-lg font-semibold">{t("investmentReview.title")}</h3>
      <Button variant="outline" disabled={query.isFetching} onClick={() => { void query.refetch(); }}>{t("investmentReview.refresh")}</Button>
    </div>
    <p className="text-sm text-muted-foreground">{t("investmentReview.scope")}</p>
    <p className="text-xs text-muted-foreground">{t("investmentReview.foreground")}</p>
    {!data && query.isPending && <p role="status">{t("ui.state.loadingPage")}</p>}
    {query.isError && <p role="alert">{t(data ? "investmentReview.staleRead" : "investmentReview.error")}</p>}
    {(acknowledge.isError || enroll.isError || snooze.isError) && <p role="alert">{t("investmentReview.writeError")}</p>}
    {data && <>
      <p className="text-xs text-muted-foreground">{t("investmentReview.evaluated", { date: data.localDate, timezone: data.timezone })}</p>
      <p className="text-sm">{t("investmentReview.currentPolicy", { window: data.policy.windowDays, low: data.policy.lowAnnualizedPercent, loss: data.policy.lossPercent, sustain: data.policy.sustainDays, age: data.policy.maxAgeDays })}</p>
      {!data.policy.enabled && <p role="status">{t("investmentReview.disabled")}</p>}
      {items.length === 0 ? <p>{t("investmentReview.empty")}</p> : <>
        {monitored.length === 0 && <div className="rounded-lg bg-muted p-3 space-y-2">
          <p>{t("investmentReview.setup")}</p>
          <Button className={actionClass} variant="outline" onClick={chooseInvestments}>{t("investmentReview.choose")}</Button>
        </div>}
        <div role="group" aria-label={t("investmentReview.listView")} className="flex flex-wrap gap-2">
          <Button className={actionClass} variant={!showAll ? "default" : "outline"} aria-pressed={!showAll} onClick={() => setShowAll(false)}>{t("investmentReview.pending", { count: pending.length })}</Button>
          <Button ref={allButton} className={actionClass} variant={showAll ? "default" : "outline"} aria-pressed={showAll} onClick={() => setShowAll(true)}>{t("investmentReview.all", { count: items.length })}</Button>
        </div>
        <h4 className="font-medium">{t("investmentReview.pending", { count: pending.length })}</h4>
        {pending.length > 0 ? <ul aria-label={t("investmentReview.pending", { count: pending.length })} className="space-y-3">{pending.map(renderItem)}</ul>
          : monitored.length > 0 && data.policy.enabled && <p>{t(insufficient ? "investmentReview.insufficient" : "investmentReview.noPending")}</p>}
        {showAll && other.length > 0 && <>
          <h4 className="font-medium">{t("investmentReview.other")}</h4>
          <ul aria-label={t("investmentReview.other")} className="space-y-3">{other.map(renderItem)}</ul>
        </>}
      </>}
      <ReviewPolicyForm key={JSON.stringify(data.policy)} policy={data.policy} revision={data.revision} disabled={writesDisabled} />
    </>}
  </section>;
}
function ReviewPolicyForm({ policy, revision, disabled }: { policy: InvestmentReviewPolicy; revision: number; disabled: boolean }) {
  const { t } = useTranslation();
  const { save } = useInvestmentReviewActions();
  const [value, setValue] = useState({ ...policy });
  const id = useId();
  const fields = ["lowAnnualizedPercent", "lossPercent", "sustainDays", "maxAgeDays"] as const;
  return <details>
    <summary className={summaryClass}>{t("investmentReview.settings")}</summary>
    <form className="space-y-3 pt-3 max-w-lg" onSubmit={event => { event.preventDefault(); save.mutate({ policy: value, expectedRevision: revision }); }}>
      <fieldset disabled={disabled || save.isPending} className="space-y-3">
        <label className="flex gap-2"><input type="checkbox" checked={value.enabled} onChange={event => setValue({ ...value, enabled: event.target.checked })} />{t("investmentReview.enable")}</label>
        <label htmlFor={`${id}-window`}>{t("investmentReview.window")}</label>
        <NativeSelect className="block max-w-full" id={`${id}-window`} value={value.windowDays} onChange={event => setValue({ ...value, windowDays: Number(event.target.value) })}>
          {[30, 7].map(days => <option key={days} value={days}>{t("investmentReview.windowDays", { days })}</option>)}
        </NativeSelect>
        {fields.map(field => <div key={field}>
          <label htmlFor={`${id}-${field}`}>{t(`investmentReview.fields.${field}`)}</label>
          <Input id={`${id}-${field}`} required type="number" step={field.endsWith("Percent") ? "0.00000001" : "1"} min={field === "lossPercent" ? -50 : field === "sustainDays" ? 2 : field === "maxAgeDays" ? 1 : 0} max={field === "lossPercent" ? -0.00000001 : field === "lowAnnualizedPercent" ? 10 : 30} value={value[field]} onChange={event => setValue({ ...value, [field]: field.endsWith("Percent") ? event.target.value : Number(event.target.value) })} />
        </div>)}
        <p className="text-xs">{t("investmentReview.policyNote")}</p>
        <Button type="submit">{t("common.save")}</Button>
      </fieldset>
      {save.isError && <p role="alert">{t("investmentReview.writeError")}</p>}
      {save.isSuccess && <p role="status">{t("investmentReview.saved")}</p>}
    </form>
  </details>;
}

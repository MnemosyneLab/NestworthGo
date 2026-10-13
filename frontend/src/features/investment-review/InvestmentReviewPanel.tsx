import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import type { InvestmentReviewPolicy } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import { useInvestmentReviewActions, useInvestmentReviews } from "@/queries/investmentReview";
import { useObjectNavigation } from "@/app/NavigationContext";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/select";

export function InvestmentReviewSummary({ isActive = true }: { isActive?: boolean }) {
  const { t } = useTranslation();
  const query = useInvestmentReviews(isActive);
  const navigation = useObjectNavigation();
  const count = (query.data?.items ?? []).filter(item => item.episodeId && !item.acknowledged && !item.snoozed).length;
  return <section aria-label={t("investmentReview.title")} className="border-t pt-3 space-y-2">
    <h3 className="font-medium">{t("investmentReview.title")}</h3>
    {query.isPending && <p role="status">{t("ui.state.loadingPage")}</p>}
    {query.isError && <p role="alert">{t(query.data ? "investmentReview.staleRead" : "investmentReview.error")}</p>}
    {query.data && <p>{t("investmentReview.summary", { count })}</p>}
    <p className="text-xs text-muted-foreground">{t("investmentReview.foreground")}</p>
    {navigation && <Button variant="link" onClick={() => navigation.open({ page: "upcoming-actions" })}>{t("investmentReview.view")}</Button>}
  </section>;
}

export function InvestmentReviewPanel({ isActive = true }: { isActive?: boolean }) {
  const { t } = useTranslation();
  const query = useInvestmentReviews(isActive);
  const { acknowledge, enroll, snooze } = useInvestmentReviewActions();
  const navigation = useObjectNavigation();
  const data = query.data;
  return <section aria-label={t("investmentReview.title")} className="rounded-xl border p-4 space-y-3" aria-busy={query.isFetching}>
    <h3 className="text-lg font-semibold">{t("investmentReview.title")}</h3>
    <p className="text-sm text-muted-foreground">{t("investmentReview.scope")}</p>
    <p className="text-sm text-muted-foreground">{t("investmentReview.foreground")}</p>
    <p className="text-xs text-muted-foreground">{t("investmentReview.priceOnly")}</p>
    <Button variant="outline" disabled={query.isFetching} onClick={() => { void query.refetch(); }}>{t("investmentReview.refresh")}</Button>
    {!data && query.isPending && <p role="status">{t("ui.state.loadingPage")}</p>}
    {query.isError && <p role="alert">{t(data ? "investmentReview.staleRead" : "investmentReview.error")}</p>}
    {(acknowledge.isError || enroll.isError || snooze.isError) && <p role="alert">{t("investmentReview.writeError")}</p>}
    {data && <>
      <p className="text-xs">{t("investmentReview.evaluated", { date: data.localDate, timezone: data.timezone })}</p>
      <ReviewPolicyForm key={JSON.stringify(data.policy)} policy={data.policy} revision={data.revision} disabled={query.isError || query.isFetching || snooze.isPending || enroll.isPending} />
      {!data.policy.enabled && <p>{t("investmentReview.disabled")}</p>}
      {(data.items ?? []).length === 0 && <p>{t("investmentReview.empty")}</p>}
      <ul className="space-y-3">{(data.items ?? []).map(item => <li key={item.instrumentId} className="rounded-lg border p-3 space-y-2">
        <h4 className="font-medium">{item.displayName}</h4>
        <label className="flex gap-2"><input type="checkbox" checked={item.monitored} disabled={query.isError || query.isFetching || enroll.isPending || snooze.isPending} onChange={event => enroll.mutate({ instrumentId: item.instrumentId, enabled: event.target.checked, expectedRevision: data.revision })} />{t("investmentReview.enroll", { name: item.displayName })}</label>
        <p>{t(`investmentReview.status.${!item.monitored ? "not_enabled" : item.snoozed ? "snoozed" : !data.policy.enabled ? "paused" : item.acknowledged && item.episodeId ? "acknowledged" : item.status}`)}</p>
        {item.snoozed && <p>{t("investmentReview.resumeDate", { date: item.snoozedUntilOn, timezone: data.timezone })}</p>}
        {item.latestOn && <p>{t("investmentReview.dates", { latest: item.latestOn, baseline: item.baselineOn || t("investmentReview.missing"), window: data.policy.windowDays, days: item.baselineOn ? item.elapsedDays : t("investmentReview.missing"), age: item.ageDays })}</p>}
        {!item.latestOn && <p>{t("investmentReview.noDates", { window: data.policy.windowDays })}</p>}
        {item.targetOn && <p className="text-xs">{t("investmentReview.target", { date: item.targetOn })}</p>}
        {item.annualizedPercent != null && <>
          <p>{t("investmentReview.estimated", { value: item.annualizedPercent })}</p>
          <p className="text-sm">{t("investmentReview.period", { value: item.periodPercent })}</p>
        </>}
        {item.observationCount > 0 && <p className="text-sm">{t("investmentReview.observations", { count: item.observationCount, missing: item.missingDays })}</p>}
        {item.reason && <p>{t(`investmentReview.reason.${item.reason}`)}</p>}
        <div className="flex flex-wrap gap-2">
          {navigation && <Button variant="outline" onClick={() => navigation.open({ page: "market-data", focus: { instrumentId: item.instrumentId, label: item.displayName } })}>{t("investmentReview.checkNav", { name: item.displayName })}</Button>}
          {item.episodeId && !item.acknowledged && !item.snoozed && <Button disabled={query.isError || query.isFetching || acknowledge.isPending} onClick={() => acknowledge.mutate(item.episodeId)}>{t("investmentReview.acknowledge", { name: item.displayName })}</Button>}
          {item.monitored && [7, 30].map(days => <Button key={days} variant="outline" disabled={query.isError || query.isFetching || snooze.isPending || enroll.isPending} onClick={() => snooze.mutate({ instrumentId: item.instrumentId, days, expectedRevision: data.revision })}>{t("investmentReview.snooze", { days, name: item.displayName })}</Button>)}
          {item.snoozed && <Button variant="outline" disabled={query.isError || query.isFetching || snooze.isPending || enroll.isPending} onClick={() => snooze.mutate({ instrumentId: item.instrumentId, days: 0, expectedRevision: data.revision })}>{t("investmentReview.resumeNow", { name: item.displayName })}</Button>}
          {item.monitored && <Button variant="outline" disabled={query.isError || query.isFetching || enroll.isPending || snooze.isPending} onClick={() => enroll.mutate({ instrumentId: item.instrumentId, enabled: false, expectedRevision: data.revision })}>{t("investmentReview.ignoreIndefinitely", { name: item.displayName })}</Button>}
        </div>
      </li>)}</ul>
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
    <summary className="cursor-pointer">{t("investmentReview.settings")}</summary>
    <form className="space-y-3 pt-3" onSubmit={event => { event.preventDefault(); save.mutate({ policy: value, expectedRevision: revision }); }}>
      <fieldset disabled={disabled || save.isPending} className="space-y-3">
        <label className="flex gap-2"><input type="checkbox" checked={value.enabled} onChange={event => setValue({ ...value, enabled: event.target.checked })} />{t("investmentReview.enable")}</label>
        <label htmlFor={`${id}-window`}>{t("investmentReview.window")}</label>
        <NativeSelect className="block" id={`${id}-window`} value={value.windowDays} onChange={event => setValue({ ...value, windowDays: Number(event.target.value) })}>
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

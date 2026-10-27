import { ReviewEvidenceTree } from "./ReviewEvidenceTree";
import { ReviewPriority } from "./ReviewPriority";
import { createPortal } from "react-dom";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { FieldError } from "@/components/ui/field-error";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/select";
import { DatePicker } from "@/components/ui/date-picker";
import { ActionMenu } from "@/components/ui/action-menu";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Notice } from "@/components/ui/notice";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { LoadingState, ErrorState, EmptyState } from "@/components/layout/PageState";
import { useObjectNavigation } from "@/app/NavigationContext";
import { useReviewAlertPage, useReviewCoveragePage, useReviewDailyEvidencePage, useReviewRulesV2, useReviewSuppressionsV2, useReviewV2Commands } from "@/queries/investmentReviewV2";
import { formatCivilDate, formatCivilDateRange } from "@/lib/time";
import { useHistoryOrigin } from "@/queries/history";
import { useInstruments } from "@/queries/investments";
import { instrumentDisplayLabel, trimInstrumentText } from "@/lib/instrumentDisplay";
import { localDateInTimeZone } from "@/features/history/historyStartDate";
import { parseWailsError, translateWailsError, type WireError } from "@/lib/wails";
import type { ReviewAlertGroup, ReviewAlertRule, ReviewRule } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import { ReviewRuleEditor } from "./ReviewRuleEditor";
import { ReviewEpisodeHistorySheet } from "./ReviewEpisodeHistorySheet";
import { reviewReasonLabel, reviewReasonList } from "./reviewEvidenceDisplay";

const statusKeys = ["unignored", "all", "pending", "acknowledged", "ignored", "unknown", "confirming", "paused", "coverage"] as const;
const typeKeys = ["all", "stock", "etf", "mutual_fund", "bank_investment_product", "bond", "precious_metal", "crypto", "other"] as const;

const detailsClass = "rounded-xl border border-border bg-card/60 text-xs text-muted-foreground";
const detailsSummaryClass = "cursor-pointer select-none rounded-xl px-3 py-2 font-medium text-foreground hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50";

function CursorPager({ canPrevious, canNext, onPrevious, onNext, label }: { canPrevious: boolean; canNext: boolean; onPrevious: () => void; onNext: () => void; label: { previous: string; next: string } }) {
  if (!canPrevious && !canNext) return null;
  return <div className="flex items-center justify-end gap-2"><Button variant="outline" size="sm" disabled={!canPrevious} onClick={onPrevious}>{label.previous}</Button><Button variant="outline" size="sm" disabled={!canNext} onClick={onNext}>{label.next}</Button></div>;
}

function nextCivilDate(value: string | undefined): string | undefined {
  if (!value || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return undefined;
  const date = new Date(`${value}T00:00:00Z`);
  if (Number.isNaN(date.getTime()) || date.toISOString().slice(0, 10) !== value) return undefined;
  date.setUTCDate(date.getUTCDate() + 1);
  return date.toISOString().slice(0, 10);
}

function ruleWindow(rule: ReviewAlertRule, t: (key: string, options?: Record<string, unknown>) => string) {
  const policy = rule.triggerPolicy;
  if (policy.kind === "consecutive") return t("reviewV2.windowConsecutive", { days: rule.window.consecutiveCount, needed: policy.count });
  if (policy.kind === "rolling") return t("reviewV2.windowRolling", { hits: rule.window.hitCount, needed: policy.minHits, days: policy.windowDays, unknowns: rule.window.unknownCount });
  return t("reviewV2.windowSingle", { date: formatCivilDate(rule.window.windowEnd) });
}

function SuppressedInstrumentName({ instrumentId }: { instrumentId: string }) {
  const { t } = useTranslation();
  const instruments = useInstruments(true);
  const instrument = (instruments.data ?? []).find(item => item.id === instrumentId);
  const label = instrument ? trimInstrumentText(instrument.name) || instrumentDisplayLabel(instrument) : "";
  return <>{label || (instruments.isPending ? "…" : t("reviewV2.unknownInvestment"))}</>;
}

function ReviewEvidenceDetails({ rule, isActive }: { rule: ReviewAlertRule; isActive: boolean }) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const [cursor, setCursor] = useState("");
  const [history, setHistory] = useState<string[]>([]);
  const evidence = useReviewDailyEvidencePage(rule.episodeId, cursor, isActive && expanded);
  return <details className={detailsClass} onToggle={event => setExpanded(event.currentTarget.open)}>
    <summary className={detailsSummaryClass}>{t("reviewV2.why")}</summary>
    <div className="flex flex-col gap-2 border-t border-border px-3 py-3">
      <p className="num">{t("reviewV2.evidenceDates", { first: formatCivilDate(rule.firstHitOn), triggered: formatCivilDate(rule.triggeredOn), latest: formatCivilDate(rule.latestClosedOn) })}</p>
      <p className="num">{t("reviewV2.windowCounts", { hits: rule.window.hitCount, falseCount: rule.window.falseCount, unknowns: rule.window.unknownCount })}</p>
      {rule.latestResult && <><p className="font-medium">{t("reviewV2.ruleResult")}: {t(`reviewV2.daily${rule.latestResult.truth === "true" ? "True" : rule.latestResult.truth === "false" ? "False" : "Unknown"}`)}</p><ReviewEvidenceTree result={rule.latestResult} /></>}
      {(rule.coverage ?? []).map((code, i) => <p key={i}>{reviewReasonLabel(t, code)}</p>)}
      {evidence.isPending && <p role="status">{t("ui.state.loadingPage")}</p>}
      {evidence.isError && <Notice tone="warning" role="alert"><div className="flex flex-wrap items-center justify-between gap-2"><span>{t("reviewV2.staleRead")}</span><Button size="sm" variant="outline" onClick={() => { setCursor(""); setHistory([]); }}>{t("reviewV2.refresh")}</Button></div></Notice>}
      {evidence.data && <><ol className="flex flex-col gap-1.5" aria-label={t("reviewV2.dailyEvidence")}>
        {(evidence.data.items ?? []).map(day => <li key={day.on} className={`num flex flex-col gap-0.5 rounded-lg border px-3 py-2 ${day.truth === "true" ? "border-warning/40 bg-warning/5" : "border-border bg-card"}`}><span className="font-semibold text-foreground">{formatCivilDate(day.on)} · {t(`reviewV2.daily${day.truth === "true" ? "True" : day.truth === "false" ? "False" : "Unknown"}`)}</span><ReviewEvidenceTree result={day} /></li>)}
      </ol><div className="flex justify-end gap-2"><Button size="sm" variant="outline" disabled={!history.length || evidence.isFetching} onClick={() => { setCursor(history[history.length - 1] ?? ""); setHistory(values => values.slice(0, -1)); }}>{t("reviewV2.previous")}</Button><Button size="sm" variant="outline" disabled={!evidence.data.nextCursor || evidence.isFetching} onClick={() => { setHistory(values => [...values, cursor]); setCursor(evidence.data!.nextCursor ?? ""); }}>{t("reviewV2.next")}</Button></div></>}
    </div>
  </details>;
}

export function InvestmentReviewPage({ isActive = true, actionsHost }: { isActive?: boolean; actionsHost?: HTMLElement | null } = {}) {
  const { t } = useTranslation();
  const navigation = useObjectNavigation();
  const historyOrigin = useHistoryOrigin();
  const householdTimezone = historyOrigin.data?.timezone ?? "";
  const householdToday = householdTimezone ? localDateInTimeZone(householdTimezone) : undefined;
  const firstResumeDate = nextCivilDate(householdToday);
  const [status, setStatus] = useState("unignored");
  const [instrumentType, setInstrumentType] = useState("all");
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState("");
  const [cursorStack, setCursorStack] = useState<string[]>([]);
  const [rulesOpen, setRulesOpen] = useState(false);
  const [suppressionsOpen, setSuppressionsOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [editing, setEditing] = useState<ReviewRule | "new" | null>(null);
  const [archiveFor, setArchiveFor] = useState<ReviewRule | null>(null);
  const [permanent, setPermanent] = useState<ReviewAlertGroup | null>(null);
  const [dateFor, setDateFor] = useState<ReviewAlertGroup | null>(null);
  const [untilOn, setUntilOn] = useState("");
  const [untilError, setUntilError] = useState("");
  const [message, setMessage] = useState("");
  const statusRoot = useRef<HTMLDivElement>(null);
  const focusReturn = useRef<{ element: Element | null; instrumentId: string; revision: string } | null>(null);
  const filter = useMemo(() => ({ status, instrumentType: instrumentType === "all" ? "" : instrumentType, query }), [status, instrumentType, query]);
  const coverageMode = status === "unknown" || status === "confirming" || status === "paused" || status === "coverage";
  const coverageFilter = useMemo(() => ({ status: status === "coverage" ? "all" : status, instrumentType: filter.instrumentType, query }), [status, filter.instrumentType, query]);
  const alerts = useReviewAlertPage(filter, cursor, isActive && !coverageMode);
  const coverage = useReviewCoveragePage(coverageFilter, cursor, isActive && coverageMode);
  const rules = useReviewRulesV2(isActive);
  const suppressions = useReviewSuppressionsV2(isActive && suppressionsOpen);
  const commands = useReviewV2Commands();
  const reconcileAsync = commands.reconcile.mutateAsync;
  const reconciling = useRef(false);
  const queuedReconcile = useRef(false);
  const activated = useRef(false);
  const seenRules = useRef("");
  useEffect(() => {
    if (!isActive) { activated.current = false; seenRules.current = ""; return; }
    const ruleSignature = (rules.data ?? []).map(rule => `${rule.id}:${rule.rowRevision}`).join("|");
    const zones = [...new Set((rules.data ?? []).filter(rule => rule.evaluator === "daily_predicate_v1" && rule.enabled && !rule.archivedAt).map(rule => rule.spec.schedule.timezone))];
    const daySignature = () => zones.map(zone => {
      try { return `${zone}:${new Intl.DateTimeFormat("en-CA", { timeZone: zone, year: "numeric", month: "2-digit", day: "2-digit" }).format(new Date())}`; }
      catch { return `${zone}:invalid`; }
    }).join("|");
    let signature = daySignature();
    const refresh = () => {
      if (reconciling.current) { queuedReconcile.current = true; return; }
      reconciling.current = true;
      void reconcileAsync().catch(() => undefined).finally(() => {
        reconciling.current = false;
        if (queuedReconcile.current) { queuedReconcile.current = false; refresh(); }
      });
    };
    if (!activated.current || seenRules.current !== ruleSignature) { activated.current = true; seenRules.current = ruleSignature; refresh(); }
    const checkDay = () => { const next = daySignature(); if (next !== signature) { signature = next; refresh(); } };
    const onFocus = () => { if (document.visibilityState === "visible") refresh(); };
    window.addEventListener("focus", onFocus);
    document.addEventListener("visibilitychange", onFocus);
    const timer = window.setInterval(checkDay, 60_000);
    return () => { window.removeEventListener("focus", onFocus); document.removeEventListener("visibilitychange", onFocus); window.clearInterval(timer); };
  }, [isActive, rules.data, reconcileAsync]);
  const busy = commands.acknowledge.isPending || commands.suppress.isPending || commands.clearSuppression.isPending || commands.reconcile.isPending || commands.archiveRule.isPending;
  const resetCursor = () => { setCursor(""); setCursorStack([]); };
  const handle = async (action: () => Promise<unknown>, success: string) => {
    try { await action(); setMessage(success); resetCursor(); }
    catch (cause) { focusReturn.current = null; setMessage(String(cause)); }
  };
  const ignore = (group: ReviewAlertGroup, mode: "permanent" | "until_date", days = 0, date = "") => { trackFocus(group.instrumentId); return handle(() => commands.suppress.mutateAsync({ instrumentId: group.instrumentId, mode, ...(days ? { days } : {}), ...(date ? { untilOn: date } : {}) }), t("reviewV2.ignoredMessage", { name: group.displayName })); };
  const openInstrument = (instrumentId: string, section: "holdings" | "performance") => navigation?.open({ page: "instrument-detail", instrumentId, section });
  const content = alerts.data;
  const items = content?.items ?? [];
  const trackFocus = (instrumentId: string) => { focusReturn.current = { element: document.activeElement, instrumentId, revision: content?.reportRevision ?? "" }; };
  const saveIgnoreUntil = async () => {
    if (!dateFor || !untilOn || !firstResumeDate || busy) return;
    trackFocus(dateFor.instrumentId);
    setMessage("");
    setUntilError("");
    if (untilOn < firstResumeDate) {
      setUntilError(t("reviewV2.ignoreUntilFutureDate", { timezone: householdTimezone }));
      return;
    }
    try {
      await commands.suppress.mutateAsync({ instrumentId: dateFor.instrumentId, mode: "until_date", untilOn });
      setMessage(t("reviewV2.ignoredMessage", { name: dateFor.displayName }));
      resetCursor();
      setDateFor(null);
    } catch (cause) {
      const wire = (cause as Error & { wireError?: WireError } | null)?.wireError ?? parseWailsError(cause);
      setUntilError(wire.code === "validation" && wire.field === "untilOn"
        ? t("reviewV2.ignoreUntilFutureDate", { timezone: householdTimezone })
        : translateWailsError(wire));
    }
  };
  useLayoutEffect(() => {
    const pending = focusReturn.current;
    if (!pending || !content || pending.revision === content.reportRevision) return;
    focusReturn.current = null;
    if (document.activeElement !== pending.element && document.activeElement !== document.body) return;
    const row = Array.from(document.querySelectorAll<HTMLElement>("[data-review-id]")).find(node => node.dataset.reviewId === pending.instrumentId);
    const target = row?.querySelector<HTMLElement>("[data-review-heading]") ?? statusRoot.current?.querySelector<HTMLElement>('[aria-pressed="true"]');
    target?.focus();
  }, [content]);
  const list = <section className="flex min-w-0 flex-col gap-4" aria-busy={alerts.isFetching}>
    {(content?.projectionStatus && content.projectionStatus !== "complete" || content?.summary || content?.publishedOn) && <Card className="flex flex-col gap-2 p-5">
      {content?.summary && <p className="num text-base font-semibold" role="status">{t("reviewV2.summaryCounts", { total: content.summary.totalGroups, matched: content.summary.matchedGroups, pending: content.summary.pendingGroups, actionable: content.summary.actionableRules, unknown: content.summary.unknownGroups })}</p>}
      {content?.publishedOn && <p className="num text-xs text-muted-foreground">{t("reviewV2.publishedOn", { date: formatCivilDate(content.publishedOn) })}</p>}
      {content?.projectionStatus && content.projectionStatus !== "complete" && <Notice tone="warning" role="status">{t("reviewV2.projectionStatus", { status: content.projectionStatus })}</Notice>}
    </Card>}
    {!items.length && !alerts.isFetching && <EmptyState variant={query || status !== "unignored" || instrumentType !== "all" ? "empty" : "success"} title={t(query || status !== "unignored" || instrumentType !== "all" ? "reviewV2.noFilterResults" : "reviewV2.noAlerts")} />}
    <ul className="flex flex-col gap-4">{items.map(group => <li key={group.instrumentId} data-review-id={group.instrumentId} className="min-w-0 overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border bg-muted/30 px-5 py-4"><div className="min-w-0"><h2 data-review-heading tabIndex={-1} className="break-words text-base font-semibold">{group.displayName}</h2><p className="num text-xs text-muted-foreground">{t(`reviewV2.types.${group.instrumentType}`)} · {group.currency}</p></div>
      <div className="flex items-center gap-2"><Badge variant={group.status === "pending" ? "warning" : "secondary"}>{t(`reviewV2.status.${group.status}`)}</Badge><ActionMenu label={t("reviewV2.moreFor", { name: group.displayName })} items={[
        { id: "ignore7", label: t("reviewV2.ignoreDays", { days: 7 }), onSelect: () => void ignore(group, "until_date", 7), disabled: busy || alerts.isError || group.status === "ignored" },
        { id: "ignore30", label: t("reviewV2.ignoreDays", { days: 30 }), onSelect: () => void ignore(group, "until_date", 30), disabled: busy || alerts.isError || group.status === "ignored" },
        { id: "ignoreUntil", label: t("reviewV2.ignoreUntil"), onSelect: () => { setDateFor(group); setUntilOn(""); setUntilError(""); }, disabled: busy || alerts.isError || group.status === "ignored" },
        { id: "ignorePermanent", label: t("reviewV2.ignorePermanent"), onSelect: () => setPermanent(group), disabled: busy || alerts.isError || group.status === "ignored" },
        { id: "holdings", label: t("reviewV2.viewHoldings"), onSelect: () => openInstrument(group.instrumentId, "holdings") },
        { id: "performance", label: t("reviewV2.viewPerformance"), onSelect: () => openInstrument(group.instrumentId, "performance") },
      ]} /></div></div>
      <div className="divide-y divide-border">{(group.rules ?? []).map(rule => <div key={rule.episodeId} className={`flex flex-col gap-2 px-5 py-4 text-sm ${rule.actionable ? "bg-warning/5" : ""}`}>
        <div className="flex flex-wrap items-center justify-between gap-2"><span className="flex min-w-0 flex-wrap items-center gap-2 font-semibold"><ReviewPriority severity={rule.severity} /><span className="break-words">{rule.name}</span></span><Badge variant={rule.actionable ? "warning" : rule.status === "acknowledged" || rule.status === "ignored" ? "secondary" : "outline"}>{t(`reviewV2.status.${rule.status}`)}</Badge></div>
        <p className="num">{ruleWindow(rule, t)}{rule.window.unknownCount > 0 && rule.triggerPolicy.kind !== "rolling" ? ` · ${t("reviewV2.unknownDays", { count: rule.window.unknownCount })}` : ""}</p>
        {rule.suppressedUntil && <p className="num text-xs text-muted-foreground">{t("reviewV2.suppressedUntil", { date: formatCivilDate(rule.suppressedUntil) })}</p>}
        {rule.actionable && <div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" disabled={busy || alerts.isError} onClick={() => { trackFocus(group.instrumentId); void handle(() => commands.acknowledge.mutateAsync({ episodeId: rule.episodeId, expectedRevision: rule.episodeRevision }), t("reviewV2.acknowledgedMessage", { name: rule.name })); }}>{t("reviewV2.acknowledge")}</Button>
        </div>}
        <ReviewEvidenceDetails rule={rule} isActive={isActive} />
      </div>)}</div>
    </li>)}</ul>
    <CursorPager label={{ previous: t("reviewV2.previous"), next: t("reviewV2.next") }} canPrevious={cursorStack.length > 0} canNext={Boolean(content?.nextCursor)} onPrevious={() => { setCursor(cursorStack[cursorStack.length - 1] ?? ""); setCursorStack(values => values.slice(0, -1)); }} onNext={() => { setCursorStack(values => [...values, cursor]); setCursor(content!.nextCursor ?? ""); }} />
  </section>;
  const coveragePage = coverage.data;
  const coverageList = <section className="flex min-w-0 flex-col gap-4" aria-busy={coverage.isFetching}>
    {coveragePage?.projectionStatus && coveragePage.projectionStatus !== "complete" && <Notice tone="warning" role="status">{t("reviewV2.projectionStatus", { status: coveragePage.projectionStatus })}</Notice>}
    {!coveragePage?.items?.length && !coverage.isFetching && <EmptyState title={t("reviewV2.noCoverage")} />}
    <ul className="flex flex-col gap-4">{(coveragePage?.items ?? []).map(group => <li key={group.instrumentId} className="min-w-0 overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border bg-muted/30 px-5 py-4"><div className="min-w-0"><h2 className="break-words text-base font-semibold">{group.displayName}</h2><p className="text-xs text-muted-foreground">{t(`reviewV2.types.${group.instrumentType}`)}</p></div><Badge variant={group.status === "unknown" || group.status === "uncomputed" ? "warning" : "secondary"}>{t(`reviewV2.status.${group.status}`)}</Badge></div>
      <div className="flex flex-col gap-3 px-5 py-4">
      {(group.rules ?? []).map(rule => <div key={rule.ruleId} className="flex flex-col gap-1 rounded-xl bg-muted/40 px-3 py-2.5 text-sm"><p className="font-semibold">{rule.name} · {t(`reviewV2.status.${rule.state}`)}</p><p className="num">{t("reviewV2.coverageWindow", { range: formatCivilDateRange(rule.window.windowStart, rule.latestClosedOn), hits: rule.window.hitCount, unknowns: rule.window.unknownCount })}</p>{(rule.missing ?? []).length > 0 && <p className="text-xs text-muted-foreground">{t("reviewV2.coverageReason", { reasons: reviewReasonList(t, rule.missing ?? []) })}</p>}</div>)}
      {!group.rules?.length && <p className="text-sm text-muted-foreground">{t("reviewV2.uncomputedHelp")}</p>}
      {navigation && <Button size="sm" variant="outline" className="self-start" onClick={() => openInstrument(group.instrumentId, "performance")}>{t("reviewV2.viewPerformance")}</Button>}
      </div>
    </li>)}</ul>
    <CursorPager label={{ previous: t("reviewV2.previous"), next: t("reviewV2.next") }} canPrevious={cursorStack.length > 0} canNext={Boolean(coveragePage?.nextCursor)} onPrevious={() => { setCursor(cursorStack[cursorStack.length - 1] ?? ""); setCursorStack(values => values.slice(0, -1)); }} onNext={() => { setCursorStack(values => [...values, cursor]); setCursor(coveragePage!.nextCursor ?? ""); }} />
  </section>;
  const reviewActions = <><Button variant="outline" disabled={busy || !isActive} onClick={() => { setMessage(""); void handle(() => commands.reconcile.mutateAsync(), t("reviewV2.refreshed")); }}>{t("reviewV2.refresh")}</Button><Button variant="outline" disabled={!isActive} onClick={() => setHistoryOpen(true)}>{t("reviewV2.historyTitle")}</Button><Button variant="outline" disabled={!isActive} onClick={() => setSuppressionsOpen(true)}>{t("reviewV2.ignoredList")}</Button><Button disabled={!isActive} onClick={() => setRulesOpen(true)}>{t("reviewV2.rules")}</Button></>;
  return <div className="flex min-w-0 flex-col gap-5">{actionsHost ? createPortal(reviewActions, actionsHost) : actionsHost === undefined ? <div className="flex flex-wrap gap-2">{reviewActions}</div> : null}
    <Card className="flex flex-wrap items-center gap-3 p-4"><div ref={statusRoot} className="min-w-0 max-w-full"><SegmentedControl label={t("reviewV2.statusFilter")} value={status} onChange={value => { setStatus(value); resetCursor(); }} options={statusKeys.map(value => ({ value, label: t(`reviewV2.status.${value}`) }))} /></div><NativeSelect aria-label={t("reviewV2.typeFilter")} className="w-full sm:w-auto" value={instrumentType} onChange={event => { setInstrumentType(event.target.value); resetCursor(); }}>{typeKeys.map(value => <option key={value} value={value}>{value === "all" ? t("reviewV2.allTypes") : t(`reviewV2.types.${value}`)}</option>)}</NativeSelect><Input aria-label={t("reviewV2.search")} placeholder={t("reviewV2.search")} className="w-full sm:max-w-52" value={query} onChange={event => { setQuery(event.target.value); resetCursor(); }} /></Card>
    {message && <Notice role="status">{message}</Notice>}
    {commands.reconcile.isError && <ErrorState title={t("reviewV2.loadError")} description={t("ui.state.errorDescription")} onRetry={() => commands.reconcile.mutateAsync().catch(() => undefined)} retryLabel={t("common.retryAction")} />}
    {coverageMode ? !coveragePage && coverage.isPending ? <LoadingState label={t("ui.state.loadingPage")} variant="list" /> : !coveragePage && coverage.isError ? <ErrorState title={t("reviewV2.loadError")} description={t("ui.state.errorDescription")} onRetry={() => void coverage.refetch()} retryLabel={t("common.retryAction")} /> : <>{coverage.isError && <Notice tone="warning" role="alert"><div className="flex flex-wrap items-center justify-between gap-2"><span>{t("reviewV2.staleRead")}</span>{cursor && <Button size="sm" variant="outline" onClick={resetCursor}>{t("reviewV2.refresh")}</Button>}</div></Notice>}{coverageList}</> : !content && alerts.isPending ? <LoadingState label={t("ui.state.loadingPage")} variant="list" /> : !content && alerts.isError ? <ErrorState title={t("reviewV2.loadError")} description={t("ui.state.errorDescription")} onRetry={() => void alerts.refetch()} retryLabel={t("common.retryAction")} /> : <>{alerts.isError && <Notice tone="warning" role="alert"><div className="flex flex-wrap items-center justify-between gap-2"><span>{t("reviewV2.staleRead")}</span>{cursor && <Button size="sm" variant="outline" onClick={() => { resetCursor(); setMessage(t("reviewV2.cursorExpired")); }}>{t("reviewV2.refresh")}</Button>}</div></Notice>}{list}</>}
    <Sheet open={rulesOpen} onOpenChange={setRulesOpen}><SheetContent size="lg"><SheetHeader><SheetTitle>{t("reviewV2.rules")}</SheetTitle></SheetHeader><Button onClick={() => { setRulesOpen(false); setEditing("new"); }}>{t("reviewV2.newRule")}</Button>{rules.isError && <Notice tone="danger" role="alert">{t("reviewV2.loadError")}</Notice>}<ul className="flex flex-col gap-3">{(rules.data ?? []).filter(rule => !rule.archivedAt).map(rule => <li key={rule.id} className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4"><div className="flex items-center justify-between gap-2"><span className="flex min-w-0 flex-wrap items-center gap-2 font-semibold"><ReviewPriority severity={rule.severity} /><span className="break-words">{rule.name}</span></span><Badge variant={rule.enabled ? "success" : "outline"}>{t(rule.enabled ? "reviewV2.enabled" : "reviewV2.paused")}</Badge></div><p className="text-xs text-muted-foreground">{t("reviewV2.ruleSummary", { mode: t(`reviewV2.${rule.spec.triggerPolicy.kind}`), timezone: rule.spec.schedule.timezone, count: leavesCount(rule) })}</p><div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" onClick={() => { setRulesOpen(false); setEditing(rule); }}>{t("reviewV2.editRule")}</Button><Button size="sm" variant="outline" disabled={busy} onClick={() => void handle(() => commands.updateRule.mutateAsync({ ruleId: rule.id, expectedRevision: rule.rowRevision, name: rule.name, enabled: !rule.enabled, severity: rule.severity, spec: rule.spec }), t("reviewV2.saved"))}>{t(rule.enabled ? "reviewV2.pause" : "reviewV2.enable")}</Button><Button size="sm" variant="ghost" disabled={busy} onClick={() => setArchiveFor(rule)}>{t("reviewV2.archive")}</Button></div></li>)}</ul>{rules.hasNextPage && <Button variant="outline" disabled={rules.isFetchingNextPage} onClick={() => void rules.fetchNextPage()}>{t("reviewV2.loadMore")}</Button>}{!rules.isPending && !rules.data?.length && <p className="text-sm text-muted-foreground">{t("reviewV2.noRules")}</p>}</SheetContent></Sheet>
    {editing && <ReviewRuleEditor key={editing === "new" ? "new" : `${editing.id}-${editing.rowRevision}`} open onOpenChange={open => { if (!open) setEditing(null); }} rule={editing === "new" ? null : editing} />}
    {historyOpen && <ReviewEpisodeHistorySheet open isActive={isActive} onOpenChange={setHistoryOpen} />}
    <Sheet open={suppressionsOpen} onOpenChange={setSuppressionsOpen}><SheetContent size="lg"><SheetHeader><SheetTitle>{t("reviewV2.ignoredList")}</SheetTitle></SheetHeader>{suppressions.isError && <Notice tone="danger" role="alert">{t("reviewV2.loadError")}</Notice>}<ul className="flex flex-col gap-3">{(suppressions.data ?? []).map(item => <li key={item.ID} className="num flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-card p-4 text-sm"><span className="min-w-0 break-words"><SuppressedInstrumentName instrumentId={item.InstrumentID} /> · {item.Mode === "permanent" ? t("reviewV2.ignorePermanent") : t("reviewV2.suppressedUntil", { date: formatCivilDate(item.UntilOn) })}</span><Button size="sm" variant="outline" disabled={busy} onClick={() => void handle(() => commands.clearSuppression.mutateAsync({ suppressionId: item.ID, expectedRevision: item.Revision }), t("reviewV2.resumed"))}>{t("reviewV2.resume")}</Button></li>)}</ul>{suppressions.hasNextPage && <Button variant="outline" disabled={suppressions.isFetchingNextPage} onClick={() => void suppressions.fetchNextPage()}>{t("reviewV2.loadMore")}</Button>}{!suppressions.isPending && !suppressions.data?.length && <p className="text-sm text-muted-foreground">{t("reviewV2.noSuppressions")}</p>}</SheetContent></Sheet>
    <AlertDialog open={archiveFor !== null} onOpenChange={open => { if (!open) setArchiveFor(null); }}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{t("reviewV2.archive")}</AlertDialogTitle><AlertDialogDescription>{t("reviewV2.archiveConfirm", { name: archiveFor?.name ?? "" })}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel><Button disabled={busy} onClick={() => { if (archiveFor) void handle(() => commands.archiveRule.mutateAsync({ ruleId: archiveFor.id, expectedRevision: archiveFor.rowRevision }), t("reviewV2.archived")); setArchiveFor(null); }}>{t("reviewV2.archive")}</Button></AlertDialogFooter></AlertDialogContent></AlertDialog>
    <AlertDialog open={permanent !== null} onOpenChange={open => { if (!open) setPermanent(null); }}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{t("reviewV2.ignorePermanent")}</AlertDialogTitle><AlertDialogDescription>{t("reviewV2.permanentConfirm", { name: permanent?.displayName ?? "" })}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel><Button disabled={busy} onClick={() => { if (permanent) void ignore(permanent, "permanent"); setPermanent(null); }}>{t("reviewV2.ignorePermanent")}</Button></AlertDialogFooter></AlertDialogContent></AlertDialog>
    <Sheet open={dateFor !== null} onOpenChange={open => { if (!open) { setDateFor(null); setUntilError(""); } }}><SheetContent size="md"><SheetHeader><SheetTitle>{t("reviewV2.ignoreUntil")}</SheetTitle></SheetHeader>
      <DatePicker aria-label={t("reviewV2.resumeOn")} aria-invalid={Boolean(untilError)} aria-describedby={untilError ? "review-ignore-until-help review-ignore-until-error" : "review-ignore-until-help"} value={untilOn} min={firstResumeDate} onChange={value => { setUntilOn(value); setUntilError(""); }} allowFuture />
      <p id="review-ignore-until-help" className="text-sm text-muted-foreground">{t("reviewV2.ignoreUntilDateHelp", { timezone: householdTimezone || "…" })}</p>
      <FieldError id="review-ignore-until-error" className="text-sm">{untilError}</FieldError>
      <Button disabled={!untilOn || !firstResumeDate || untilOn < firstResumeDate || busy} onClick={() => void saveIgnoreUntil()}>{t("reviewV2.saveIgnore")}</Button>
    </SheetContent></Sheet>
  </div>;
}

function leavesCount(rule: ReviewRule) { return (rule.spec.predicate.all ?? rule.spec.predicate.any ?? []).length; }

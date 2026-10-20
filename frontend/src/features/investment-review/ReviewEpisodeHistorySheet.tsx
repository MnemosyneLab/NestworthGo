import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Notice } from "@/components/ui/notice";
import { NativeSelect } from "@/components/ui/select";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useReviewEpisodeEventPage, useReviewEpisodeHistoryPage, useReviewRulesV2 } from "@/queries/investmentReviewV2";
import type { ReviewEpisodeHistoryItem } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";

function eventWindow(payloadJSON: string): { hitCount: number; falseCount: number; unknownCount: number } | null {
  try {
    const value: unknown = JSON.parse(payloadJSON);
    if (!value || typeof value !== "object") return null;
    const window = (value as { Window?: unknown }).Window;
    if (!window || typeof window !== "object") return null;
    const counts = window as Record<string, unknown>;
    return typeof counts.hitCount === "number" && typeof counts.falseCount === "number" && typeof counts.unknownCount === "number"
      ? { hitCount: counts.hitCount, falseCount: counts.falseCount, unknownCount: counts.unknownCount } : null;
  } catch { return null; }
}

function EpisodeEvents({ episodeId, isActive }: { episodeId: string; isActive: boolean }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [cursor, setCursor] = useState("");
  const [previous, setPrevious] = useState<string[]>([]);
  const events = useReviewEpisodeEventPage(episodeId, cursor, open && isActive);
  return <details onToggle={event => setOpen(event.currentTarget.open)} className="rounded-lg border border-border bg-muted/30 text-sm">
    <summary className="cursor-pointer select-none rounded-lg px-3 py-1.5 font-medium text-foreground hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50">{t("reviewV2.historyEvents")}</summary>
    <div className="space-y-2 px-3 pb-3">
    {events.isPending && <p role="status" className="animate-pulse text-muted-foreground">{t("ui.state.loadingPage")}</p>}
    {events.isError && <Notice tone="warning" role="alert"><div className="flex flex-wrap items-center justify-between gap-2"><span>{t("reviewV2.staleRead")}</span><Button size="sm" variant="outline" onClick={() => { setCursor(""); setPrevious([]); void events.refetch(); }}>{t("reviewV2.refresh")}</Button></div></Notice>}
    {events.data && <><ol className="space-y-1.5">{(events.data.items ?? []).map(item => {
      const window = eventWindow(item.payloadJson);
      return <li key={item.id} className="rounded-lg border border-border bg-card p-2"><span className="font-medium">{t(`reviewV2.historyEvent.${item.kind}`)}</span><span className="num ml-2 text-muted-foreground">{item.createdAt}</span>{window && <p className="num text-muted-foreground">{t("reviewV2.windowCounts", { hits: window.hitCount, falseCount: window.falseCount, unknowns: window.unknownCount })}</p>}</li>;
    })}</ol>
      <div className="flex justify-end gap-2 pt-1"><Button size="sm" variant="outline" aria-label={t("reviewV2.previousEventPage")} disabled={!previous.length || events.isFetching} onClick={() => { setCursor(previous[previous.length - 1] ?? ""); setPrevious(values => values.slice(0, -1)); }}>{t("reviewV2.previous")}</Button><Button size="sm" variant="outline" aria-label={t("reviewV2.nextEventPage")} disabled={!events.data.nextCursor || events.isFetching} onClick={() => { setPrevious(values => [...values, cursor]); setCursor(events.data!.nextCursor ?? ""); }}>{t("reviewV2.next")}</Button></div>
    </>}
    </div>
  </details>;
}

function EpisodeRow({ item, isActive }: { item: ReviewEpisodeHistoryItem; isActive: boolean }) {
  const { t } = useTranslation();
  const state = item.resolvedAt ? t("reviewV2.historyResolved", { reason: t(`reviewV2.historyReason.${item.resolvedReason}`) }) : item.acknowledgedAt ? t("reviewV2.status.acknowledged") : t("reviewV2.status.pending");
  const stateVariant = item.resolvedAt ? "success" : item.acknowledgedAt ? "secondary" : "warning";
  return <li className="flex flex-col gap-2 rounded-2xl border border-border bg-card p-4 text-sm shadow-sm">
    <div className="flex flex-wrap items-start justify-between gap-2"><span className="min-w-0 break-words font-semibold">{item.instrumentName}</span><Badge variant={stateVariant}>{state}</Badge></div>
    <p className="num">{t("reviewV2.historyTriggered", { date: item.triggeredOn, hits: item.hitCount, from: item.windowStart, to: item.windowEnd })}</p>
    <p className="num text-xs text-muted-foreground">{t("reviewV2.historyVersion", { version: item.openedConditionVersion, detected: item.firstDetectedAt })}</p>
    {item.acknowledgedAt && <p className="num text-xs text-muted-foreground">{t("reviewV2.historyAcknowledged", { at: item.acknowledgedAt })}</p>}
    {item.resolvedAt && <p className="num text-xs text-muted-foreground">{t("reviewV2.historyClosed", { at: item.resolvedAt })}</p>}
    <EpisodeEvents episodeId={item.episodeId} isActive={isActive} />
  </li>;
}

export function ReviewEpisodeHistorySheet({ open, isActive = true, onOpenChange }: { open: boolean; isActive?: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation();
  const rules = useReviewRulesV2(open && isActive);
  const dailyRules = (rules.data ?? []).filter(rule => rule.evaluator === "daily_predicate_v1");
  const [chosenRule, setChosenRule] = useState("");
  const ruleId = chosenRule || dailyRules[0]?.id || "";
  const [cursor, setCursor] = useState("");
  const [previous, setPrevious] = useState<string[]>([]);
  const history = useReviewEpisodeHistoryPage(ruleId, cursor, open && isActive);
  const changeRule = (id: string) => { setChosenRule(id); setCursor(""); setPrevious([]); };
  return <Sheet open={open} onOpenChange={onOpenChange}><SheetContent size="lg"><SheetHeader><SheetTitle>{t("reviewV2.historyTitle")}</SheetTitle></SheetHeader>
    {rules.isError && <Notice tone="danger" role="alert">{t("reviewV2.loadError")}</Notice>}
    {!!dailyRules.length && <NativeSelect aria-label={t("reviewV2.historyRule")} value={ruleId} onChange={event => changeRule(event.target.value)}>{dailyRules.map(rule => <option key={rule.id} value={rule.id}>{rule.name}</option>)}</NativeSelect>}
    {rules.hasNextPage && <Button size="sm" variant="outline" disabled={rules.isFetchingNextPage} onClick={() => void rules.fetchNextPage()}>{t("reviewV2.loadMore")}</Button>}
    {!dailyRules.length && !rules.isPending && <p className="text-sm text-muted-foreground">{t("reviewV2.noRules")}</p>}
    {history.isPending && ruleId && <p role="status" className="animate-pulse text-sm text-muted-foreground">{t("ui.state.loadingPage")}</p>}
    {history.isError && <Notice tone="warning" role="alert"><div className="flex flex-wrap items-center justify-between gap-2"><span>{t("reviewV2.staleRead")}</span><Button size="sm" variant="outline" onClick={() => { setCursor(""); setPrevious([]); void history.refetch(); }}>{t("reviewV2.refresh")}</Button></div></Notice>}
    {history.data && <><ul className="flex flex-col gap-3">{(history.data.items ?? []).map(item => <EpisodeRow key={item.episodeId} item={item} isActive={isActive} />)}</ul>
      {!history.data.items?.length && <p className="rounded-xl border border-dashed border-border px-4 py-6 text-center text-sm text-muted-foreground">{t("reviewV2.historyEmpty")}</p>}
      <div className="flex justify-end gap-2 pt-1"><Button variant="outline" disabled={!previous.length || history.isFetching} onClick={() => { setCursor(previous[previous.length - 1] ?? ""); setPrevious(values => values.slice(0, -1)); }}>{t("reviewV2.previous")}</Button><Button variant="outline" disabled={!history.data.nextCursor || history.isFetching} onClick={() => { setPrevious(values => [...values, cursor]); setCursor(history.data!.nextCursor ?? ""); }}>{t("reviewV2.next")}</Button></div>
    </>}
  </SheetContent></Sheet>;
}

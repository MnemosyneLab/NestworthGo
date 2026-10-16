import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
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

function EpisodeEvents({ episodeId }: { episodeId: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [cursor, setCursor] = useState("");
  const [previous, setPrevious] = useState<string[]>([]);
  const events = useReviewEpisodeEventPage(episodeId, cursor, open);
  return <details onToggle={event => setOpen(event.currentTarget.open)} className="text-sm">
    <summary className="cursor-pointer py-1">{t("reviewV2.historyEvents")}</summary>
    {events.isPending && <p role="status">{t("ui.state.loadingPage")}</p>}
    {events.isError && <p role="alert">{t("reviewV2.staleRead")} <Button size="sm" variant="outline" onClick={() => { setCursor(""); setPrevious([]); void events.refetch(); }}>{t("reviewV2.refresh")}</Button></p>}
    {events.data && <><ol className="space-y-1">{(events.data.items ?? []).map(item => {
      const window = eventWindow(item.payloadJson);
      return <li key={item.id} className="rounded border p-2"><span className="font-medium">{t(`reviewV2.historyEvent.${item.kind}`)}</span><span className="ml-2 text-muted-foreground">{item.createdAt}</span>{window && <p>{t("reviewV2.windowCounts", { hits: window.hitCount, falseCount: window.falseCount, unknowns: window.unknownCount })}</p>}</li>;
    })}</ol>
      <div className="flex gap-2 pt-2"><Button size="sm" variant="outline" aria-label={t("reviewV2.previousEventPage")} disabled={!previous.length || events.isFetching} onClick={() => { setCursor(previous[previous.length - 1] ?? ""); setPrevious(values => values.slice(0, -1)); }}>{t("reviewV2.previous")}</Button><Button size="sm" variant="outline" aria-label={t("reviewV2.nextEventPage")} disabled={!events.data.nextCursor || events.isFetching} onClick={() => { setPrevious(values => [...values, cursor]); setCursor(events.data!.nextCursor ?? ""); }}>{t("reviewV2.next")}</Button></div>
    </>}
  </details>;
}

function EpisodeRow({ item }: { item: ReviewEpisodeHistoryItem }) {
  const { t } = useTranslation();
  const state = item.resolvedAt ? t("reviewV2.historyResolved", { reason: t(`reviewV2.historyReason.${item.resolvedReason}`) }) : item.acknowledgedAt ? t("reviewV2.status.acknowledged") : t("reviewV2.status.pending");
  return <li className="space-y-1 rounded-lg border p-3 text-sm">
    <div className="flex flex-wrap justify-between gap-2"><span className="font-medium">{item.instrumentName}</span><span>{state}</span></div>
    <p>{t("reviewV2.historyTriggered", { date: item.triggeredOn, hits: item.hitCount, from: item.windowStart, to: item.windowEnd })}</p>
    <p className="text-xs text-muted-foreground">{t("reviewV2.historyVersion", { version: item.openedConditionVersion, detected: item.firstDetectedAt })}</p>
    {item.acknowledgedAt && <p className="text-xs">{t("reviewV2.historyAcknowledged", { at: item.acknowledgedAt })}</p>}
    {item.resolvedAt && <p className="text-xs">{t("reviewV2.historyClosed", { at: item.resolvedAt })}</p>}
    <EpisodeEvents episodeId={item.episodeId} />
  </li>;
}

export function ReviewEpisodeHistorySheet({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation();
  const rules = useReviewRulesV2(open);
  const dailyRules = (rules.data ?? []).filter(rule => rule.evaluator === "daily_predicate_v1");
  const [chosenRule, setChosenRule] = useState("");
  const ruleId = chosenRule || dailyRules[0]?.id || "";
  const [cursor, setCursor] = useState("");
  const [previous, setPrevious] = useState<string[]>([]);
  const history = useReviewEpisodeHistoryPage(ruleId, cursor, open);
  const changeRule = (id: string) => { setChosenRule(id); setCursor(""); setPrevious([]); };
  return <Sheet open={open} onOpenChange={onOpenChange}><SheetContent size="lg"><SheetHeader><SheetTitle>{t("reviewV2.historyTitle")}</SheetTitle></SheetHeader>
    {rules.isError && <p role="alert">{t("reviewV2.loadError")}</p>}
    {!!dailyRules.length && <NativeSelect aria-label={t("reviewV2.historyRule")} value={ruleId} onChange={event => changeRule(event.target.value)}>{dailyRules.map(rule => <option key={rule.id} value={rule.id}>{rule.name}</option>)}</NativeSelect>}
    {rules.hasNextPage && <Button size="sm" variant="outline" disabled={rules.isFetchingNextPage} onClick={() => void rules.fetchNextPage()}>{t("reviewV2.loadMore")}</Button>}
    {!dailyRules.length && !rules.isPending && <p>{t("reviewV2.noRules")}</p>}
    {history.isPending && ruleId && <p role="status">{t("ui.state.loadingPage")}</p>}
    {history.isError && <p role="alert">{t("reviewV2.staleRead")} <Button size="sm" variant="outline" onClick={() => { setCursor(""); setPrevious([]); void history.refetch(); }}>{t("reviewV2.refresh")}</Button></p>}
    {history.data && <><ul className="space-y-2">{(history.data.items ?? []).map(item => <EpisodeRow key={item.episodeId} item={item} />)}</ul>
      {!history.data.items?.length && <p>{t("reviewV2.historyEmpty")}</p>}
      <div className="flex gap-2 pt-3"><Button variant="outline" disabled={!previous.length || history.isFetching} onClick={() => { setCursor(previous[previous.length - 1] ?? ""); setPrevious(values => values.slice(0, -1)); }}>{t("reviewV2.previous")}</Button><Button variant="outline" disabled={!history.data.nextCursor || history.isFetching} onClick={() => { setPrevious(values => [...values, cursor]); setCursor(history.data!.nextCursor ?? ""); }}>{t("reviewV2.next")}</Button></div>
    </>}
  </SheetContent></Sheet>;
}

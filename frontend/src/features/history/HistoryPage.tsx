import { DateRangeControl } from "@/components/charts/DateRangeControl";
import { originLocalDate } from "@/features/insights/analysisRequest";
import { ymdInTimeZone } from "@/features/insights/calendar";
import { useMarketDataHealth } from "@/queries/marketdata";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { ActionMenu } from "@/components/ui/action-menu";
import { IconTile } from "@/components/ui/icon-tile";
import { activityStyle } from "@/lib/activityStyle";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { displayError } from "@/lib/display";
import { instrumentDisplayLabel, instrumentDisplayLabels } from "@/lib/instrumentDisplay";
import { ErrorState, EmptyState, LoadingState } from "@/components/layout/PageState";
import { PageIntro } from "@/components/layout/PageHeader";
import { PageChrome } from "@/components/layout/PageChrome";
import { useAccounts } from "@/queries/accounts";
import { useHoldingsByAccounts, useInstruments } from "@/queries/investments";
import { useActivity, useActivityPage, useDailySnapshotState, useHistoryOrigin, useRebuildHistoricalSnapshots, useUndoChange } from "@/queries/history";
import { useSettings } from "@/queries/settings";
import { RecordChangeForm } from "@/features/history/RecordChangeForm";
import { StartHistoryForm } from "@/features/history/StartHistoryForm";
import { activityToInitialCommand } from "@/features/history/activityToCommand";
import { activitySentence } from "@/features/history/activitySentence";
import { ActivityDetailSheet } from "@/features/history/ActivityDetailSheet";
import { ProductDetailSheet } from "@/features/liquidity/ProductSheets";
import { DatePicker } from "@/components/ui/date-picker";
import { resolvedTimeZone } from "@/lib/time";
import type { ActivityDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import type { HistoryNavigationFilters } from "@/app/navigation";

const ACTIVITY_KINDS = ["cash_in", "cash_out", "cash_dividend", "cash_transfer", "fx_conversion", "position_transfer", "buy", "sell", "value_update", "debt_draw", "debt_payment"];

function Timeline({ navigationFilters }: { navigationFilters?: HistoryNavigationFilters }) {
  const { t } = useTranslation();
  const accounts = useAccounts({ includeArchived: true });
  const instruments = useInstruments(true);
  const accountIds = (accounts.data ?? []).map((record) => record.account.id);
  const holdings = useHoldingsByAccounts(accountIds);
  const origin = useHistoryOrigin();
  const undoChange = useUndoChange();
  const [open, setOpen] = useState(false);
  const [productTarget, setProductTarget] = useState<string | null>(null);
  const [fixTarget, setFixTarget] = useState<ActivityDTO | null>(null);
  const [undoTarget, setUndoTarget] = useState<ActivityDTO | null>(null);
  const [detailTarget, setDetailTarget] = useState<ActivityDTO | null>(null);
  const [kindFilter, setKindFilter] = useState(navigationFilters?.kinds?.[0] ?? "");
  const [accountFilter, setAccountFilter] = useState(navigationFilters?.accountId ?? "");
  const [instrumentFilter, setInstrumentFilter] = useState(navigationFilters?.instrumentId ?? "");
  const [fromLocalDate, setFromLocalDate] = useState(navigationFilters?.from ?? "");
  const [toLocalDate, setToLocalDate] = useState(navigationFilters?.to ?? "");
  const today = ymdInTimeZone(new Date(), origin.data?.timezone);
  const originDate = origin.data?.startedAt ? originLocalDate(origin.data.startedAt, origin.data.timezone) : undefined;
  const originalActivity = useActivity(detailTarget?.reversesActivityId ?? "");
  const activities = useActivityPage({
    accountId: accountFilter || undefined,
    instrumentId: instrumentFilter || undefined,
    kinds: kindFilter ? [kindFilter] : undefined,
    fromLocalDate: fromLocalDate || undefined,
    toLocalDate: toLocalDate || undefined,
    limit: 50,
  });

  if (activities.isLoading || accounts.isLoading || instruments.isLoading || holdings.isLoading) {
    return <LoadingState label={t("history.loading")} />;
  }

  if (activities.isError || accounts.isError || instruments.isError || holdings.isError) {
    return (
      <ErrorState
        title={t("history.loadError")}
        description={t("ui.state.errorDescription")}
        onRetry={() => { void Promise.all([activities.refetch(), accounts.refetch(), instruments.refetch(), holdings.refetch()]); }}
        retryLabel={t("common.retryAction")}
      />
    );
  }

  const activityList = activities.data?.pages.flatMap((page) => page.activities ?? []) ?? [];
  const reversedActivityIds = new Set(
    activityList
      .map((activity) => activity.reversesActivityId)
      .filter((id): id is string => Boolean(id)),
  );
  const accountNames = new Map((accounts.data ?? []).map((record) => [record.account.id, record.account.name]));
  const instrumentNames = instrumentDisplayLabels(instruments.data ?? [], t("history.unknownInstrument"));
  const holdingNames = new Map(
    Object.entries(holdings.data ?? {}).flatMap(([accountId, accountHoldings]) =>
      (accountHoldings ?? []).map((holding) => [
        holding.id,
        `${accountNames.get(accountId) ?? t("history.unknownAccount")} · ${instrumentNames.get(holding.instrumentId) ?? t("history.unknownInstrument")}`,
      ] as const),
    ),
  );

  // Group the flat, newest-first timeline by local date for scanning.
  const activityGroups: Array<{ date: string; items: typeof activityList }> = [];
  for (const activity of activityList) {
    const last = activityGroups[activityGroups.length - 1];
    if (last && last.date === activity.effectiveLocalDate) last.items.push(activity);
    else activityGroups.push({ date: activity.effectiveLocalDate, items: [activity] });
  }

  return (
    <div className="flex flex-col gap-4">
      <section className="flex flex-col gap-4 rounded-2xl border border-border bg-card p-4" aria-label={t("history.filterLabel")}>
        <div className="grid gap-3 sm:grid-cols-3">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="history-filter-kind">{t("history.filterKind")}</Label>
            <NativeSelect id="history-filter-kind" value={kindFilter} onChange={(event) => setKindFilter(event.target.value)}>
              <option value="">{t("history.filterAll")}</option>
              {ACTIVITY_KINDS.map((kind) => <option key={kind} value={kind}>{t(`history.kind.${kind}`)}</option>)}
            </NativeSelect>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="history-filter-account">{t("history.filterAccount")}</Label>
            <NativeSelect id="history-filter-account" value={accountFilter} onChange={(event) => setAccountFilter(event.target.value)}>
              <option value="">{t("history.filterAll")}</option>
              {(accounts.data ?? []).filter((record) => !record.account.archivedAt).map((record) => <option key={record.account.id} value={record.account.id}>{record.account.name}</option>)}
            </NativeSelect>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="history-filter-instrument">{t("history.filterInstrument")}</Label>
            <NativeSelect id="history-filter-instrument" value={instrumentFilter} onChange={(event) => setInstrumentFilter(event.target.value)}>
              <option value="">{t("history.filterAll")}</option>
              {(instruments.data ?? []).filter((instrument) => !instrument.archivedAt).map((instrument) => <option key={instrument.id} value={instrument.id}>{instrumentDisplayLabel(instrument, instrument.name)}</option>)}
            </NativeSelect>
          </div>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="history-filter-from">{t("history.filterFrom")}</Label>
            <DatePicker id="history-filter-from" value={fromLocalDate} clearable min={originDate} max={toLocalDate || today} onChange={setFromLocalDate} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="history-filter-to">{t("history.filterTo")}</Label>
            <DatePicker id="history-filter-to" value={toLocalDate} clearable min={fromLocalDate || originDate} max={today} onChange={setToLocalDate} />
          </div>
        </div>
        <div className="border-t border-border pt-3">
          <DateRangeControl
            value={{ from: fromLocalDate || originDate || "", to: toLocalDate || today }}
            min={originDate}
            max={today}
            disabled={!originDate}
            onChange={({ from, to }) => { setFromLocalDate(from); setToLocalDate(to); }}
          />
        </div>
        {(kindFilter || accountFilter || instrumentFilter || fromLocalDate || toLocalDate) && <Button type="button" variant="ghost" size="sm" className="self-end" onClick={() => { setKindFilter(""); setAccountFilter(""); setInstrumentFilter(""); setFromLocalDate(""); setToLocalDate(""); }}>{t("history.filterClear")}</Button>}
      </section>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">{t("history.activityKind")}</p>
        <PageChrome
          pageId="history"
          actions={
            <Sheet open={open} onOpenChange={setOpen}>
              <SheetTrigger className={cn(buttonVariants(), "gap-2")}>
                <Plus className="size-4" aria-hidden="true" /> {t("history.recordButton")}
              </SheetTrigger>
              <SheetContent size="lg">
                <SheetHeader>
                  <SheetTitle>{t("history.formLabel")}</SheetTitle>
                </SheetHeader>
                <div>
                  <RecordChangeForm
                    onRecorded={() => {
                      toast.success(t("history.recorded"));
                      setOpen(false);
                    }}
                  />
                </div>
              </SheetContent>
            </Sheet>
          }
        />
      </div>

      {activityList.length === 0 ? (
        <EmptyState title={t("history.noActivityYet")} description={t("history.noActivityDescription")} />
      ) : (
        <div className="flex flex-col gap-6" data-testid="activity-list" aria-label={t("history.activityKind")}>
          {activityGroups.map((group) => (
            <section key={group.date} className="flex flex-col gap-2">
              <h3 className="num px-1 text-xs font-semibold text-muted-foreground">
                <time dateTime={group.items[0].effectiveAt}>{group.date}</time>
              </h3>
              <ul className="overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
                {group.items.map((activity) => {
                  const canModify = activity.kind !== "cost_adjustment" && !activity.reversesActivityId && !reversedActivityIds.has(activity.id);
                  const summary = activitySentence(t, activity, accountNames, instrumentNames, holdingNames);
                  const style = activityStyle(activity.kind);
                  const menuItems = canModify && !activity.productContext
                    ? [
                        { id: "fix", label: t("history.fixAction"), onSelect: () => setFixTarget(activity) },
                        { id: "undo", label: t("history.undoAction"), onSelect: () => setUndoTarget(activity) },
                      ]
                    : [];
                  return (
                    <li key={activity.id} className="relative flex flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-3 text-sm transition-colors last:border-0 hover:bg-primary/5">
                      <IconTile icon={style.icon} tone={style.tone} />
                      <div className="flex min-w-0 flex-1 flex-col gap-1">
                        <p className="font-semibold text-foreground">{summary}</p>
                        {(activity.reversesActivityId || activity.correctionGroupId) && (
                          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                            {activity.reversesActivityId && <Badge variant="secondary">{t("history.reversal")}</Badge>}
                            {activity.correctionGroupId && !activity.reversesActivityId && <Badge variant="secondary">{t("history.corrected")}</Badge>}
                          </div>
                        )}
                      </div>
                      <span className="flex shrink-0 items-center gap-1">
                        {/* The Details button stretches over the whole row so the entire row opens the detail sheet. */}
                        <Button variant="ghost" size="sm" className="after:absolute after:inset-0 after:rounded-none focus-visible:ring-inset" onClick={() => setDetailTarget(activity)}>
                          {t("common.details")}
                        </Button>
                        {activity.productContext && <Button variant="outline" size="sm" className="relative z-10" onClick={() => setProductTarget(activity.productContext!.productId)}>{t("availableFunds.manageProductOperation")}</Button>}
                        {menuItems.length > 0 && (
                          <span className="relative z-10">
                            <ActionMenu label={t("common.actions")} items={menuItems} />
                          </span>
                        )}
                      </span>
                    </li>
                  );
                })}
              </ul>
            </section>
          ))}
        </div>
      )}
      {activities.hasNextPage && (
        <Button
          type="button"
          variant="outline"
          className="self-center"
          data-testid="history-load-more"
          disabled={activities.isFetchingNextPage}
          onClick={() => {
            void activities.fetchNextPage();
          }}
        >
          {t("history.loadMore")}
        </Button>
      )}

      <AlertDialog open={undoTarget !== null} onOpenChange={(next) => { if (!next) setUndoTarget(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("history.undoTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("history.undoDescription")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (!undoTarget) return;
                undoChange.mutate(undoTarget.id, {
                  onSuccess: () => toast.success(t("history.undone")),
                  onError: (error) => toast.error(displayError(error, t("history.actionError"))),
                });
              }}
            >
              {t("history.undoAction")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Sheet open={fixTarget !== null} onOpenChange={(next) => !next && setFixTarget(null)}>
        <SheetContent size="lg">
          <SheetHeader>
            <SheetTitle>{t("history.fixTitle")}</SheetTitle>
          </SheetHeader>
          <div>
            {fixTarget && (
              <RecordChangeForm
                key={fixTarget.id}
                fixActivityId={fixTarget.id}
                initial={activityToInitialCommand(fixTarget)}
                originalEffectiveAt={fixTarget.effectiveAt}
                onRecorded={() => {
                  toast.success(t("history.fixed"));
                  setFixTarget(null);
                }}
              />
            )}
          </div>
        </SheetContent>
      </Sheet>
      {productTarget && <ProductDetailSheet key={productTarget} productId={productTarget} open onOpenChange={(next) => { if (!next) setProductTarget(null); }} />}
      <ActivityDetailSheet
        activity={detailTarget}
        timezone={origin.data?.timezone}
        accounts={accountNames}
        instruments={instrumentNames}
        holdings={holdingNames}
        originalActivity={originalActivity.data}
        originalActivityLoading={originalActivity.isLoading}
        onClose={() => setDetailTarget(null)}
        t={t}
      />
    </div>
  );
}

/**
 * HistoryPage implements Starting Point, Record change, Timeline, Undo, and
 * Fix through HistoryService. Mutating actions keep the append-only history
 * contract visible to the user and never expose backend enum identifiers.
 */
export function HistoryPage({ navigationFilters }: { navigationFilters?: HistoryNavigationFilters }) {
  const { t } = useTranslation();
  const origin = useHistoryOrigin();
  const snapshotState = useDailySnapshotState();
  const health = useMarketDataHealth();
  const snapshotIssue = health.data?.issues?.find((issue) => issue.kind === "snapshot_missing" || issue.kind === "snapshot_outdated");
  const rebuildSnapshots = useRebuildHistoricalSnapshots();
  const settings = useSettings();

  const repairSnapshots = () => {
    if (health.isError || !snapshotIssue?.executable || !snapshotIssue.rangeStart || !snapshotIssue.rangeEnd) return;
    const start = snapshotIssue.rangeStart;
    const end = snapshotIssue.rangeEnd;
    rebuildSnapshots.mutate({ startDate: start, endDate: end }, {
      onSuccess: (count) => toast.success(t("history.snapshotsRepaired", { count })),
      onError: (error) => toast.error(displayError(error, t("history.actionError"))),
    });
  };

  return (
    <div className="flex flex-col gap-6">
      <PageChrome pageId="history" title={t("nav.history")} />
      <PageIntro description={t("history.description")} />
      {origin.isLoading && <LoadingState label={t("ui.state.loadingPage")} />}
      {origin.isError && (
        <ErrorState
          title={t("history.loadError")}
          description={t("ui.state.errorDescription")}
          onRetry={() => origin.refetch()}
          retryLabel={t("common.retryAction")}
        />
      )}
      {!origin.isLoading && !origin.isError && !origin.data && <StartHistoryForm />}
      {!origin.isLoading && !origin.isError && origin.data && (
        <>
          <p className="text-sm text-muted-foreground" data-testid="history-origin-timezone">
            {t("history.originTimezone", { timezone: origin.data.timezone })}
          </p>
          {settings.data && resolvedTimeZone(settings.data.timezone) !== origin.data.timezone && (
            <p className="text-xs text-muted-foreground">
              {t("history.settingsTimezoneDiff", { origin: origin.data.timezone, presentation: resolvedTimeZone(settings.data.timezone) })}
            </p>
          )}
          <Timeline key={JSON.stringify(navigationFilters ?? {})} navigationFilters={navigationFilters} />
          <section className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-border bg-muted/40 px-4 py-3" aria-label={t("history.snapshotHealth")}>
            <div className="flex flex-col gap-1 text-sm">
              <p className="font-medium">{t("history.snapshotHealth")}</p>
              <p className="text-muted-foreground">
                {health.isError ? t("review.healthFailed") : !health.data ? t("review.healthLoading") : snapshotIssue
                  ? t("history.snapshotNeedsRepair", { date: snapshotIssue.rangeStart })
                  : snapshotState.data?.lastCompletedClosedOn
                    ? t("history.snapshotHealthyThrough", { date: snapshotState.data.lastCompletedClosedOn })
                    : t("history.snapshotNotBuilt")}
              </p>
              {snapshotIssue && !snapshotIssue.executable && <p className="text-xs text-muted-foreground">{t("review.snapshotPrerequisite")}</p>}
            </div>
            <Button type="button" variant="outline" size="sm" onClick={repairSnapshots} disabled={rebuildSnapshots.isPending || health.isError || !snapshotIssue?.executable}>
              {rebuildSnapshots.isPending ? t("common.pending") : t("history.repairSnapshots")}
            </Button>
          </section>
        </>
      )}
    </div>
  );
}

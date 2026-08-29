import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
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
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { displayError } from "@/lib/display";
import { ErrorState, EmptyState, LoadingState } from "@/components/layout/PageState";
import { PageHeader } from "@/components/layout/PageHeader";
import { useAccounts } from "@/queries/accounts";
import { useInstruments } from "@/queries/investments";
import { useActivityPage, useHistoryOrigin, useUndoChange } from "@/queries/history";
import { useSettings } from "@/queries/settings";
import { RecordChangeForm } from "@/features/history/RecordChangeForm";
import { StartHistoryForm } from "@/features/history/StartHistoryForm";
import { activityToInitialCommand } from "@/features/history/activityToCommand";
import { activitySentence } from "@/features/history/activitySentence";
import { ActivityDetailSheet } from "@/features/history/ActivityDetailSheet";
import { DatePicker } from "@/components/ui/date-picker";
import { resolvedTimeZone } from "@/lib/time";
import type { ActivityDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

const ACTIVITY_KINDS = ["cash_in", "cash_out", "cash_dividend", "cash_transfer", "fx_conversion", "position_transfer", "buy", "sell", "value_update", "debt_draw", "debt_payment"];

function Timeline() {
  const { t } = useTranslation();
  const accounts = useAccounts({});
  const instruments = useInstruments();
  const origin = useHistoryOrigin();
  const undoChange = useUndoChange();
  const [open, setOpen] = useState(false);
  const [fixTarget, setFixTarget] = useState<ActivityDTO | null>(null);
  const [detailTarget, setDetailTarget] = useState<ActivityDTO | null>(null);
  const [kindFilter, setKindFilter] = useState("");
  const [accountFilter, setAccountFilter] = useState("");
  const [fromLocalDate, setFromLocalDate] = useState("");
  const [toLocalDate, setToLocalDate] = useState("");
  const activities = useActivityPage({
    accountId: accountFilter || undefined,
    kinds: kindFilter ? [kindFilter] : undefined,
    fromLocalDate: fromLocalDate || undefined,
    toLocalDate: toLocalDate || undefined,
    limit: 50,
  });

  if (activities.isLoading || accounts.isLoading || instruments.isLoading) {
    return <LoadingState label={t("history.loading")} />;
  }

  if (activities.isError) {
    return (
      <ErrorState
        title={t("history.loadError")}
        description={t("ui.state.errorDescription")}
        onRetry={() => activities.refetch()}
        retryLabel={t("common.retryAction")}
      />
    );
  }

  const activityList = activities.data?.activities ?? [];
  const reversedActivityIds = new Set(
    activityList
      .map((activity) => activity.reversesActivityId)
      .filter((id): id is string => Boolean(id)),
  );
  const accountNames = new Map((accounts.data ?? []).map((record) => [record.account.id, record.account.name]));
  const instrumentNames = new Map((instruments.data ?? []).map((instrument) => [instrument.id, instrument.name]));

  return (
    <div className="flex flex-col gap-4">
      <section className="grid gap-3 rounded-lg border border-border bg-card p-3 sm:grid-cols-2 lg:grid-cols-4" aria-label={t("history.filterLabel")}>
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
          <Label htmlFor="history-filter-from">{t("history.filterFrom")}</Label>
          <DatePicker id="history-filter-from" value={fromLocalDate} onChange={setFromLocalDate} />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="history-filter-to">{t("history.filterTo")}</Label>
          <DatePicker id="history-filter-to" value={toLocalDate} onChange={setToLocalDate} />
        </div>
        {(kindFilter || accountFilter || fromLocalDate || toLocalDate) && <Button type="button" variant="ghost" size="sm" className="self-end sm:col-span-2 lg:col-span-4 lg:justify-self-end" onClick={() => { setKindFilter(""); setAccountFilter(""); setFromLocalDate(""); setToLocalDate(""); }}>{t("history.filterClear")}</Button>}
      </section>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">{t("history.activityKind")}</p>
        <Sheet open={open} onOpenChange={setOpen}>
          <SheetTrigger className={cn(buttonVariants(), "gap-2") }>
            <Plus className="size-4" aria-hidden="true" /> {t("history.recordButton")}
          </SheetTrigger>
          <SheetContent>
            <SheetHeader>
              <SheetTitle>{t("history.formLabel")}</SheetTitle>
            </SheetHeader>
            <div className="overflow-y-auto">
              <RecordChangeForm
                onRecorded={() => {
                  toast.success(t("history.recorded"));
                  setOpen(false);
                }}
              />
            </div>
          </SheetContent>
        </Sheet>
      </div>

      {activityList.length === 0 ? (
        <EmptyState title={t("history.noActivityYet")} description={t("history.noActivityDescription")} />
      ) : (
        <ul className="flex flex-col gap-2" data-testid="activity-list" aria-label={t("history.activityKind")}>
          {activityList.map((activity) => {
            const canModify = !activity.reversesActivityId && !reversedActivityIds.has(activity.id);
            const summary = activitySentence(t, activity, accountNames, instrumentNames);
            return (
              <li key={activity.id} className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-border px-3 py-3 text-sm">
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <p className="font-medium text-foreground">{summary}</p>
                  <div className="flex flex-wrap items-center gap-2 text-muted-foreground">
                    <time dateTime={activity.effectiveAt}>{activity.effectiveLocalDate}</time>
                    {activity.reversesActivityId && <Badge variant="secondary">{t("history.reversal")}</Badge>}
                    {activity.correctionGroupId && !activity.reversesActivityId && <Badge variant="secondary">{t("history.corrected")}</Badge>}
                  </div>
                </div>
                <span className="flex shrink-0 items-center gap-2">
                  <Button variant="outline" size="sm" onClick={() => setDetailTarget(activity)}>
                    {t("common.details")}
                  </Button>
                  {canModify && (
                    <>
                      <Button variant="outline" size="sm" onClick={() => setFixTarget(activity)}>
                        {t("history.fixAction")}
                      </Button>
                      <AlertDialog>
                        <AlertDialogTrigger className={cn(buttonVariants({ variant: "outline", size: "sm" }))}>
                          {t("history.undoAction")}
                        </AlertDialogTrigger>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>{t("history.undoTitle")}</AlertDialogTitle>
                            <AlertDialogDescription>{t("history.undoDescription")}</AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
                            <AlertDialogAction
                              onClick={() =>
                                undoChange.mutate(activity.id, {
                                  onSuccess: () => toast.success(t("history.undone")),
                                  onError: (error) => toast.error(displayError(error, t("history.actionError"))),
                                })
                              }
                            >
                              {t("history.undoAction")}
                            </AlertDialogAction>
                          </AlertDialogFooter>
                        </AlertDialogContent>
                      </AlertDialog>
                    </>
                  )}
                </span>
              </li>
            );
          })}
        </ul>
      )}

      <Sheet open={fixTarget !== null} onOpenChange={(next) => !next && setFixTarget(null)}>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>{t("history.fixTitle")}</SheetTitle>
          </SheetHeader>
          <div className="overflow-y-auto">
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
      <ActivityDetailSheet
        activity={detailTarget}
        timezone={origin.data?.timezone}
        accounts={accountNames}
        instruments={instrumentNames}
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
export function HistoryPage() {
  const { t } = useTranslation();
  const origin = useHistoryOrigin();
  const settings = useSettings();

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("nav.history")} description={t("history.description")} />
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
          <Timeline />
        </>
      )}
    </div>
  );
}

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
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
import { useHistoryOrigin, useListActivities, useUndoChange } from "@/queries/history";
import { RecordChangeForm } from "@/features/history/RecordChangeForm";
import { StartHistoryForm } from "@/features/history/StartHistoryForm";
import { activityToInitialCommand } from "@/features/history/activityToCommand";
import { activitySentence } from "@/features/history/activitySentence";
import type { ActivityDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

function Timeline() {
  const { t } = useTranslation();
  const activities = useListActivities();
  const accounts = useAccounts({});
  const instruments = useInstruments();
  const undoChange = useUndoChange();
  const [open, setOpen] = useState(false);
  const [fixTarget, setFixTarget] = useState<ActivityDTO | null>(null);

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

  const reversedActivityIds = new Set(
    (activities.data ?? [])
      .map((activity) => activity.reversesActivityId)
      .filter((id): id is string => Boolean(id)),
  );
  const accountNames = new Map((accounts.data ?? []).map((record) => [record.account.id, record.account.name]));
  const instrumentNames = new Map((instruments.data ?? []).map((instrument) => [instrument.id, instrument.name]));

  return (
    <div className="flex flex-col gap-4">
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

      {(activities.data ?? []).length === 0 ? (
        <EmptyState title={t("history.noActivityYet")} description={t("history.noActivityDescription")} />
      ) : (
        <ul className="flex flex-col gap-2" data-testid="activity-list" aria-label={t("history.activityKind")}>
          {(activities.data ?? []).map((activity) => {
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
                {canModify && (
                  <span className="flex shrink-0 items-center gap-2">
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
                  </span>
                )}
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
                onRecorded={() => {
                  toast.success(t("history.fixed"));
                  setFixTarget(null);
                }}
              />
            )}
          </div>
        </SheetContent>
      </Sheet>
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
      {!origin.isLoading && !origin.isError && origin.data && <Timeline />}
    </div>
  );
}

import { useState } from "react";
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
import { useHistoryOrigin, useStartHistory, useListActivities, useUndoChange } from "@/queries/history";
import { RecordChangeForm } from "@/features/history/RecordChangeForm";

function StartHistoryPrompt() {
  const startHistory = useStartHistory();
  const [timezone, setTimezone] = useState(() => Intl.DateTimeFormat().resolvedOptions().timeZone);

  return (
    <div className="mx-auto flex max-w-md flex-col gap-4 py-12 text-center">
      <h2 className="text-lg font-semibold text-foreground">Start History</h2>
      <p className="text-sm text-muted-foreground">
        History captures your current balances as a Starting Point, then tracks every change from here forward.
      </p>
      <div className="flex flex-col gap-1.5 text-left">
        <label htmlFor="history-timezone" className="text-sm font-medium">
          Timezone
        </label>
        <input
          id="history-timezone"
          value={timezone}
          onChange={(event) => setTimezone(event.target.value)}
          className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground"
        />
      </div>
      <Button onClick={() => startHistory.mutate(timezone, { onSuccess: () => toast.success("History started") })} disabled={startHistory.isPending}>
        Start History
      </Button>
      {startHistory.isError && (
        <p role="alert" className="text-sm text-destructive">
          {(startHistory.error as Error).message}
        </p>
      )}
    </div>
  );
}

function Timeline() {
  const activities = useListActivities();
  const undoChange = useUndoChange();
  const [open, setOpen] = useState(false);

  return (
    <div className="flex flex-col gap-4">
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetTrigger className={cn(buttonVariants(), "gap-2 self-start")}>
          <Plus className="size-4" /> Record change
        </SheetTrigger>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>Record change</SheetTitle>
          </SheetHeader>
          <div className="overflow-y-auto">
            <RecordChangeForm
              onRecorded={() => {
                toast.success("Change recorded");
                setOpen(false);
              }}
            />
          </div>
        </SheetContent>
      </Sheet>

      {activities.data && activities.data.length === 0 && <p className="text-sm text-muted-foreground">No activity yet.</p>}

      <ul className="flex flex-col gap-2" data-testid="activity-list">
        {(activities.data ?? []).map((activity) => (
          <li key={activity.id} className="flex items-center justify-between rounded-md border border-border px-3 py-2 text-sm">
            <span className="flex items-center gap-2">
              <Badge variant="outline">{activity.kind}</Badge>
              {activity.effectiveLocalDate}
              {activity.reversesActivityId && <Badge variant="secondary">Reversal</Badge>}
            </span>
            {!activity.reversesActivityId && (
              <AlertDialog>
                <AlertDialogTrigger className={cn(buttonVariants({ variant: "outline", size: "sm" }))}>Undo</AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>Undo this change?</AlertDialogTitle>
                    <AlertDialogDescription>This reverses the change with a new offsetting activity. It cannot be undone twice.</AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction
                      onClick={() =>
                        undoChange.mutate(activity.id, {
                          onSuccess: () => toast.success("Change undone"),
                          onError: (error) => toast.error((error as Error).message),
                        })
                      }
                    >
                      Undo
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * HistoryPage implements Starting Point, Record change (all ten
 * domain.PreviewChange kinds), Timeline, and Undo end to end through
 * HistoryService (implementation plan Phase 5), per the navigation
 * decision that Activity lives inside History rather than as a separate
 * nav entry. Fix (replacing a change with a corrected one) is a
 * documented gap for a following pass: it needs the same dynamic form as
 * Record change pre-filled from the original activity's effects, which
 * is additional UI work beyond what this pass's time allowed; the Go
 * binding (HistoryService.FixChange) is already implemented and tested.
 */
export function HistoryPage() {
  const origin = useHistoryOrigin();

  if (origin.isLoading) {
    return null;
  }
  if (!origin.data) {
    return <StartHistoryPrompt />;
  }
  return <Timeline />;
}

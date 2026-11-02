import { CheckCircle2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { PageChrome } from "@/components/layout/PageChrome";
import { PageIntro } from "@/components/layout/PageHeader";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { hasUnresolvedSheetConfirmation } from "@/components/ui/sheet-confirmation";
import { cn } from "@/lib/utils";
import type { ReconciliationNavigationFocus, TasksTab } from "@/app/navigation";
import { InvestmentReviewPage } from "@/features/investment-review/InvestmentReviewPage";
import { ReconciliationPage } from "@/features/reconciliation/ReconciliationPage";
import { UpcomingActionsPage } from "@/features/upcoming-actions/UpcomingActionsPage";
import { formatTasksBadge, useTasksCounts } from "./useTasksCounts";

const TAB_ORDER: TasksTab[] = ["upcoming", "review", "reconciliation"];

function TabBadge({ count, testId, neutral }: { count: number; testId: string; neutral?: boolean }) {
  const { t } = useTranslation();
  if (count <= 0) return null;
  return (
    <>
      <span
        aria-hidden="true"
        data-testid={testId}
        className={cn(
          "ml-1.5 inline-flex min-w-5 items-center justify-center rounded-full px-1.5 text-xs font-semibold",
          neutral ? "bg-muted text-muted-foreground" : "bg-warning/18 text-warning-foreground",
        )}
      >
        {formatTasksBadge(count)}
      </span>
      <span className="sr-only">{t("tasks.badge", { count })}</span>
    </>
  );
}

export function TasksPage({
  isActive = true,
  tab,
  onTabChange,
  accountId,
  sessionId,
  navigationRequestId,
  reconciliationEntries,
}: {
  isActive?: boolean;
  tab: TasksTab;
  onTabChange: (tab: TasksTab) => void;
  accountId?: string;
  sessionId?: string;
  navigationRequestId?: number;
  reconciliationEntries?: Array<ReconciliationNavigationFocus & { navigationRequestId: number }>;
}) {
  const { t } = useTranslation();
  const counts = useTasksCounts(isActive);
  const [actionsHost, setActionsHost] = useState<HTMLDivElement | null>(null);
  const [reconciliationSeen, setReconciliationSeen] = useState(tab === "reconciliation");
  if (tab === "reconciliation" && !reconciliationSeen) setReconciliationSeen(true);
  const entries = reconciliationEntries ?? [{ accountId, sessionId, navigationRequestId: navigationRequestId ?? 0 }];
  const allClear = counts.upcoming.isSuccess && counts.review.isSuccess && counts.sessions.isSuccess
    && counts.attention === 0 && counts.actionable === 0 && counts.openSessions === 0;
  const selectTab = (value: string | number | null) => {
    if (hasUnresolvedSheetConfirmation()) return;
    const next = TAB_ORDER.find((item) => item === value);
    if (next) onTabChange(next);
  };
  return (
    <div className="flex min-w-0 flex-col gap-5">
      <PageChrome
        pageId="tasks"
        enabled={isActive}
        title={t("tasks.title")}
        actions={<div ref={setActionsHost} className="contents" />}
      />
      {/* The page bar already shows the title; this heading stays as the focus target for in-app navigation. */}
      <h2 id="tasks-heading" tabIndex={-1} className="sr-only">{t("tasks.title")}</h2>
      <PageIntro
        description={t("tasks.description")}
        status={allClear ? <p role="status" className="flex items-center gap-1.5 text-sm text-success"><CheckCircle2 aria-hidden="true" className="size-4" />{t("tasks.allClear")}</p> : undefined}
      />
      <Tabs value={tab} onValueChange={selectTab}>
        <TabsList aria-label={t("tasks.title")} activateOnFocus>
          <TabsTrigger value="upcoming">
            {t("tasks.tabs.upcoming")}
            <TabBadge count={counts.attention} testId="tasks-tab-badge-upcoming" />
          </TabsTrigger>
          <TabsTrigger value="review">
            {t("tasks.tabs.review")}
            <TabBadge count={counts.actionable} testId="tasks-tab-badge-review" />
          </TabsTrigger>
          <TabsTrigger value="reconciliation">
            {t("tasks.tabs.reconciliation")}
            <TabBadge count={counts.openSessions} testId="tasks-tab-badge-reconciliation" neutral />
          </TabsTrigger>
        </TabsList>
        <TabsContent value="upcoming" keepMounted>
          <UpcomingActionsPage isActive={isActive && tab === "upcoming"} actionsHost={tab === "upcoming" ? actionsHost : null} />
        </TabsContent>
        <TabsContent value="review" keepMounted>
          <InvestmentReviewPage isActive={isActive && tab === "review"} actionsHost={tab === "review" ? actionsHost : null} />
        </TabsContent>
        <TabsContent value="reconciliation" keepMounted>
          {reconciliationSeen && entries.map(entry => (
            <div key={entry.navigationRequestId} hidden={entry.navigationRequestId !== (navigationRequestId ?? 0)}>
              <ReconciliationPage
                accountId={entry.accountId}
                sessionId={entry.sessionId}
                navigationRequestId={entry.navigationRequestId}
                isActive={isActive && tab === "reconciliation" && entry.navigationRequestId === (navigationRequestId ?? 0)}
              />
            </div>
          ))}
        </TabsContent>
      </Tabs>
    </div>
  );
}

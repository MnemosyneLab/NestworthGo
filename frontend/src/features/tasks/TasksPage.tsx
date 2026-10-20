import { useState } from "react";
import { useTranslation } from "react-i18next";
import { PageChrome } from "@/components/layout/PageChrome";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { hasUnresolvedSheetConfirmation } from "@/components/ui/sheet-confirmation";
import { cn } from "@/lib/utils";
import type { TasksTab } from "@/app/navigation";
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
}: {
  isActive?: boolean;
  tab: TasksTab;
  onTabChange: (tab: TasksTab) => void;
  accountId?: string;
  sessionId?: string;
}) {
  const { t } = useTranslation();
  const counts = useTasksCounts(isActive);
  const [actionsHost, setActionsHost] = useState<HTMLDivElement | null>(null);
  const [reconciliationSeen, setReconciliationSeen] = useState(tab === "reconciliation");
  if (tab === "reconciliation" && !reconciliationSeen) setReconciliationSeen(true);
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
      <div className="flex flex-col gap-1.5">
        <h2 id="tasks-heading" tabIndex={-1} className="text-xl font-semibold tracking-tight">{t("tasks.title")}</h2>
        <p className="max-w-3xl text-sm leading-6 text-muted-foreground">{t("tasks.description")}</p>
        {allClear && <p role="status">{t("tasks.allClear")}</p>}
      </div>
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
          {reconciliationSeen && (
            <ReconciliationPage
              key={`${accountId ?? ""}:${sessionId ?? ""}`}
              accountId={accountId}
              sessionId={sessionId}
              isActive={isActive && tab === "reconciliation"}
            />
          )}
        </TabsContent>
      </Tabs>
    </div>
  );
}

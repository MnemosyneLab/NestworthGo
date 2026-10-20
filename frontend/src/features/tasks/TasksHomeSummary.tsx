import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { useObjectNavigation } from "@/app/NavigationContext";
import type { TasksTab } from "@/app/navigation";
import { useTasksCounts } from "./useTasksCounts";

function SummaryLink({
  tab,
  pending,
  failed,
  label,
}: {
  tab: TasksTab;
  pending: boolean;
  failed: boolean;
  label: string;
}) {
  const { t } = useTranslation();
  const navigation = useObjectNavigation();
  const section = t(`tasks.tabs.${tab}`);
  const text = pending ? t("ui.state.loadingPage") : failed ? t("tasks.summaryUnavailable", { section }) : label;
  return (
    <Button
      className="h-auto max-w-full whitespace-normal p-0 text-left"
      variant="link"
      onClick={() => navigation?.open({ page: "tasks", tab })}
    >
      {text}
    </Button>
  );
}

/** One homepage strip. Each count opens its Tasks & checks tab. */
export function TasksHomeSummary({ isActive = true }: { isActive?: boolean }) {
  const { t } = useTranslation();
  const counts = useTasksCounts(isActive);
  return (
    <section aria-label={t("tasks.title")} className="flex min-w-0 flex-col gap-2 border-t border-border pt-4" data-testid="tasks-home-summary">
      <h3 className="font-medium">{t("tasks.title")}</h3>
      <div className="flex flex-col items-start gap-1">
        <SummaryLink
          tab="upcoming"
          pending={counts.upcoming.isPending && !counts.upcoming.data}
          failed={counts.upcoming.isError && !counts.upcoming.data}
          label={t("tasks.summaryUpcoming", { count: counts.attention })}
        />
        <SummaryLink
          tab="review"
          pending={counts.review.isPending && !counts.review.data}
          failed={counts.review.isError && !counts.review.data}
          label={t("tasks.summaryReview", { count: counts.actionable })}
        />
        <SummaryLink
          tab="reconciliation"
          pending={counts.sessions.isPending && !counts.sessions.data}
          failed={counts.sessions.isError && !counts.sessions.data}
          label={t("tasks.summaryReconciliation", { count: counts.openSessions })}
        />
      </div>
    </section>
  );
}

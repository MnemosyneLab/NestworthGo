import { useQuery, type QueryClient } from "@tanstack/react-query";
import { Service as LiquidityService } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity";
import { Service as ReconciliationService } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation";
import type { TasksTab } from "@/app/navigation";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

/** Shared with the review tab's default unignored page so the badge reuses that read. */
export const tasksReviewCountFilter = { status: "unignored", instrumentType: "", query: "" };

export const tasksCountStaleTime = 60_000;

export function tasksReviewCountQueryKey() {
  return [...queryKeys.reviewV2.all, "alerts", tasksReviewCountFilter, ""] as const;
}

export function formatTasksBadge(count: number) {
  return count > 99 ? "99+" : String(count);
}

export function urgentTasksTab(attention: number, actionable: number): TasksTab {
  if (attention > 0) return "upcoming";
  if (actionable > 0) return "review";
  return "upcoming";
}

/** Chooses a tab at navigation time. Later count updates must not call this again. */
export function resolveTasksTab(input: { explicit?: TasksTab; previous: TasksTab | null; attention: number; actionable: number }): TasksTab {
  if (input.explicit) return input.explicit;
  if (input.previous) return input.previous;
  return urgentTasksTab(input.attention, input.actionable);
}

export function readCachedTasksAttention(client: QueryClient) {
  const upcoming = client.getQueryData<{ counts?: { attention?: number } }>(queryKeys.liquidity.upcomingActions);
  const review = client.getQueryData<{ summary?: { actionableRules?: number } }>(tasksReviewCountQueryKey());
  return {
    attention: upcoming?.counts?.attention ?? 0,
    actionable: review?.summary?.actionableRules ?? 0,
  };
}

export function useTasksCounts(enabled = true) {
  const upcoming = useQuery({
    queryKey: queryKeys.liquidity.upcomingActions,
    queryFn: () => callService(() => LiquidityService.UpcomingActions()),
    enabled,
    staleTime: tasksCountStaleTime,
    refetchOnWindowFocus: true,
  });
  const review = useQuery({
    queryKey: tasksReviewCountQueryKey(),
    queryFn: () => callService(() => LiquidityService.ListReviewAlertsV2({ filter: tasksReviewCountFilter, cursor: "", pageSize: 50 })),
    enabled,
    staleTime: tasksCountStaleTime,
    refetchOnWindowFocus: true,
  });
  const sessions = useQuery({
    queryKey: ["reconciliation", "sessions", ""],
    queryFn: () => callService(() => ReconciliationService.List()),
    enabled,
    staleTime: tasksCountStaleTime,
    refetchOnWindowFocus: true,
  });
  const attention = upcoming.data?.counts.attention ?? 0;
  const actionable = review.data?.summary?.actionableRules ?? 0;
  const openSessions = (sessions.data ?? []).filter((session) => session.status === "in_progress" || session.status === "paused").length;
  return { attention, actionable, openSessions, sidebarCount: attention + actionable, upcoming, review, sessions };
}

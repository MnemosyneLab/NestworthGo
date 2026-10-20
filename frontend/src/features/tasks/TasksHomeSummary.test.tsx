import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NavigationContext } from "@/app/NavigationContext";
import i18n from "@/i18n";
import { createTestQueryClient } from "@/test/queryClient";
import { upcomingData } from "@/test/upcomingActions";
import { TasksHomeSummary } from "./TasksHomeSummary";
import { tasksReviewCountQueryKey } from "./useTasksCounts";

const services = vi.hoisted(() => ({
  upcoming: vi.fn(),
  alerts: vi.fn(),
  sessions: vi.fn(),
}));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({
  Service: {
    UpcomingActions: services.upcoming,
    ListReviewAlertsV2: services.alerts,
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation", () => ({
  Service: { List: services.sessions },
}));

const queryClient = createTestQueryClient({ retry: false });
const open = vi.fn();

function reviewSummary(actionable: number) {
  return {
    items: [],
    nextCursor: "",
    summary: { actionableRules: actionable },
    reportRevision: "synthetic",
    projectionStatus: "complete",
    publishedOn: "2026-10-15",
  };
}

function counts(attention: number) {
  const data = upcomingData();
  data.counts.attention = attention;
  return data;
}

function mount() {
  return render(
    <QueryClientProvider client={queryClient}>
      <NavigationContext.Provider value={{ open, openHealth: vi.fn() }}>
        <TasksHomeSummary />
      </NavigationContext.Provider>
    </QueryClientProvider>,
  );
}

beforeEach(async () => {
  await i18n.changeLanguage("en");
  queryClient.clear();
  vi.clearAllMocks();
  services.upcoming.mockResolvedValue(counts(2));
  services.alerts.mockResolvedValue(reviewSummary(1));
  services.sessions.mockResolvedValue([
    { id: "open", status: "in_progress" },
    { id: "paused", status: "paused" },
  ]);
});

describe("Tasks home summary", () => {
  it("keeps cached counts and retries only the source whose refresh failed", async () => {
    mount();
    expect(await screen.findByRole("button", { name: "Reconciliation, 2 unfinished" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Upcoming actions, 2 need attention" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Investment review, 1 to handle" })).toBeVisible();

    services.sessions.mockRejectedValueOnce(new Error("offline"));
    await queryClient.refetchQueries({ queryKey: ["reconciliation", "sessions", ""] });

    expect(screen.getByRole("button", { name: "Reconciliation, 2 unfinished" })).toBeVisible();
    expect(await screen.findByText(i18n.t("tasks.summaryStale"))).toBeVisible();
    expect(screen.getAllByText(i18n.t("tasks.summaryStale"))).toHaveLength(1);
    expect(screen.queryByText(i18n.t("tasks.summaryUnavailable", { section: "Reconciliation" }))).not.toBeInTheDocument();

    services.sessions.mockResolvedValue([{ id: "open", status: "in_progress" }]);
    await userEvent.click(screen.getByRole("button", { name: i18n.t("tasks.summaryStaleRetry", { section: "Reconciliation" }) }));
    expect(await screen.findByRole("button", { name: "Reconciliation, 1 unfinished" })).toBeVisible();
    expect(screen.queryByText(i18n.t("tasks.summaryStale"))).not.toBeInTheDocument();
    expect(open).not.toHaveBeenCalled();
  });

  it("shows an initial failure without a number, and a later failure beside the old number", async () => {
    services.upcoming.mockRejectedValueOnce(new Error("offline"));
    mount();
    expect(await screen.findByRole("button", { name: i18n.t("tasks.summaryUnavailable", { section: "Upcoming actions" }) })).toBeVisible();
    expect(screen.queryByText(i18n.t("tasks.summaryStale"))).not.toBeInTheDocument();

    services.alerts.mockResolvedValue(reviewSummary(1));
    expect(await screen.findByRole("button", { name: "Investment review, 1 to handle" })).toBeVisible();
    services.alerts.mockRejectedValueOnce(new Error("offline"));
    await queryClient.refetchQueries({ queryKey: tasksReviewCountQueryKey() });
    expect(screen.getByRole("button", { name: "Investment review, 1 to handle" })).toBeVisible();
    expect(await screen.findByText(i18n.t("tasks.summaryStale"))).toBeVisible();
    expect(screen.getAllByRole("button", { name: i18n.t("tasks.summaryUnavailable", { section: "Upcoming actions" }) })).toHaveLength(1);
  });
});

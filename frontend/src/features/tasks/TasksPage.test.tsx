import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { ClipboardCheck } from "lucide-react";
import { AppShell } from "@/app/AppShell";
import { NavigationContext } from "@/app/NavigationContext";
import { NAV_GROUPS, resolveLegacyTarget, type TasksTab } from "@/app/navigation";
import { registerSheetConfirmation, resolveSheetConfirmation } from "@/components/ui/sheet-confirmation";
import i18n from "@/i18n";
import { createTestQueryClient } from "@/test/queryClient";
import { upcomingData } from "@/test/upcomingActions";
import { useUiStore } from "@/stores/ui";
import { formatTasksBadge, resolveTasksTab } from "./useTasksCounts";
import { TasksPage } from "./TasksPage";

const services = vi.hoisted(() => ({
  upcoming: vi.fn(),
  alerts: vi.fn(),
  rules: vi.fn(),
  sessions: vi.fn(),
  reconcile: vi.fn(),
}));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({
  Service: {
    UpcomingActions: services.upcoming,
    ListReviewAlertsV2: services.alerts,
    ListReviewRulesV2: services.rules,
    ListReviewCoverageV2: () => Promise.resolve({ items: [], reportRevision: "coverage", projectionStatus: "complete" }),
    ListReviewDailyEvidenceV2: () => Promise.resolve({ items: [] }),
    ListReviewEpisodeHistoryV2: () => Promise.resolve({ items: [] }),
    ListReviewEpisodeEventsV2: () => Promise.resolve({ items: [] }),
    ListReviewSuppressionsV2: () => Promise.resolve([]),
    ReconcileInvestmentReviewsV2: services.reconcile,
    AcknowledgeReviewEpisodeV2: vi.fn(),
    SetReviewSuppressionV2: vi.fn(),
    ClearReviewSuppressionV2: vi.fn(),
    CreateReviewRuleV2: vi.fn(),
    UpdateReviewRuleV2: vi.fn(),
    ArchiveReviewRuleV2: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation", () => ({
  Service: { List: services.sessions },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({
  Service: { HistoryOrigin: () => Promise.resolve({ id: "origin", timezone: "UTC", startedAt: "2026-01-01T00:00:00Z" }) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: {
    Load: () => Promise.resolve({ language: "en", appearance: "system", timezone: "UTC", weekStart: "monday", dateFormat: "iso" }),
    Save: () => Promise.resolve(),
  },
}));
vi.mock("@/queries/investments", () => ({ useInstruments: () => ({ data: [], isPending: false }) }));
vi.mock("@/features/reconciliation/ReconciliationPage", () => ({
  ReconciliationPage: ({ accountId, sessionId, isActive }: { accountId?: string; sessionId?: string; isActive?: boolean }) => (
    <section aria-label="Reconciliation session">{isActive ? "active" : "inactive"} {accountId ?? "-"} {sessionId ?? "-"}</section>
  ),
}));

const queryClient = createTestQueryClient({ retry: false });
const settings = {
  appearance: "system",
  language: "en",
  timezone: "UTC",
  weekStart: "monday",
  dateFormat: "iso",
} as never;

function reviewSummary(actionable: number) {
  return {
    items: [],
    nextCursor: "",
    summary: { totalGroups: 0, matchedGroups: 0, pendingGroups: 0, acknowledgedGroups: 0, ignoredGroups: 0, unknownGroups: 0, pausedGroups: 0, actionableRules: actionable },
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

function TasksHarness({ initial = "upcoming" as TasksTab, accountId, sessionId }: { initial?: TasksTab; accountId?: string; sessionId?: string }) {
  const [tab, setTab] = useState(initial);
  return <TasksPage tab={tab} onTabChange={setTab} accountId={accountId} sessionId={sessionId} />;
}

function mountTasks(props?: { initial?: TasksTab; accountId?: string; sessionId?: string }) {
  return render(
    <QueryClientProvider client={queryClient}>
      <NavigationContext.Provider value={{ open: vi.fn(), openHealth: vi.fn() }}>
        <TasksHarness {...props} />
      </NavigationContext.Provider>
    </QueryClientProvider>,
  );
}

beforeEach(async () => {
  await i18n.changeLanguage("en");
  queryClient.clear();
  vi.clearAllMocks();
  services.upcoming.mockResolvedValue(counts(0));
  services.alerts.mockResolvedValue(reviewSummary(0));
  services.rules.mockResolvedValue([]);
  services.sessions.mockResolvedValue([]);
  services.reconcile.mockResolvedValue({});
  useUiStore.setState({ sidebarCollapsed: false, appearance: "system", accent: "indigo" });
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })) as unknown as typeof window.matchMedia;
});

describe("task tab choice", () => {
  it("uses the deep link, then the last tab, then the first urgent badge", () => {
    expect(resolveTasksTab({ explicit: "reconciliation", previous: "review", attention: 4, actionable: 2 })).toBe("reconciliation");
    expect(resolveTasksTab({ previous: "review", attention: 4, actionable: 2 })).toBe("review");
    expect(resolveTasksTab({ previous: null, attention: 1, actionable: 9 })).toBe("upcoming");
    expect(resolveTasksTab({ previous: null, attention: 0, actionable: 2 })).toBe("review");
    expect(resolveTasksTab({ previous: null, attention: 0, actionable: 0 })).toBe("upcoming");
    expect(formatTasksBadge(0)).toBe("0");
    expect(formatTasksBadge(99)).toBe("99");
    expect(formatTasksBadge(100)).toBe("99+");
  });
});

describe("Tasks & checks page", () => {
  it("orders the tabs, keeps zero badges hidden, and does not switch after counts arrive", async () => {
    services.alerts.mockResolvedValue(reviewSummary(4));
    mountTasks();
    const tabs = await screen.findAllByRole("tab");
    expect(tabs.map((tab) => tab.textContent?.replace(/\d.*/, "").trim())).toEqual([
      i18n.t("tasks.tabs.upcoming"),
      i18n.t("tasks.tabs.review"),
      i18n.t("tasks.tabs.reconciliation"),
    ]);
    expect(tabs[0]).toHaveAttribute("aria-selected", "true");
    await waitFor(() => expect(screen.getByTestId("tasks-tab-badge-review")).toHaveTextContent("4"));
    expect(screen.queryByTestId("tasks-tab-badge-upcoming")).not.toBeInTheDocument();
    expect(screen.queryByTestId("tasks-tab-badge-reconciliation")).not.toBeInTheDocument();
    expect(tabs[0]).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByText(i18n.t("tasks.allClear"))).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Reconciliation session" })).not.toBeInTheDocument();
  });

  it("shows 99+ and a neutral reconciliation badge, and announces the real count", async () => {
    services.upcoming.mockResolvedValue(counts(120));
    services.sessions.mockResolvedValue([
      { id: "open", status: "in_progress" },
      { id: "paused", status: "paused" },
      { id: "done", status: "completed" },
    ]);
    mountTasks();
    expect(await screen.findByTestId("tasks-tab-badge-upcoming")).toHaveTextContent("99+");
    expect(screen.getByTestId("tasks-tab-badge-reconciliation")).toHaveTextContent("2");
    expect(screen.getByTestId("tasks-tab-badge-reconciliation")).toHaveClass("bg-muted");
    expect(screen.getByRole("tab", { name: /Upcoming actions/ })).toHaveAccessibleName(new RegExp(i18n.t("tasks.badge", { count: 120 })));
  });

  it("says the page is clear only after every badge is zero", async () => {
    mountTasks();
    expect(await screen.findByText(i18n.t("tasks.allClear"))).toBeVisible();
    expect(screen.queryByTestId("tasks-tab-badge-upcoming")).not.toBeInTheDocument();
  });

  it("moves across tabs with the arrow keys and mounts reconciliation on first visit", async () => {
    const user = userEvent.setup();
    mountTasks();
    const upcoming = await screen.findByRole("tab", { name: /Upcoming actions/ });
    const review = screen.getByRole("tab", { name: /Investment review/ });
    const reconciliation = screen.getByRole("tab", { name: /Reconciliation/ });
    upcoming.focus();
    await user.keyboard("{ArrowRight}");
    expect(review).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByRole("region", { name: "Reconciliation session" })).not.toBeInTheDocument();
    await user.keyboard("{ArrowRight}");
    expect(reconciliation).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("region", { name: "Reconciliation session" })).toHaveTextContent("active");
    await user.keyboard("{Home}");
    expect(upcoming).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("region", { name: "Reconciliation session", hidden: true })).toHaveTextContent("inactive");
  });

  it("opens a legacy reconciliation target on that session", async () => {
    const target = resolveLegacyTarget({ page: "reconciliation", accountId: "bank-1", sessionId: "session-9" });
    expect(target).toMatchObject({ page: "tasks", tab: "reconciliation", accountId: "bank-1", sessionId: "session-9" });
    if (target.page !== "tasks") throw new Error("legacy reconciliation did not resolve to tasks");
    mountTasks({ initial: target.tab, accountId: target.accountId, sessionId: target.sessionId });
    expect(await screen.findByRole("region", { name: "Reconciliation session" })).toHaveTextContent("active bank-1 session-9");
    expect(screen.getByRole("tab", { name: /Reconciliation/ })).toHaveAttribute("aria-selected", "true");
  });

  it("blocks a tab change while a sheet confirmation is unresolved", async () => {
    const owner = {};
    registerSheetConfirmation(owner);
    const user = userEvent.setup();
    try {
      mountTasks();
      await user.click(await screen.findByRole("tab", { name: /Investment review/ }));
      expect(screen.getByRole("tab", { name: /Upcoming actions/ })).toHaveAttribute("aria-selected", "true");
    } finally {
      resolveSheetConfirmation(owner);
    }
  });
});

describe("sidebar tasks badge", () => {
  it("uses ClipboardCheck and shows the urgent total, a collapsed dot, and no reconciliation count", async () => {
    expect(NAV_GROUPS.find((group) => group.id === "activity")?.items[0]?.icon).toBe(ClipboardCheck);
    services.upcoming.mockResolvedValue(counts(80));
    services.alerts.mockResolvedValue(reviewSummary(25));
    services.sessions.mockResolvedValue([{ id: "open", status: "in_progress" }]);
    const view = render(
      <QueryClientProvider client={queryClient}>
        <AppShell activePageId="overview" onNavigate={vi.fn()} settings={settings}>page</AppShell>
      </QueryClientProvider>,
    );
    const nav = await screen.findByRole("navigation", { name: i18n.t("ui.navigation.main") });
    const button = await withinNav(nav, new RegExp(i18n.t("tasks.sidebarLabel", { page: i18n.t("nav.tasks"), count: 105 })));
    expect(button).toBeInTheDocument();
    expect(await screen.findByTestId("tasks-sidebar-count")).toHaveTextContent("99+");
    expect(screen.queryByTestId("tasks-sidebar-dot")).not.toBeInTheDocument();
    useUiStore.setState({ sidebarCollapsed: true });
    view.rerender(
      <QueryClientProvider client={queryClient}>
        <AppShell activePageId="overview" onNavigate={vi.fn()} settings={settings}>page</AppShell>
      </QueryClientProvider>,
    );
    expect(await screen.findByTestId("tasks-sidebar-dot")).toBeInTheDocument();
    expect(screen.queryByTestId("tasks-sidebar-count")).not.toBeInTheDocument();
  });
});

function withinNav(nav: HTMLElement, name: RegExp) {
  return waitFor(() => {
    const match = [...nav.querySelectorAll("button")].find((button) => name.test(button.getAttribute("aria-label") ?? ""));
    expect(match).toBeTruthy();
    return match!;
  });
}

import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NavigationContext } from "@/app/NavigationContext";
import { createTestQueryClient } from "@/test/queryClient";
import { upcomingData } from "@/test/upcomingActions";
import i18n from "@/i18n";
import { reviewV2Key } from "@/queries/investmentReviewV2";
import { UpcomingActionsPage } from "./UpcomingActionsPage";

const services = vi.hoisted(() => ({
  upcoming: vi.fn(), alerts: vi.fn(), rules: vi.fn(), coverage: vi.fn(), evidence: vi.fn(), history: vi.fn(), events: vi.fn(), suppressions: vi.fn(), reconcile: vi.fn(),
}));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({
  Service: {
    UpcomingActions: services.upcoming,
    ListReviewAlertsV2: services.alerts,
    ListReviewRulesV2: services.rules,
    ListReviewCoverageV2: services.coverage,
    ListReviewDailyEvidenceV2: services.evidence,
    ListReviewEpisodeHistoryV2: services.history,
    ListReviewEpisodeEventsV2: services.events,
    ListReviewSuppressionsV2: services.suppressions,
    ReconcileInvestmentReviewsV2: services.reconcile,
    AcknowledgeReviewEpisodeV2: vi.fn(),
    SetReviewSuppressionV2: vi.fn(),
    ClearReviewSuppressionV2: vi.fn(),
    CreateReviewRuleV2: vi.fn(),
    UpdateReviewRuleV2: vi.fn(),
    ArchiveReviewRuleV2: vi.fn(),
  },
}));

function reviewGroup(instrumentId: string, displayName: string) {
  return {
    instrumentId, displayName, instrumentType: "stock", currency: "USD", status: "pending",
    rules: [{
      ruleId: `${instrumentId}-rule`, name: "Daily threshold", severity: "warning", enabled: true,
      episodeId: `${instrumentId}-episode`, episodeRevision: 3, triggeredOn: "2026-10-15", firstHitOn: "2026-10-15", firstDetectedAt: "2026-10-15T12:00:00Z",
      latestClosedOn: "2026-10-15", window: { truth: "true", windowStart: "2026-10-15", windowEnd: "2026-10-15", hitCount: 1, falseCount: 0, unknownCount: 0, consecutiveCount: 1 },
      status: "pending", actionable: true, triggerPolicy: { kind: "single" }, coverage: [],
    }],
  };
}

function reviewPage(cursor: string) {
  const hasCursor = Boolean(cursor);
  return {
    items: [reviewGroup(hasCursor ? "synthetic-second" : "synthetic-first", hasCursor ? "Synthetic second holding" : "Synthetic fund")],
    nextCursor: hasCursor ? "" : "review-page-two",
    summary: { totalGroups: 2, matchedGroups: 2, pendingGroups: 2, acknowledgedGroups: 0, ignoredGroups: 0, unknownGroups: 0, pausedGroups: 0, actionableRules: 2 },
    reportRevision: "synthetic-review-v2", projectionStatus: "complete", publishedOn: "2026-10-15",
  };
}

const open = vi.fn();
const queryClient = createTestQueryClient({ retry: false });
function tree(isActive = true) {
  return <QueryClientProvider client={queryClient}><NavigationContext.Provider value={{ open, openHealth: vi.fn() }}><UpcomingActionsPage isActive={isActive} /></NavigationContext.Provider></QueryClientProvider>;
}

beforeEach(async () => {
  await i18n.changeLanguage("en");
  vi.clearAllMocks();
  queryClient.clear();
  services.upcoming.mockResolvedValue(upcomingData());
  services.alerts.mockImplementation(async (request: { cursor?: string }) => reviewPage(request.cursor ?? ""));
  services.rules.mockResolvedValue([]);
  services.coverage.mockResolvedValue({ items: [], reportRevision: "synthetic-coverage", projectionStatus: "complete" });
  services.evidence.mockResolvedValue({ items: [], episodeId: "", reportRevision: "synthetic-evidence" });
  services.history.mockResolvedValue({ items: [], nextCursor: "", reportRevision: "synthetic-history", ruleId: "" });
  services.events.mockResolvedValue({ items: [], nextCursor: "", reportRevision: "synthetic-events", episodeId: "" });
  services.suppressions.mockResolvedValue([]);
  services.reconcile.mockResolvedValue({});
});

describe("Upcoming Actions investment review entry", () => {
  it("uses the V2 page, keeps controls and paging state through tab changes, and suspends hidden reads", async () => {
    const user = userEvent.setup();
    const view = render(tree());
    expect(services.alerts).not.toHaveBeenCalled();
    expect(services.rules).not.toHaveBeenCalled();
    await user.click(screen.getByRole("tab", { name: "Investment review" }));
    expect(screen.getByRole("tab", { name: "Investment review" })).toHaveFocus();
    expect(await screen.findByRole("heading", { name: "Synthetic fund" })).toBeInTheDocument();
    expect(services.alerts).toHaveBeenCalledWith(expect.objectContaining({ filter: { status: "unignored", instrumentType: "", query: "" } }));
    expect(screen.getByRole("button", { name: "Refresh" })).toBeVisible();
    expect(screen.getByRole("button", { name: "History" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Ignored investments" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Rules" })).toBeVisible();

    await user.type(screen.getByRole("textbox", { name: "Search investments" }), "synthetic");
    await user.selectOptions(screen.getByRole("combobox", { name: "Type" }), "stock");
    await user.click(screen.getByRole("button", { name: "To review" }));
    await waitFor(() => expect(services.alerts).toHaveBeenCalledWith(expect.objectContaining({ filter: { status: "pending", instrumentType: "stock", query: "synthetic" } })));
    await user.click(screen.getByRole("button", { name: "Next" }));
    expect(await screen.findByRole("heading", { name: "Synthetic second holding" })).toBeInTheDocument();
    expect(services.alerts).toHaveBeenCalledWith(expect.objectContaining({ cursor: "review-page-two" }));

    await user.click(screen.getByRole("button", { name: "Rules" }));
    expect(await screen.findByRole("button", { name: "New rule" })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("tab", { name: "Upcoming actions" }));
    expect(screen.getByRole("tab", { name: "Upcoming actions" })).toHaveFocus();
    const inactiveCallCount = services.alerts.mock.calls.length;
    await user.click(screen.getByRole("tab", { name: "Investment review" }));
    expect(screen.getByRole("tab", { name: "Investment review" })).toHaveFocus();
    expect(screen.getByRole("textbox", { name: "Search investments" })).toHaveValue("synthetic");
    expect(screen.getByRole("combobox", { name: "Type" })).toHaveValue("stock");
    expect(screen.getByRole("button", { name: "To review" })).toHaveAttribute("aria-pressed", "true");
    expect(await screen.findByRole("heading", { name: "Synthetic second holding" })).toBeInTheDocument();
    expect(services.alerts.mock.calls.length).toBeGreaterThanOrEqual(inactiveCallCount);

    view.rerender(tree(false));
    const beforeInvalidate = services.alerts.mock.calls.length;
    await queryClient.invalidateQueries({ queryKey: reviewV2Key });
    expect(services.alerts).toHaveBeenCalledTimes(beforeInvalidate);
    view.rerender(tree(true));
    await waitFor(() => expect(services.alerts.mock.calls.length).toBeGreaterThan(beforeInvalidate));
  });

  it("opens holdings and performance from the new review page with only an instrument id and section", async () => {
    const user = userEvent.setup();
    render(tree());
    await user.click(screen.getByRole("tab", { name: "Investment review" }));
    await screen.findByRole("heading", { name: "Synthetic fund" });
    await user.click(screen.getByRole("button", { name: "More actions for Synthetic fund" }));
    await user.click(await screen.findByRole("menuitem", { name: "View holdings" }));
    expect(open).toHaveBeenLastCalledWith({ page: "instrument-detail", instrumentId: "synthetic-first", section: "holdings" });
    await waitFor(() => expect(screen.queryByRole("menuitem", { name: "View holdings" })).not.toBeInTheDocument());
    await user.click(screen.getByRole("button", { name: "More actions for Synthetic fund" }));
    await user.click(await screen.findByRole("menuitem", { name: "View performance" }));
    expect(open).toHaveBeenLastCalledWith({ page: "instrument-detail", instrumentId: "synthetic-first", section: "performance" });
    expect(open.mock.calls.flatMap(([target]) => Object.keys(target))).not.toContain("accountId");
    expect(open.mock.calls.flatMap(([target]) => Object.keys(target))).not.toContain("from");
    expect(open.mock.calls.flatMap(([target]) => Object.keys(target))).not.toContain("to");
  });
});

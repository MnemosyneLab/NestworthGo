import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/i18n";
import { NavigationContext } from "@/app/NavigationContext";
import type { ReviewAlertPage, ReviewCoveragePage } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import { InvestmentReviewPage } from "./InvestmentReviewPage";

const state = vi.hoisted(() => {
  const mutation = () => ({ mutateAsync: vi.fn(async () => ({})), isPending: false });
  return {
    filter: null as unknown,
    coverageFilter: null as unknown,
    alert: { data: null as ReviewAlertPage | null, isPending: false, isError: false, isFetching: false, error: null, refetch: vi.fn() },
    evidence: { data: { items: [{ on: "2026-10-15", truth: "true", leaves: [] }], reportRevision: "evidence-1", episodeId: "episode-one", nextCursor: "" }, isPending: false, isError: false, isFetching: false },
    coverage: { data: null as ReviewCoveragePage | null, isPending: false, isError: false, isFetching: false, refetch: vi.fn() },
    rules: { data: [], isPending: false, isError: false, hasNextPage: false, isFetchingNextPage: false, fetchNextPage: vi.fn() },
    suppressions: { data: [], isPending: false, isError: false, hasNextPage: false, isFetchingNextPage: false, fetchNextPage: vi.fn() },
    commands: { acknowledge: mutation(), suppress: mutation(), clearSuppression: mutation(), reconcile: mutation(), createRule: mutation(), updateRule: mutation(), archiveRule: mutation() },
  };
});
vi.mock("@/queries/investmentReviewV2", () => ({
  useReviewAlertPage: (filter: unknown) => { state.filter = filter; return state.alert; },
  useReviewDailyEvidencePage: () => state.evidence,
  useReviewCoveragePage: (filter: unknown) => { state.coverageFilter = filter; return state.coverage; },
  useReviewRulesV2: () => state.rules,
  useReviewSuppressionsV2: () => state.suppressions,
  useReviewV2Commands: () => state.commands,
}));
vi.mock("./ReviewRuleEditor", () => ({ ReviewRuleEditor: () => null }));

function report(): ReviewAlertPage {
  return {
    reportRevision: "published-synthetic-1", projectionStatus: "complete", publishedOn: "2026-10-15", nextCursor: "next-page",
    summary: { totalGroups: 3, matchedGroups: 1, pendingGroups: 1, acknowledgedGroups: 1, ignoredGroups: 1, unknownGroups: 0, pausedGroups: 0, actionableRules: 2 },
    items: [{ instrumentId: "synthetic-investment", displayName: "Synthetic fund", instrumentType: "mutual_fund" as never, currency: "USD",
      status: "pending", rules: [
        { ruleId: "rule-one", name: "Daily amount", severity: "warning", enabled: true, episodeId: "episode-one", episodeRevision: 4, triggeredOn: "2026-10-12", firstHitOn: "2026-10-12", firstDetectedAt: "2026-10-12T12:00:00Z", latestClosedOn: "2026-10-15", window: { truth: "true" as never, windowStart: "2026-10-12", windowEnd: "2026-10-15", hitCount: 4, falseCount: 0, unknownCount: 0, consecutiveCount: 4 }, status: "pending", actionable: true, triggerPolicy: { kind: "consecutive", count: 3 }, coverage: [], latestResult: { on: "2026-10-15", truth: "true" as never, leaves: [{ metric: "amount", truth: "true" as never, actual: "1200", threshold: "1000", currency: "USD" }] } },
        { ruleId: "rule-two", name: "Price loss", severity: "notice", enabled: true, episodeId: "episode-two", episodeRevision: 2, triggeredOn: "2026-10-15", firstHitOn: "2026-10-15", firstDetectedAt: "2026-10-15T12:00:00Z", latestClosedOn: "2026-10-15", window: { truth: "true" as never, windowStart: "2026-10-06", windowEnd: "2026-10-15", hitCount: 5, falseCount: 3, unknownCount: 2, consecutiveCount: 1 }, status: "pending", actionable: true, triggerPolicy: { kind: "rolling", windowDays: 10, minHits: 5 }, coverage: [] },
      ] }],
  };
}

const open = vi.fn();
function tree() { return <NavigationContext.Provider value={{ open, openHealth: vi.fn() }}><InvestmentReviewPage /></NavigationContext.Provider>; }
function mount() { return render(tree()); }
beforeEach(async () => {
  vi.clearAllMocks();
  await i18n.changeLanguage("en");
  state.alert.data = report(); state.alert.isPending = state.alert.isError = state.alert.isFetching = false;
  state.rules.data = [];
  state.rules.hasNextPage = false;
  state.suppressions.hasNextPage = false;
  state.coverage.data = { items: [{ instrumentId: "synthetic-gap", displayName: "Synthetic gap", instrumentType: "stock" as never, status: "unknown", rules: [{ ruleId: "rule-gap", name: "Missing price", state: "unknown", latestClosedOn: "2026-10-15", window: { truth: "" as never, windowStart: "2026-10-06", windowEnd: "2026-10-15", hitCount: 0, falseCount: 0, unknownCount: 10, consecutiveCount: 0 }, missing: ["quote_missing"] }] }], reportRevision: "coverage-1", projectionStatus: "complete", publishedOn: "2026-10-15" };
});

describe("daily investment review page", () => {
  it.each([
    ["complete", false],
    ["pending", true],
    ["error", true],
  ] as const)("shows the alert projection-status banner only when status is %s", (projectionStatus, visible) => {
    state.alert.data = { ...report(), projectionStatus };
    mount();
    const banner = screen.queryByText(i18n.t("reviewV2.projectionStatus", { status: projectionStatus }));
    if (visible) expect(banner).toBeVisible();
    else expect(banner).not.toBeInTheDocument();
  });
  it("loads the next rule and suppression pages on demand", async () => {
    state.rules.hasNextPage = true;
    state.suppressions.hasNextPage = true;
    mount();
    await userEvent.click(screen.getByRole("button", { name: "Rules" }));
    await userEvent.click(screen.getByRole("button", { name: "Load more" }));
    expect(state.rules.fetchNextPage).toHaveBeenCalledOnce();
    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    await userEvent.click(screen.getByRole("button", { name: "Ignored investments" }));
    await userEvent.click(screen.getByRole("button", { name: "Load more" }));
    expect(state.suppressions.fetchNextPage).toHaveBeenCalledOnce();
  });
  it("labels a migrated legacy rule without opening the daily rule editor for it", async () => {
    state.rules.data = [{ id: "legacy-rule", evaluator: "legacy_v1", enabled: true, rowRevision: 1, archivedAt: null }] as never;
    mount();
    await userEvent.click(screen.getByRole("button", { name: "Rules" }));
    expect(screen.getByText(i18n.t("reviewV2.legacyRuleHelp"))).toBeVisible();
    expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
  });
  it.each(["en", "zh-CN", "zh-TW"])("groups every alert for one investment and explains its fixed window in %s", async language => {
    await i18n.changeLanguage(language);
    mount();
    expect(screen.getAllByRole("heading", { name: "Synthetic fund" })).toHaveLength(1);
    expect(screen.getByText("Daily amount")).toBeVisible();
    expect(screen.getByText("Price loss")).toBeVisible();
    expect(screen.getByText(i18n.t("reviewV2.windowConsecutive", { days: 4, needed: 3 }))).toBeVisible();
    expect(screen.getByText(i18n.t("reviewV2.windowRolling", { hits: 5, needed: 5, days: 10, unknowns: 2 }))).toBeVisible();
    await userEvent.click(screen.getAllByText(i18n.t("reviewV2.why"))[0]);
    expect(screen.getByText(i18n.t("reviewV2.windowCounts", { hits: 4, falseCount: 0, unknowns: 0 }))).toBeVisible();
    expect(screen.getByText(i18n.t("reviewV2.summaryCounts", { total: 3, matched: 1, pending: 1, actionable: 2, unknown: 0 }))).toBeVisible();
    expect(screen.getAllByText("2026-10-15 · " + i18n.t("reviewV2.dailyTrue"))).toHaveLength(2);
    expect(screen.queryByText(/reviewV2\./)).not.toBeInTheDocument();
  });
  it("keeps status and type filters independent and acknowledges exactly one episode revision", async () => {
    mount();
    expect(state.filter).toEqual({ status: "unignored", instrumentType: "", query: "" });
    await userEvent.click(screen.getByRole("button", { name: "Acknowledged" }));
    expect(state.filter).toEqual({ status: "acknowledged", instrumentType: "", query: "" });
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Type" }), "stock");
    expect(state.filter).toEqual({ status: "acknowledged", instrumentType: "stock", query: "" });
    await userEvent.click(screen.getAllByRole("button", { name: "Got it" })[0]);
    await waitFor(() => expect(state.commands.acknowledge.mutateAsync).toHaveBeenCalledWith({ episodeId: "episode-one", expectedRevision: 4 }));
  });
  it("shows a missing-data holding with no open episode in the coverage view", async () => {
    mount();
    await userEvent.click(screen.getByRole("button", { name: "Data missing" }));
    expect(state.coverageFilter).toEqual({ status: "unknown", instrumentType: "", query: "" });
    expect(screen.getByRole("heading", { name: "Synthetic gap" })).toBeVisible();
    expect(screen.getByText("Missing price · Data missing")).toBeVisible();
    expect(screen.queryByText("Synthetic fund")).not.toBeInTheDocument();
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Type" }), "stock");
    expect(state.coverageFilter).toEqual({ status: "unknown", instrumentType: "stock", query: "" });
  });
  it("requires an explicit permanent-ignore confirmation for all rules", async () => {
    mount();
    await userEvent.click(screen.getByRole("button", { name: "More actions for Synthetic fund" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Ignore permanently" }));
    expect(screen.getByText(i18n.t("reviewV2.permanentConfirm", { name: "Synthetic fund" }))).toBeVisible();
    expect(state.commands.suppress.mutateAsync).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Ignore permanently" }));
    await waitFor(() => expect(state.commands.suppress.mutateAsync).toHaveBeenCalledWith({ instrumentId: "synthetic-investment", mode: "permanent" }));
  });
  it("returns focus to the active filter after the acknowledged row leaves the report", async () => {
    const view = mount();
    const button = screen.getAllByRole("button", { name: "Got it" })[0];
    button.focus();
    await userEvent.keyboard("{Enter}");
    state.alert.data = { ...report(), reportRevision: "published-synthetic-2", items: [] };
    view.rerender(tree());
    expect(screen.getByRole("button", { name: "Not ignored" })).toHaveFocus();
  });
  it("does not steal focus if the user moved away while acknowledgement completed", async () => {
    const view = mount();
    screen.getAllByRole("button", { name: "Got it" })[0].focus();
    await userEvent.keyboard("{Enter}");
    const refresh = screen.getByRole("button", { name: "Refresh" });
    refresh.focus();
    state.alert.data = { ...report(), reportRevision: "published-synthetic-2", items: [] };
    view.rerender(tree());
    expect(refresh).toHaveFocus();
  });
});

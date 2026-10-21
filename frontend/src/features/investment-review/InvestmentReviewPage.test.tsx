import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/i18n";
import { resetDisplayPreferences, setDisplayPreferences } from "@/lib/displayPreferences";
import { localDateInTimeZone } from "@/features/history/historyStartDate";
import { NavigationContext } from "@/app/NavigationContext";
import type { ReviewAlertPage, ReviewCoveragePage } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import { InvestmentReviewPage } from "./InvestmentReviewPage";

const state = vi.hoisted(() => {
  const mutation = () => ({ mutateAsync: vi.fn(async () => ({})), get isPending() { return state.suppressPending; } });
  return {
    suppressPending: false,
    origin: { timezone: "Asia/Singapore" },
    filter: null as unknown,
    coverageFilter: null as unknown,
    instruments: { data: [] as Array<{ id: string; name: string; shortName?: string; symbol?: string }>, isPending: false },
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
vi.mock("@/queries/investments", () => ({ useInstruments: () => state.instruments }));
vi.mock("@/queries/history", () => ({ useHistoryOrigin: () => ({ data: state.origin }) }));
vi.mock("@/queries/settings", () => ({ useSettings: () => ({ data: { weekStart: "monday", dateFormat: "iso" } }) }));
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

const defaultEvidence = structuredClone(state.evidence.data);
const open = vi.fn();
function tree() { return <NavigationContext.Provider value={{ open, openHealth: vi.fn() }}><InvestmentReviewPage /></NavigationContext.Provider>; }
function mount() { return render(tree()); }
beforeEach(async () => {
  vi.clearAllMocks();
  state.evidence.data = structuredClone(defaultEvidence);
  state.instruments.data = []; state.instruments.isPending = false;
  state.suppressPending = false;
  state.origin = { timezone: "Asia/Singapore" };
  await i18n.changeLanguage("en");
  state.alert.data = report(); state.alert.isPending = state.alert.isError = state.alert.isFetching = false;
  state.rules.data = [];
  state.rules.hasNextPage = false;
  state.suppressions.hasNextPage = false;
  state.suppressions.data = [];
  state.coverage.data = { items: [{ instrumentId: "synthetic-gap", displayName: "Synthetic gap", instrumentType: "stock" as never, status: "unknown", rules: [{ ruleId: "rule-gap", name: "Missing price", state: "unknown", latestClosedOn: "2026-10-15", window: { truth: "" as never, windowStart: "2026-10-06", windowEnd: "2026-10-15", hitCount: 0, falseCount: 0, unknownCount: 10, consecutiveCount: 0 }, missing: ["quote_missing"] }] }], reportRevision: "coverage-1", projectionStatus: "complete", publishedOn: "2026-10-15" };
});

afterEach(() => resetDisplayPreferences());

describe("daily investment review page", () => {
  it.each([
    ["day-first", "15/10/2026", "15/11/2026"],
    ["month-first", "10/15/2026", "11/15/2026"],
  ] as const)("formats publication and both suppression dates using %s", async (dateFormat, published, until) => {
    setDisplayPreferences({ dateFormat, timezone: "America/Los_Angeles" });
    state.alert.data!.items![0].rules![0].suppressedUntil = "2026-11-15";
    state.suppressions.data = [{ ID: "synthetic-suppression", InstrumentID: "synthetic-investment", Mode: "until_date", UntilOn: "2026-11-15", Revision: 1 }] as never[];
    mount();
    expect(screen.getByText(i18n.t("reviewV2.publishedOn", { date: published }))).toBeVisible();
    expect(screen.getByText(i18n.t("reviewV2.suppressedUntil", { date: until }))).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Ignored investments" }));
    expect(screen.getByRole("dialog")).toHaveTextContent(i18n.t("reviewV2.suppressedUntil", { date: until }));
    expect(state.alert.data!.publishedOn).toBe("2026-10-15");
    expect(state.alert.data!.items![0].rules![0].suppressedUntil).toBe("2026-11-15");
    expect(state.suppressions.data[0]).toMatchObject({ UntilOn: "2026-11-15" });
  });
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
  it.each(["en"])("groups every alert for one investment and explains its fixed window in %s", async language => {
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
  it("uses the household civil date to block today and earlier resume dates", async () => {
    const today = localDateInTimeZone("Asia/Singapore")!;
    const yesterdayValue = new Date(`${today}T00:00:00Z`);
    yesterdayValue.setUTCDate(yesterdayValue.getUTCDate() - 1);
    const yesterday = yesterdayValue.toISOString().slice(0, 10);
    const tomorrowValue = new Date(`${today}T00:00:00Z`);
    tomorrowValue.setUTCDate(tomorrowValue.getUTCDate() + 1);
    const tomorrow = tomorrowValue.toISOString().slice(0, 10);
    const dateLabel = (value: string) => new Intl.DateTimeFormat("en-US", { month: "long", day: "numeric", timeZone: "UTC" }).format(new Date(`${value}T12:00:00Z`));
    const user = userEvent.setup();
    mount();
    await user.click(screen.getByRole("button", { name: "More actions for Synthetic fund" }));
    await user.click(await screen.findByRole("menuitem", { name: "Ignore until date" }));
    expect(screen.getByText("Choose a resume date after today in Asia/Singapore.")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Resume on" }));
    expect(screen.getByRole("button", { name: new RegExp(dateLabel(yesterday)) })).toBeDisabled();
    expect(screen.getByRole("button", { name: new RegExp(dateLabel(today)) })).toBeDisabled();
    expect(screen.getByRole("button", { name: new RegExp(dateLabel(tomorrow)) })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: new RegExp(dateLabel(tomorrow)) }));
    expect(screen.getByRole("button", { name: "Ignore until this date" })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "Ignore until this date" }));
    await waitFor(() => expect(state.commands.suppress.mutateAsync).toHaveBeenCalledWith({ instrumentId: "synthetic-investment", mode: "until_date", untilOn: tomorrow }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
  it("keeps the resume date sheet open after structured validation and allows retry or cancel", async () => {
    const today = localDateInTimeZone("Asia/Singapore")!;
    const tomorrowValue = new Date(`${today}T00:00:00Z`);
    tomorrowValue.setUTCDate(tomorrowValue.getUTCDate() + 1);
    const tomorrow = tomorrowValue.toISOString().slice(0, 10);
    const dateLabel = (value: string) => new Intl.DateTimeFormat("en-US", { month: "long", day: "numeric", timeZone: "UTC" }).format(new Date(`${value}T12:00:00Z`));
    const wireError = { code: "validation", field: "untilOn", message: "secret raw backend message" };
    state.commands.suppress.mutateAsync.mockRejectedValueOnce(Object.assign(new Error(JSON.stringify(wireError)), { wireError }));
    const user = userEvent.setup();
    mount();
    await user.click(screen.getByRole("button", { name: "More actions for Synthetic fund" }));
    await user.click(await screen.findByRole("menuitem", { name: "Ignore until date" }));
    await user.click(screen.getByRole("button", { name: "Resume on" }));
    await user.click(screen.getByRole("button", { name: new RegExp(dateLabel(tomorrow)) }));
    await user.click(screen.getByRole("button", { name: "Ignore until this date" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Choose a resume date after today in Asia/Singapore.");
    expect(screen.getByRole("alert")).not.toHaveTextContent("secret raw backend message");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    const resumeOn = screen.getByRole("button", { name: "Resume on" });
    expect(resumeOn).toHaveTextContent(tomorrow);
    expect(resumeOn).toHaveAttribute("aria-invalid", "true");
    expect(resumeOn).toHaveAttribute("aria-describedby", "review-ignore-until-help review-ignore-until-error");
    expect(document.getElementById("review-ignore-until-error")).toHaveTextContent("Choose a resume date after today in Asia/Singapore.");
    let complete!: () => void;
    state.commands.suppress.mutateAsync.mockImplementationOnce(() => {
      state.suppressPending = true;
      return new Promise<object>(resolve => { complete = () => resolve({}); });
    });
    const save = screen.getByRole("button", { name: "Ignore until this date" });
    await user.click(save);
    await waitFor(() => expect(save).toBeDisabled());
    await user.click(save);
    expect(state.commands.suppress.mutateAsync).toHaveBeenCalledTimes(2);
    state.suppressPending = false;
    complete();
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: "More actions for Synthetic fund" }));
    await user.click(await screen.findByRole("menuitem", { name: "Ignore until date" }));
    expect(screen.getByRole("button", { name: "Resume on" })).not.toHaveTextContent(tomorrow);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
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
  it("shows instrument names, never ids, in the ignored investments sheet", async () => {
    state.instruments.data = [{ id: "7b7e0c5e-aaaa-bbbb-cccc-000000000001", name: "Synthetic Global Fund", shortName: "Global Fund" }];
    state.suppressions.data = [
      { ID: "s1", InstrumentID: "7b7e0c5e-aaaa-bbbb-cccc-000000000001", Mode: "permanent", UntilOn: "", Revision: 1 },
      { ID: "s2", InstrumentID: "7b7e0c5e-aaaa-bbbb-cccc-00000000dead", Mode: "until_date", UntilOn: "2026-11-15", Revision: 1 },
    ] as never[];
    mount();
    await userEvent.click(screen.getByRole("button", { name: "Ignored investments" }));
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent("Synthetic Global Fund · Ignore permanently");
    expect(dialog).toHaveTextContent("Unknown investment · Resumes on 2026-11-15");
    expect(dialog).not.toHaveTextContent("7b7e0c5e");
  });
  it("formats evidence amounts and percentages and translates reason codes", async () => {
    const rule = state.alert.data!.items![0].rules![0];
    rule.latestResult = { on: "2026-10-15", truth: "false" as never, leaves: [
      { metric: "amount", truth: "true" as never, actual: "126.5000", currency: "USD" as never },
      { metric: "price_change", truth: "false" as never, actual: "-3.456" },
      { metric: "cost", truth: "unknown" as never, reason: "fx_missing", currency: "USD" as never },
      { metric: "price_change", truth: "unknown" as never, reason: "daily_close_missing" },
    ] };
    rule.coverage = ["historical_cost", "not_a_known_code"];
    state.evidence.data.items = [{ on: "2026-10-15", truth: "unknown" as never, leaves: [{ metric: "amount", truth: "unknown" as never, reason: "daily_close_missing", currency: "USD" as never }] }] as never;
    mount();
    await userEvent.click(screen.getAllByText(i18n.t("reviewV2.why"))[0]);
    expect(screen.getByText("Amount: US$126.50 ✓")).toBeVisible();
    expect(screen.getByText("Price change: -3.46% ×")).toBeVisible();
    expect(screen.getByText("Cost: Unknown ? · Exchange rate missing")).toBeVisible();
    expect(screen.getByText("Price change: Unknown ? · No closing price for this day")).toBeVisible();
    expect(screen.getAllByText("Amount: Unknown · No closing price for this day").length).toBeGreaterThan(0);
    expect(screen.getByText("Cost basis unknown on this day")).toBeVisible();
    expect(screen.getByText("Unknown reason")).toBeVisible();
    expect(screen.queryByText("not_a_known_code")).not.toBeInTheDocument();
    expect(screen.queryByText(/126\.5000|daily_close_missing|fx_missing/)).not.toBeInTheDocument();
  });
  it("uses singular forms for single hits and conditions", async () => {
    const rule = state.alert.data!.items![0].rules![0];
    rule.window = { ...rule.window, hitCount: 1, falseCount: 1, unknownCount: 1, consecutiveCount: 1 };
    state.rules.data = [{ id: "r1", name: "One condition rule", enabled: true, evaluator: "daily_predicate_v1", severity: "warning", rowRevision: 1, spec: { triggerPolicy: { kind: "single" }, schedule: { timezone: "UTC" }, predicate: { all: [{}] } } }] as never[];
    mount();
    await userEvent.click(screen.getAllByText(i18n.t("reviewV2.why"))[0]);
    expect(screen.getByText("1 hit · 1 miss · 1 unknown")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Rules" }));
    expect(await screen.findByText("1 condition · One daily hit · UTC")).toBeVisible();
  });
});

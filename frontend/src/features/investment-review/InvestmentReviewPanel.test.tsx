import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { InvestmentReviewReport } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import { NavigationContext } from "@/app/NavigationContext";
import i18n from "@/i18n";
import { InvestmentReviewPanel, InvestmentReviewSummary } from "./InvestmentReviewPanel";

const state = vi.hoisted(() => ({
  query: { data: undefined as InvestmentReviewReport | undefined, isPending: false, isError: false, isFetching: false, refetch: vi.fn() },
  save: { mutate: vi.fn(), isPending: false, isError: false, isSuccess: false },
  acknowledge: { mutate: vi.fn(), isPending: false, isError: false },
  enroll: { mutate: vi.fn(), isPending: false, isError: false },
  snooze: { mutate: vi.fn(), isPending: false, isError: false },
}));
vi.mock("@/queries/investmentReview", () => ({ useInvestmentReviews: () => state.query, useInvestmentReviewActions: () => state }));
const open = vi.fn();
function fixture(): InvestmentReviewReport {
  return {
    schemaVersion: "investment-review/1", asOf: "2026-10-13T12:00:00Z", localDate: "2026-10-13", timezone: "Asia/Shanghai", revision: 1,
    policy: { enabled: true, windowDays: 30, lowAnnualizedPercent: "1", lossPercent: "-1", sustainDays: 7, maxAgeDays: 7 },
    items: [{ instrumentId: "synthetic-id", displayName: "Synthetic fund", status: "low_yield", baselineOn: "2026-09-07", latestOn: "2026-10-09", targetOn: "2026-09-09", elapsedDays: 32, ageDays: 4, missingDays: 20, observationCount: 13, annualizedPercent: "0.25", periodPercent: "0.0219178082", episodeId: "episode-one", reason: "", acknowledged: false, monitored: true, snoozed: false, snoozedUntilOn: "" }],
  };
}
function mount(element: React.ReactNode = <InvestmentReviewPanel />) { return render(element, { wrapper: ({ children }) => <NavigationContext.Provider value={{ open, openHealth: vi.fn() }}>{children}</NavigationContext.Provider> }); }
beforeEach(async () => {
  vi.clearAllMocks(); await i18n.changeLanguage("en"); state.query.data = fixture();
  state.query.isError = state.query.isPending = state.query.isFetching = false;
  state.save.isError = state.save.isPending = state.save.isSuccess = false;
  state.acknowledge.isError = state.acknowledge.isPending = state.enroll.isError = state.enroll.isPending = state.snooze.isError = state.snooze.isPending = false;
});
describe("investment review UI", () => {
  async function allInvestments() {
    await userEvent.click(screen.getByRole("button", { name: i18n.t("investmentReview.all", { count: state.query.data!.items!.length }) }));
  }
  async function controls() {
    await userEvent.click(screen.getByText(i18n.t("investmentReview.controls", { name: "Synthetic fund" })));
  }
  it.each(["en", "zh-CN", "zh-TW"])("keeps exact estimates and accessible calculation details in %s", async language => {
    await i18n.changeLanguage(language); mount(); const user = userEvent.setup();
    expect(screen.getByText(i18n.t("investmentReview.estimated", { value: "0.25" }))).toBeVisible();
    const summary = screen.getByText(i18n.t("investmentReview.calculation", { name: "Synthetic fund" }));
    const dates = screen.getByText(i18n.t("investmentReview.dates", { latest: "2026-10-09", baseline: "2026-09-07", window: 30, days: 32, age: 4 }));
    expect(dates).not.toBeVisible();
    summary.focus(); await user.click(summary);
    expect(summary.closest("details")).toHaveAttribute("open"); expect(summary).toHaveFocus();
    expect(dates).toBeVisible();
    expect(screen.getByText(i18n.t("investmentReview.period", { value: "0.0219178082" }))).toBeVisible();
    expect(screen.getByText(i18n.t("investmentReview.target", { date: "2026-09-09" }))).toBeVisible();
    expect(screen.getByText(i18n.t("investmentReview.observations", { count: 13, missing: 20 }))).toBeVisible();
    expect(screen.getByText(i18n.t("investmentReview.foreground"))).toBeVisible();
    expect(screen.getByText(i18n.t("investmentReview.priceOnly"))).toBeVisible();
    await user.click(summary); expect(dates).not.toBeVisible(); expect(summary).toHaveFocus();
    expect(screen.queryByText(/investmentReview\./)).not.toBeInTheDocument();
  });
  it("orders pending loss before low yield with stable name and ID ties, keeping other holdings accessible", async () => {
    const item = fixture().items![0];
    state.query.data!.items = [
      { ...item, instrumentId: "yield", displayName: "A yield" },
      { ...item, instrumentId: "z-loss", displayName: "Same loss", status: "loss" },
      { ...item, instrumentId: "stale", displayName: "A stale", status: "stale", episodeId: "" },
      { ...item, instrumentId: "a-loss", displayName: "Same loss", status: "loss" },
      { ...item, instrumentId: "first-loss", displayName: "B loss", status: "loss" },
      { ...item, instrumentId: "off", displayName: "B off", monitored: false, episodeId: "" },
    ];
    const view = mount();
    expect(screen.getAllByRole("heading", { level: 5 }).map(node => node.textContent)).toEqual(["B loss", "Same loss", "Same loss", "A yield"]);
    for (const button of screen.getAllByRole("button", { name: /View NAV/ })) await userEvent.click(button);
    expect(open.mock.calls.map(([destination]) => destination.focus.instrumentId)).toEqual(["first-loss", "a-loss", "z-loss", "yield"]);
    state.query.data!.items!.reverse(); view.rerender(<InvestmentReviewPanel />);
    expect(screen.getAllByRole("heading", { level: 5 }).map(node => node.textContent)).toEqual(["B loss", "Same loss", "Same loss", "A yield"]);
    await allInvestments();
    expect(screen.getAllByRole("heading", { level: 5 }).map(node => node.textContent)).toEqual(["B loss", "Same loss", "Same loss", "A yield", "A stale", "B off"]);
  });
  it.each(["low_yield", "loss"])("keeps %s visible beside acknowledgement without a new action", async status => {
    state.query.data!.items![0].status = status; state.query.data!.items![0].acknowledged = true;
    mount(); expect(screen.getByText(i18n.t("investmentReview.noPending"))).toBeVisible();
    await allInvestments();
    expect(screen.getByText(i18n.t(`investmentReview.status.${status}`))).toBeVisible();
    expect(screen.getByText(i18n.t("investmentReview.status.acknowledged"))).toBeVisible();
    expect(screen.queryByRole("button", { name: /Reviewed/ })).not.toBeInTheDocument();
  });
  it.each(["en", "zh-CN", "zh-TW"])("offers explicit setup and per-investment opt-in without auto-enrollment in %s", async language => {
    await i18n.changeLanguage(language);
    state.query.data!.items![0].monitored = false; state.query.data!.items![0].episodeId = "";
    mount(); expect(screen.getByText(i18n.t("investmentReview.setup"))).toBeVisible();
    expect(screen.getByText(i18n.t("investmentReview.currentPolicy", { window: 30, low: "1", loss: "-1", sustain: 7, age: 7 }))).toBeVisible();
    expect(screen.queryByRole("checkbox", { name: i18n.t("investmentReview.enroll", { name: "Synthetic fund" }) })).not.toBeInTheDocument(); expect(state.enroll.mutate).not.toHaveBeenCalled();
    const user = userEvent.setup(); const choose = screen.getByRole("button", { name: i18n.t("investmentReview.choose") });
    choose.focus(); await user.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: i18n.t("investmentReview.all", { count: 1 }) })).toHaveFocus();
    const checkbox = screen.getByRole("checkbox", { name: i18n.t("investmentReview.enroll", { name: "Synthetic fund" }) });
    expect(checkbox).not.toBeChecked(); expect(state.enroll.mutate).not.toHaveBeenCalled();
    checkbox.focus(); await user.keyboard(" ");
    expect(state.enroll.mutate).toHaveBeenCalledWith({ instrumentId: "synthetic-id", enabled: true, expectedRevision: 1 });
  });
  it("keeps NAV and reviewed actions prominent with exact application payloads", async () => {
    mount();
    await userEvent.click(screen.getByRole("button", { name: "Reviewed · Synthetic fund" }));
    expect(state.acknowledge.mutate).toHaveBeenCalledWith("episode-one");
    await userEvent.click(screen.getByRole("button", { name: "View NAV · Synthetic fund" }));
    expect(open).toHaveBeenCalledWith({ page: "market-data", focus: { instrumentId: "synthetic-id", label: "Synthetic fund" } });
    await controls(); await userEvent.click(screen.getByRole("checkbox", { name: "Enable review reminders · Synthetic fund" }));
    expect(state.enroll.mutate).toHaveBeenCalledWith({ instrumentId: "synthetic-id", enabled: false, expectedRevision: 1 });
  });
  it("lets a keyboard user edit preferences and preserves them across unrelated revisions and view changes", async () => {
    const view = mount(); const user = userEvent.setup();
    const preferences = screen.getByText("Review preferences"); preferences.focus(); await user.keyboard("{Enter}");
    expect(preferences).toHaveFocus();
    await user.selectOptions(screen.getByRole("combobox", { name: "Review window" }), "7");
    const low = screen.getByRole("spinbutton", { name: "Low estimated annualized return threshold (%)" });
    await user.clear(low); await user.type(low, "2.5");
    await allInvestments();
    state.query.data = { ...fixture(), revision: 2, policy: { ...fixture().policy }, items: [{ ...fixture().items![0], snoozed: true, episodeId: "", snoozedUntilOn: "2026-10-20" }] };
    view.rerender(<InvestmentReviewPanel />);
    expect(low).toHaveValue(2.5); expect(screen.getByRole("combobox", { name: "Review window" })).toHaveValue("7");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(state.save.mutate).toHaveBeenCalledWith({ expectedRevision: 2, policy: { ...fixture().policy, windowDays: 7, lowAnnualizedPercent: "2.5" } });
  });
  it("distinguishes loading, failed reads, cached stale reads and write failures", async () => {
    state.query.data = undefined; state.query.isPending = true;
    const view = mount(); expect(screen.getByRole("status")).toBeVisible();
    state.query.isPending = false; state.query.isError = true; view.rerender(<InvestmentReviewPanel />);
    expect(screen.getByRole("alert")).toHaveTextContent("Unable to load");
    state.query.data = fixture(); view.rerender(<InvestmentReviewPanel />);
    expect(screen.getByRole("alert")).toHaveTextContent("last successfully read");
    expect(screen.getByRole("button", { name: /Reviewed/ })).toBeDisabled();
    await controls(); expect(screen.getByRole("checkbox", { name: /Enable review reminders/ })).toBeDisabled();
    state.query.isError = false; state.save.isError = state.acknowledge.isError = true; view.rerender(<InvestmentReviewPanel />);
    expect(screen.getAllByRole("alert")).toHaveLength(2);
  });
  it("distinguishes no eligible holdings, no pending monitored reviews, insufficient data and master pause", async () => {
    state.query.data!.items = []; const view = mount(); expect(screen.getByText(i18n.t("investmentReview.empty"))).toBeVisible();
    expect(screen.getByText("Review preferences")).toBeVisible();
    state.query.data = fixture(); state.query.data.items![0].status = "healthy"; state.query.data.items![0].episodeId = "";
    view.rerender(<InvestmentReviewPanel />); expect(screen.getByText(i18n.t("investmentReview.noPending"))).toBeVisible();
    state.query.data.items = [{ ...fixture().items![0], status: "unavailable", reason: "baseline_too_old", episodeId: "", annualizedPercent: null, periodPercent: null }];
    view.rerender(<InvestmentReviewPanel />); expect(screen.getByText(i18n.t("investmentReview.insufficient"))).toBeVisible();
    await allInvestments(); expect(screen.getByText(/coverage is insufficient/)).toBeVisible();
    expect(screen.queryByText(/Estimated annualized return:/)).not.toBeInTheDocument();
    state.query.data.items![0].status = "stale"; state.query.data.items![0].annualizedPercent = "0.25";
    view.rerender(<InvestmentReviewPanel />); expect(screen.getByText(i18n.t("investmentReview.insufficient"))).toBeVisible();
    expect(screen.getByText(i18n.t("investmentReview.status.stale"))).toBeVisible();
    state.query.data.policy.enabled = false; view.rerender(<InvestmentReviewPanel />);
    expect(screen.getByRole("status")).toHaveTextContent(i18n.t("investmentReview.disabled"));
    expect(screen.getByText(i18n.t("investmentReview.status.paused"))).toBeVisible();
    expect(screen.queryByRole("button", { name: /Reviewed/ })).not.toBeInTheDocument();
  });
  it.each(["en", "zh-CN", "zh-TW"])("keeps the zero-pending summary entry and routes to Upcoming actions in %s", async language => {
    await i18n.changeLanguage(language); state.query.data!.items![0].acknowledged = true;
    const view = mount(<InvestmentReviewSummary />);
    expect(screen.getByText(i18n.t("investmentReview.summary", { count: 0 }))).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: i18n.t("investmentReview.view") }));
    expect(open).toHaveBeenCalledWith({ page: "upcoming-actions" });
    state.query.data!.items![0].monitored = false; view.rerender(<InvestmentReviewSummary />);
    expect(screen.getByText(i18n.t("investmentReview.setup"))).toBeVisible();
  });
  it.each(["en", "zh-CN", "zh-TW"])("keeps localized snooze/resume/off controls and original handling priority in %s", async language => {
    await i18n.changeLanguage(language); const view = mount(); const user = userEvent.setup();
    const summary = screen.getByText(i18n.t("investmentReview.controls", { name: "Synthetic fund" }));
    expect(summary.closest("details")).not.toHaveAttribute("open");
    summary.focus(); await user.click(summary); expect(summary).toHaveFocus();
    for (const days of [7, 30]) {
      await user.click(screen.getByRole("button", { name: i18n.t("investmentReview.snooze", { days, name: "Synthetic fund" }) }));
      expect(state.snooze.mutate).toHaveBeenLastCalledWith({ instrumentId: "synthetic-id", days, expectedRevision: 1 });
    }
    await allInvestments();
    state.query.data!.policy.enabled = false;
    state.query.data!.items![0] = { ...fixture().items![0], snoozed: true, snoozedUntilOn: "2026-10-20", episodeId: "" };
    view.rerender(<InvestmentReviewPanel />);
    expect(screen.getByText(i18n.t("investmentReview.status.snoozed"))).toBeVisible();
    expect(screen.queryByText(i18n.t("investmentReview.status.paused"))).not.toBeInTheDocument();
    expect(screen.getByText(i18n.t("investmentReview.status.low_yield"))).toBeVisible();
    expect(screen.getByText(i18n.t("investmentReview.resumeDate", { date: "2026-10-20", timezone: "Asia/Shanghai" }))).toBeVisible();
    await controls();
    const resume = screen.getByRole("button", { name: i18n.t("investmentReview.resumeNow", { name: "Synthetic fund" }) });
    resume.focus(); await user.keyboard("{Enter}");
    expect(state.snooze.mutate).toHaveBeenLastCalledWith({ instrumentId: "synthetic-id", days: 0, expectedRevision: 1 });
    await user.click(screen.getByRole("button", { name: i18n.t("investmentReview.ignoreIndefinitely", { name: "Synthetic fund" }) }));
    expect(state.enroll.mutate).toHaveBeenCalledWith({ instrumentId: "synthetic-id", enabled: false, expectedRevision: 1 });
    state.query.data!.items![0].monitored = false; view.rerender(<InvestmentReviewPanel />);
    expect(screen.getByText(i18n.t("investmentReview.status.not_enabled"))).toBeVisible();
  });
  it.each(["stale", "fetching", "snooze", "enroll", "acknowledge"])("guards all writes during %s, preserving NAV inspection", async guard => {
    state.query.data!.items![0].snoozed = true; state.query.data!.items![0].episodeId = "";
    if (guard === "stale") state.query.isError = true;
    else if (guard === "fetching") state.query.isFetching = true;
    else state[guard as "snooze" | "enroll" | "acknowledge"].isPending = true;
    mount(); await allInvestments(); await controls(); await userEvent.click(screen.getByText("Review preferences"));
    for (const name of [/Ignore for 7/, /Ignore for 30/, /Resume now/, /Disable reminders/, /^Save$/]) expect(screen.getByRole("button", { name })).toBeDisabled();
    expect(screen.getByRole("checkbox", { name: /Enable review reminders/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: /View NAV/ })).toBeEnabled();
    expect(state.snooze.mutate).not.toHaveBeenCalled(); expect(state.enroll.mutate).not.toHaveBeenCalled();
  });
});


describe("compact homepage investment review", () => {
  it.each(["en", "zh-CN", "zh-TW"])("distinguishes opt-in from monitoring without pending alerts in %s", async language => {
    await i18n.changeLanguage(language);
    state.query.data!.items![0].episodeId = "";
    state.query.data!.items![0].status = "healthy";
    state.query.data!.items![0].monitored = false;
    const view = mount(<InvestmentReviewSummary compact />);
    expect(screen.getByText(i18n.t("homepage.monitoringOff"))).toBeVisible();
    expect(screen.queryByText(i18n.t("investmentReview.summary", { count: 0 }))).not.toBeInTheDocument();
    state.query.data!.items![0].monitored = true;
    view.rerender(<InvestmentReviewSummary compact />);
    expect(screen.getByText(i18n.t("homepage.monitoringNoAlerts"))).toBeVisible();
    state.query.data!.items![0].status = "stale";
    view.rerender(<InvestmentReviewSummary compact />);
    expect(screen.getByText(i18n.t("homepage.monitoringInsufficient"))).toBeVisible();
    state.query.data!.policy.enabled = false;
    view.rerender(<InvestmentReviewSummary compact />);
    expect(screen.getByText(i18n.t("investmentReview.disabled"))).toBeVisible();
    const action = screen.getByRole("button", { name: i18n.t("investmentReview.view") });
    action.focus(); await userEvent.keyboard("{Enter}");
    expect(open).toHaveBeenCalledWith({ page: "upcoming-actions" });
  });
  it("shows real reminders with the original condition and source as-of zone", () => {
    state.query.data!.items!.push({ ...state.query.data!.items![0], instrumentId: "loss", displayName: "Synthetic loss", status: "loss" });
    mount(<InvestmentReviewSummary compact />);
    expect(screen.getByText("Synthetic loss")).toBeVisible();
    expect(screen.getByText(i18n.t("investmentReview.status.loss"))).toHaveClass("text-destructive");
    expect(screen.getByText(i18n.t("investmentReview.status.low_yield"))).toBeVisible();
    expect(screen.getByText(/Asia\/Shanghai/)).toBeVisible();
  });
  it("keeps loading, error, stale and retry distinct", async () => {
    state.query.data = undefined; state.query.isPending = true;
    const view = mount(<InvestmentReviewSummary compact />);
    expect(screen.getByRole("status")).toBeVisible();
    state.query.isPending = false; state.query.isError = true;
    view.rerender(<InvestmentReviewSummary compact />);
    expect(screen.getByRole("alert")).toHaveTextContent(i18n.t("investmentReview.error"));
    await userEvent.click(screen.getByRole("button", { name: i18n.t("common.retryAction") }));
    expect(state.query.refetch).toHaveBeenCalledOnce();
    state.query.data = fixture(); view.rerender(<InvestmentReviewSummary compact />);
    expect(screen.getByRole("alert")).toHaveTextContent(i18n.t("investmentReview.staleRead"));
    expect(screen.getByText("Synthetic fund")).toBeVisible();
  });
});

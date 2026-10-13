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
}));
vi.mock("@/queries/investmentReview", () => ({ useInvestmentReviews: () => state.query, useInvestmentReviewActions: () => state }));
const open = vi.fn();
function fixture(): InvestmentReviewReport {
  return {
    schemaVersion: "investment-review/1", asOf: "2026-10-13T12:00:00Z", localDate: "2026-10-13", timezone: "Asia/Shanghai", revision: 1,
    policy: { enabled: true, windowDays: 30, lowAnnualizedPercent: "1", lossPercent: "-1", sustainDays: 7, maxAgeDays: 7 },
    items: [{ instrumentId: "synthetic-id", displayName: "Synthetic fund", status: "low_yield", baselineOn: "2026-09-07", latestOn: "2026-10-09", targetOn: "2026-09-09", elapsedDays: 32, ageDays: 4, missingDays: 20, observationCount: 13, annualizedPercent: "0.25", periodPercent: "0.0219178082", episodeId: "episode-one", reason: "", acknowledged: false, monitored: true }],
  };
}
function mount(element: React.ReactNode = <InvestmentReviewPanel />) { return render(<NavigationContext.Provider value={{ open, openHealth: vi.fn() }}>{element}</NavigationContext.Provider>); }
beforeEach(async () => {
  vi.clearAllMocks(); await i18n.changeLanguage("en"); state.query.data = fixture();
  state.query.isError = state.query.isPending = state.query.isFetching = false;
  state.save.isError = state.save.isPending = state.save.isSuccess = false;
  state.acknowledge.isError = state.acknowledge.isPending = state.enroll.isError = state.enroll.isPending = false;
});
describe("investment review UI", () => {
  it.each(["en", "zh-CN", "zh-TW"])("discloses estimated return, actual coverage and foreground scope in %s", async language => {
    await i18n.changeLanguage(language); mount();
    expect(screen.getByText(i18n.t("investmentReview.estimated", { value: "0.25" }))).toBeInTheDocument();
    expect(screen.getByText(i18n.t("investmentReview.dates", { latest: "2026-10-09", baseline: "2026-09-07", window: 30, days: 32, age: 4 }))).toBeInTheDocument();
    expect(screen.getByText(i18n.t("investmentReview.observations", { count: 13, missing: 20 }))).toBeInTheDocument();
    expect(screen.getByText(i18n.t("investmentReview.foreground"))).toBeInTheDocument();
    expect(screen.getByText(i18n.t("investmentReview.priceOnly"))).toBeInTheDocument();
    expect(screen.queryByText(/investmentReview\./)).not.toBeInTheDocument();
  });
  it("allows opt-in and durable acknowledgement, with safe NAV navigation", async () => {
    mount(); const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: "Enable review reminders · Synthetic fund" }));
    expect(state.enroll.mutate).toHaveBeenCalledWith({ instrumentId: "synthetic-id", enabled: false, expectedRevision: 1 });
    await user.click(screen.getByRole("button", { name: "Acknowledge review · Synthetic fund" }));
    expect(state.acknowledge.mutate).toHaveBeenCalledWith("episode-one");
    await user.click(screen.getByRole("button", { name: "Check NAV · Synthetic fund" }));
    expect(open).toHaveBeenCalledWith({ page: "market-data", focus: { instrumentId: "synthetic-id", label: "Synthetic fund" } });
  });
  it("lets a keyboard user configure a 7-day window and explicit percent thresholds", async () => {
    mount(); const user = userEvent.setup();
    await user.tab(); expect(screen.getByRole("button", { name: "Refresh reviews" })).toHaveFocus();
    await user.tab(); expect(screen.getByText("Review preferences")).toHaveFocus();
    await user.keyboard("{Enter}");
    await user.selectOptions(screen.getByRole("combobox", { name: "Review window" }), "7");
    const low = screen.getByRole("spinbutton", { name: "Low estimated annualized return threshold (%)" });
    await user.clear(low); await user.type(low, "2.5");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(state.save.mutate).toHaveBeenCalledWith({ expectedRevision: 1, policy: { ...fixture().policy, windowDays: 7, lowAnnualizedPercent: "2.5" } });
  });
  it("distinguishes pending, error, stale-read, empty and unsafe coverage states", () => {
    state.query.data = undefined; state.query.isPending = true;
    const view = mount(); expect(screen.getByRole("status")).toBeInTheDocument();
    state.query.isPending = false; state.query.isError = true; view.rerender(<InvestmentReviewPanel />);
    expect(screen.getByRole("alert")).toHaveTextContent("Unable to load");
    state.query.data = fixture(); view.rerender(<InvestmentReviewPanel />);
    expect(screen.getByRole("alert")).toHaveTextContent("last successfully read");
    expect(screen.getByRole("button", { name: "Acknowledge review · Synthetic fund" })).toBeDisabled();
    expect(screen.getByRole("checkbox", { name: "Enable review reminders · Synthetic fund" })).toBeDisabled();
    state.query.isError = false; state.query.data.items = [{ ...fixture().items![0], status: "unavailable", reason: "baseline_too_old", episodeId: "", annualizedPercent: null, periodPercent: null }];
    view.rerender(<InvestmentReviewPanel />);
    expect(screen.getByText(/coverage is insufficient/)).toBeInTheDocument();
    expect(screen.queryByText(/Estimated annualized return:/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Acknowledge review/ })).not.toBeInTheDocument();
    state.query.data.items = []; view.rerender(<InvestmentReviewPanel />);
    expect(screen.getByText(/No active fund/)).toBeInTheDocument();
  });
  it("keeps an acknowledged episode out of the summary and does not add refresh prompts", () => {
    const view = mount(<InvestmentReviewSummary />);
    expect(screen.getByText("1 unacknowledged review reminder")).toBeInTheDocument();
    state.query.data!.items![0].acknowledged = true;
    view.rerender(<InvestmentReviewSummary />);
    expect(screen.getByText("0 unacknowledged review reminders")).toBeInTheDocument();
    view.rerender(<InvestmentReviewPanel />);
    expect(screen.getByText("This review episode is acknowledged")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Acknowledge review/ })).not.toBeInTheDocument();
  });
  it("shows unmonitored data without reminding and handles write failures", () => {
    state.query.data!.items![0].monitored = false; state.query.data!.items![0].episodeId = "";
    state.save.isError = state.acknowledge.isError = true;
    mount();
    expect(screen.getByText("Reminders not enabled for this investment")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Acknowledge review/ })).not.toBeInTheDocument();
    expect(screen.getAllByRole("alert")).toHaveLength(2);
  });
});

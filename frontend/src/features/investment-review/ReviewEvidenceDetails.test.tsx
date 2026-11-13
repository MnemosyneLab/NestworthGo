import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/i18n";
import type { ReviewAlertRule, ReviewDailyResult } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import { ReviewEvidenceDetails } from "./ReviewEvidenceDetails";

const state = vi.hoisted(() => ({ first: {} as Record<string, unknown>, older: {} as Record<string, unknown>, calls: vi.fn(), refetch: vi.fn() }));
vi.mock("@/queries/investmentReviewV2", () => ({
  useReviewDailyEvidencePage: (episode: string, cursor: string, active: boolean, publication: string) => {
    state.calls(episode, cursor, active, publication);
    return cursor ? state.older : state.first;
  },
}));

function day(on: string, truth = "true", actual = "-1.02", amount = "36353.1500") {
  return { on, truth, amountExact: amount, costExact: "36000", currency: "CNY",
    predicate: { all: [{ leaf: { metric: "amount", operator: ">", value: "100", currency: "CNY" } }, { any: [
      { leaf: { metric: "annualized", operator: "<", value: "0", lookbackDays: 7 } },
      { leaf: { metric: "annualized", operator: "<", value: "0", lookbackDays: 30 } },
    ] }] }, leaves: [
      { metric: "amount", actual: amount, currency: "CNY", truth: "true", effectiveOn: on },
      { metric: "annualized", actual, truth, baselineOn: "2026-10-29", effectiveOn: on },
      { metric: "annualized", actual: "0.80", truth: "false", baselineOn: "2026-10-06", effectiveOn: on },
    ],
  } as ReviewDailyResult;
}
function rule(latest = day("2026-11-09", "")) {
  return { episodeId: "synthetic-episode", episodeRevision: 4, latestResult: latest, triggeredOn: "2026-11-03", firstHitOn: "2026-11-02",
    triggerWindow: { windowStart: "2026-10-28", windowEnd: "2026-11-03", hitCount: 2 },
    window: { windowStart: "2026-11-03", windowEnd: "2026-11-09", hitCount: 3, falseCount: 3, unknownCount: 1 }, coverage: ["daily_close_missing"],
  } as ReviewAlertRule;
}
function mount(value = rule()) { return render(<ReviewEvidenceDetails rule={value} isActive publication="published-1" />); }
function data(items = [day("2026-11-09", ""), day("2026-11-05"), day("2026-11-04", "true", "-2.56", "36346.02")]) {
  return { items, latestKnownResult: day("2026-11-05"), reportRevision: "evidence-1", episodeId: "synthetic-episode", episodeRevision: 4, conditionVersion: 2, nextCursor: "older" };
}

beforeEach(async () => {
  vi.clearAllMocks();
  await i18n.changeLanguage("en");
  state.first = { data: data(), isPending: false, isFetching: false, isError: false, refetch: state.refetch };
  state.older = { ...state.first, data: { ...data([day("2026-11-03", "true", "-3.07")]), nextCursor: "" } };
});

describe("compact investment review evidence", () => {
  it.each(["en", "zh-CN", "zh-TW"])("separates trigger, latest unknown and most recent valid assessment in %s", async language => {
    await i18n.changeLanguage(language);
    mount();
    expect(screen.getByText(i18n.t("reviewV2.latestAssessment"))).toBeVisible();
    expect(screen.getByText(i18n.t("reviewV2.latestKnown"))).toBeVisible();
    expect(screen.getByText(i18n.t("reviewV2.unknownDoesNotRecover"))).toBeVisible();
    expect(screen.getByText(/2026-11-03.*2.*2026-10-28/)).toBeVisible();
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    expect(screen.queryByText(/reviewV2\./)).not.toBeInTheDocument();
  });

  it("opens by keyboard and replaces the selected day's financial values and grouped conditions", async () => {
    const user = userEvent.setup();
    mount();
    screen.getByText(i18n.t("reviewV2.dayHistory")).focus();
    await user.keyboard("{Enter}");
    const select = await screen.findByRole("combobox", { name: "Evaluation date" });
    await user.selectOptions(select, "2026-11-05");
    const details = screen.getByRole("region", { name: "Selected day details" });
    expect(details).toHaveTextContent("CN¥36,353.15");
    expect(details).toHaveTextContent("-1.02%");
    expect(details).toHaveTextContent("2026-10-29 → 2026-11-05");
    expect(within(details).getByText(/7 days/)).toBeVisible();
    expect(within(details).getByText(/30 days/)).toBeVisible();
    expect(within(details).getByText("Meet all")).toBeVisible();
    expect(within(details).getByText("Meet any")).toBeVisible();
    expect(within(details).getAllByTitle("36353.1500")[0]).toHaveTextContent("CN¥36,353.15");
    await user.selectOptions(select, "2026-11-04");
    expect(details).toHaveTextContent("-2.56%");
    expect(details).toHaveTextContent("CN¥36,346.02");
    expect(details).not.toHaveTextContent("-1.02%");
    expect(screen.getAllByRole("region")).toHaveLength(1);
  });

  it("uses a backend-selected valid result beyond the loaded page and never infers a missing position", async () => {
    const olderKnown = day("2026-10-01", "false", "0.80");
    delete olderKnown.amountExact; delete olderKnown.costExact;
    state.first.data = { ...data([day("2026-11-09", "")]), latestKnownResult: olderKnown };
    mount();
    expect(screen.getByText("2026-10-01")).toBeVisible();
    await userEvent.click(screen.getByText(i18n.t("reviewV2.dayHistory")));
    await userEvent.selectOptions(await screen.findByRole("combobox"), "2026-10-01");
    const details = screen.getByRole("region");
    expect(within(details).getByText("Holding amount").nextElementSibling).toHaveTextContent("Unknown");
    expect(within(details).getByText("Cost").nextElementSibling).toHaveTextContent("Unknown");
  });

  it("keeps position amounts unavailable when their historical currency is absent", async () => {
    const known = day("2026-10-01");
    delete known.currency;
    state.first.data = { ...data(), latestKnownResult: known };
    mount();
    await userEvent.click(screen.getByRole("button", { name: i18n.t("reviewV2.dayHistory") }));
    await userEvent.selectOptions(await screen.findByRole("combobox"), "2026-10-01");
    const details = screen.getByRole("region");
    expect(within(details).getByText("Holding amount").nextElementSibling).toHaveTextContent("Unknown");
    expect(within(details).getByText("Cost").nextElementSibling).toHaveTextContent("Unknown");
  });

  it("reads valid latest results lazily and binds the query cache to the publication", () => {
    mount(rule(day("2026-11-09")));
    expect(state.calls).toHaveBeenCalledWith("synthetic-episode", "", false, "4:published-1");
    expect(screen.queryByText("Most recent valid assessment")).not.toBeInTheDocument();
  });

  it("rejects stale episode context and offers retry without displaying its historical values", async () => {
    state.first.data = { ...data(), episodeRevision: 3 };
    const refreshReport = vi.fn();
    render(<ReviewEvidenceDetails rule={rule()} isActive onRefresh={refreshReport} />);
    expect(screen.queryByText("2026-11-05")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(state.refetch).toHaveBeenCalledOnce();
    expect(refreshReport).toHaveBeenCalledOnce();
  });

  it("pages in both directions and rejects a mixed publication rather than showing stale daily details", async () => {
    const view = mount();
    await userEvent.click(screen.getByText(i18n.t("reviewV2.dayHistory")));
    await userEvent.click(await screen.findByRole("button", { name: "Next" }));
    expect(screen.getByRole("combobox")).toHaveValue("2026-11-03");
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Previous" }));
    expect(screen.getByRole("combobox")).toHaveValue("2026-11-09");
    state.older.data = { ...data(), reportRevision: "evidence-2" };
    await userEvent.click(screen.getByRole("button", { name: "Next" }));
    view.rerender(<ReviewEvidenceDetails rule={rule()} isActive publication="published-1" />);
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
    expect(screen.getByRole("alert")).toBeVisible();
  });

  it("distinguishes loading, no valid results and a failed read", () => {
    state.first = { ...state.first, data: undefined, isPending: true };
    const view = mount();
    expect(screen.getByRole("status")).toBeVisible();
    state.first = { ...state.first, data: { ...data([]), latestKnownResult: undefined }, isPending: false };
    view.rerender(<ReviewEvidenceDetails rule={rule()} isActive />);
    expect(screen.getByText("No valid assessment in this alert’s history.")).toBeVisible();
    state.first = { ...state.first, isError: true };
    view.rerender(<ReviewEvidenceDetails rule={rule()} isActive />);
    expect(screen.getByRole("alert")).toBeVisible();
    expect(screen.queryByText("No valid assessment in this alert’s history.")).not.toBeInTheDocument();
  });
});

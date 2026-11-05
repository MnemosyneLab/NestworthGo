import { useState } from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { ReturnAnalysisPage } from "@/features/insights/ReturnAnalysisPage";
import { useAnalysisStore } from "@/stores/analysis";
import { AssistantPageProvider, useAssistantPage } from "./PageContext";
import { ModelConversation } from "./ModelConversation";
import type { ModelTarget } from "./modelNavigation";

const fixture = vi.hoisted(() => ({
  account: "00000000-0000-4000-8000-000000000001", other: "00000000-0000-4000-8000-000000000002",
  trend: vi.fn(), origin: vi.fn(), accounts: vi.fn(), dispatch: vi.fn(), blocked: false,
}));
const api = vi.hoisted(() => ({ Status: vi.fn(), PendingPageReads: vi.fn(), PublishPage: vi.fn(), GrantNavigation: vi.fn(), ConsumeNavigation: vi.fn(), List: vi.fn(), Inspect: vi.fn(), Start: vi.fn(), Cancel: vi.fn(), Create: vi.fn(), Delete: vi.fn(), Messages: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service", () => api);
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analysis", () => ({ Service: {
  ReturnTrend: (...args: unknown[]) => fixture.trend(...args), ReturnCalendar: vi.fn().mockResolvedValue({available: false}),
  Contribution: vi.fn().mockResolvedValue({available: false}), ContributionItem: vi.fn(), ReturnDay: vi.fn(),
} }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({ Service: { ListAccounts: () => fixture.accounts() } }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({ Service: { HistoryOrigin: () => fixture.origin() } }));
vi.mock("@/queries/catalog", () => ({ useCatalog: () => ({data: {currencies: ["USD", "EUR"], instrumentTypes: ["stock", "etf"]}, isLoading: false, isError: false}) }));
vi.mock("@/queries/investments", () => ({ useInstruments: () => ({data: []}) }));
vi.mock("@/queries/directory", () => ({ useMembers: () => ({data: []}) }));
vi.mock("@/queries/household", () => ({ useBootstrap: () => ({data: {household: {baseCurrency: "USD"}}}) }));
vi.mock("@/queries/settings", () => ({ useSettings: () => ({data: {dateFormat: "year-first", timezone: "UTC"}}) }));

const target = (accountId = fixture.account): ModelTarget => ({kind: "return-trend", accountId,
  trend: {from: "2026-02-01", to: "2026-02-28", valuation: "base", includeCash: false, display: "linked_rate", currency: "USD", assetClass: "stock"}});
const result = (display = "linked_rate") => ({available: true, display, points: [], sources: [], amount: {amount: "12.34", currency: "USD"}, rate: "0.01", ratedDays: 28, totalDays: 28, status: "ok", valuationForced: null});

function Command({request}: {request: ModelTarget}) {
  const page = useAssistantPage()!;
  const [outcome, setOutcome] = useState("idle");
  return <><button onClick={() => {setOutcome("waiting"); void page.navigateModel(page.modelStamp, async () => request).then(setOutcome);}}>Apply synthetic action</button><output data-testid="outcome">{outcome}</output></>;
}
function Harness({request = target(), chat = false, retained = false}: {request?: ModelTarget; chat?: boolean; retained?: boolean}) {
  const [view, setView] = useState("accounts:");
  return <AssistantPageProvider householdId="synthetic" incarnation="synthetic" surface="desktop" viewKey={view} blocked={() => fixture.blocked}
    onNavigate={next => {
      fixture.dispatch(next);
      if ("mobileAccountId" in next || next.page !== "return-analysis") return false;
      useAnalysisStore.getState().replaceFilters(next.analysis ?? {});
      useAnalysisStore.getState().setReturnView({tab: next.tab, cursor: next.cursor});
      setView("return-analysis:"); return true;
    }}>
    <button onClick={() => setView("accounts:")}>Leave page</button>
    {(retained || view === "return-analysis:") && <div hidden={view !== "return-analysis:"}><ReturnAnalysisPage /></div>}
    {chat ? <ModelConversation /> : <Command request={request} />}
  </AssistantPageProvider>;
}
function mount(request = target(), chat = false, retained = false) {
  return render(<QueryClientProvider client={createTestQueryClient()}><Harness request={request} chat={chat} retained={retained} /></QueryClientProvider>);
}
beforeEach(() => {
  vi.useFakeTimers({toFake: ["Date"]}); vi.setSystemTime(new Date("2026-11-05T12:00:00Z"));
  vi.clearAllMocks(); api.Status.mockResolvedValue({available: false}); fixture.blocked = false; useAnalysisStore.getState().reset();
  fixture.origin.mockResolvedValue({timezone: "UTC", startedAt: "2026-01-01T00:00:00Z"});
  fixture.accounts.mockResolvedValue([{account: {id: fixture.account, name: "Synthetic brokerage"}}, {account: {id: fixture.other, name: "Synthetic brokerage"}}]);
  fixture.trend.mockImplementation((_request, display) => Promise.resolve(result(display)));
  useAnalysisStore.getState().setFilters({scope: "account", scopeId: fixture.other, moreFilters: {memberId: "stale-member", instrumentId: "stale-instrument"}});
});
afterEach(() => vi.useRealTimers());

it.each(["account", "household"])("verifies actual %s query and real controls after replacing stale filters", async scope => {
  mount(target(scope === "account" ? fixture.account : ""));
  await userEvent.click(screen.getByText("Apply synthetic action"));
  await waitFor(() => expect(screen.getByTestId("outcome")).toHaveTextContent("opened"));
  expect(fixture.trend).toHaveBeenLastCalledWith({scopeKind: scope, ...(scope === "account" ? {scopeId: fixture.account} : {}), from: "2026-02-01", to: "2026-02-28", valuation: "base", basis: "investment", includeCash: false, currency: "USD", assetClass: "stock"}, "linked_rate");
  expect(document.getElementById("analysis-scope")).toHaveValue(scope === "account" ? "account" : "portfolio");
  if (scope === "account") expect(document.getElementById("analysis-scope-id")).toHaveValue(fixture.account);
  expect(document.getElementById("return-trend-display")).toHaveValue("linked_rate");
  expect(document.getElementById("analysis-from")).toHaveTextContent("2026-02-01");
  expect(document.getElementById("analysis-to")).toHaveTextContent("2026-02-28");
  expect(useAnalysisStore.getState().moreFilters).toEqual({currency: "USD", assetClass: "stock"});
  expect(fixture.dispatch).toHaveBeenCalledTimes(1);
});

it("waits for the committed page and actual query completion instead of accepting the store update", async () => {
  let finish!: (value: ReturnType<typeof result>) => void;
  fixture.trend.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
  mount(); await userEvent.click(screen.getByText("Apply synthetic action"));
  await waitFor(() => expect(fixture.trend).toHaveBeenCalled());
  expect(screen.getByTestId("outcome")).toHaveTextContent("waiting");
  await act(async () => finish(result()));
  await waitFor(() => expect(screen.getByTestId("outcome")).toHaveTextContent("opened"));
});

it.each(["failed", "unavailable", "forced-valuation", "wrong-display", "clamped", "missing-account"])("does not claim success for %s", async failure => {
  if (failure === "failed") fixture.trend.mockRejectedValue(new Error("synthetic query failure"));
  if (failure === "unavailable") fixture.trend.mockResolvedValue({...result(), available: false});
  if (failure === "forced-valuation") fixture.trend.mockResolvedValue({...result(), valuationForced: "base"});
  if (failure === "wrong-display") fixture.trend.mockResolvedValue(result("cumulative_amount"));
  if (failure === "clamped") fixture.origin.mockResolvedValue({timezone: "UTC", startedAt: "2026-02-10T00:00:00Z"});
  if (failure === "missing-account") fixture.accounts.mockResolvedValue([]);
  mount(); await userEvent.click(screen.getByText("Apply synthetic action"));
  await waitFor(() => expect(screen.getByTestId("outcome")).toHaveTextContent("unverified"), {timeout: 2500});
});

it.each(["scope", "leave", "review"])("rejects a late query after concurrent %s change", async change => {
  let finish!: (value: ReturnType<typeof result>) => void;
  fixture.trend.mockImplementation(() => new Promise(resolve => {finish = resolve;}));
  mount(); await userEvent.click(screen.getByText("Apply synthetic action"));
  await waitFor(() => expect(fixture.trend).toHaveBeenCalled());
  if (change === "leave") await userEvent.click(screen.getByText("Leave page"));
  if (change === "scope") await userEvent.selectOptions(document.getElementById("analysis-scope-id")!, fixture.other);
  if (change === "review") fixture.blocked = true;
  await act(async () => finish(result()));
  await waitFor(() => expect(screen.getByTestId("outcome")).toHaveTextContent("unverified"));
});

it("completes the authorized chat flow only after the real trend page acknowledges the request", async () => {
  api.Status.mockResolvedValue({epoch: "host", revision: 1, available: true, config: {enabled: true, model: "mock"}});
  api.PendingPageReads.mockResolvedValue([]); api.PublishPage.mockResolvedValue({id: "lease"}); api.GrantNavigation.mockResolvedValue(undefined);
  const prior = {id: "prior", state: "succeeded", text: "Synthetic prior answer", prompt: "Earlier", model: "mock", created: "2026-11-05T00:00:00Z"};
  api.List.mockResolvedValue([{id: "conversation", runCount: 1}]); api.Inspect.mockResolvedValue({id: "conversation", runs: [prior], runCount: 1, offset: 0});
  api.ConsumeNavigation.mockResolvedValue(target());
  api.Start.mockImplementation(async q => {
    api.Inspect.mockResolvedValue({id: "conversation", runCount: 2, offset: 1, runs: [{...prior, id: "run", requestId: q.requestId, text: "Trend requested"}], navigation: {id: "intent", leaseId: q.pageLeaseId, requestId: q.requestId}});
    return {state: "accepted"};
  });
  let finish!: (value: ReturnType<typeof result>) => void;
  fixture.trend.mockImplementation(() => new Promise(resolve => {finish = resolve;}));
  mount(target(), true); const user = userEvent.setup(); await screen.findByText("Synthetic prior answer");
  await user.type(screen.getByLabelText("Message"), "Open synthetic brokerage trend February 1–28, 2026, base USD, stocks, exclude cash, linked rate");
  await user.click(screen.getByText("Page navigation"));
  await user.click(screen.getByRole("button", {name: "Send and allow one page change"}));
  await waitFor(() => expect(fixture.trend).toHaveBeenCalled());
  expect(screen.queryByText("The requested page was opened.")).not.toBeInTheDocument();
  await act(async () => finish(result()));
  await screen.findByText("The requested page was opened.");
  expect(api.GrantNavigation).toHaveBeenCalledTimes(1); expect(api.ConsumeNavigation).toHaveBeenCalledTimes(1);
});


it("reconfigures a retained real trend page with account, dates and display in one navigation", async () => {
  useAnalysisStore.getState().setReturnView({tab: "trend"});
  useAnalysisStore.getState().setFilters({from: "2026-03-01", to: "2026-03-15", trendDisplay: "cumulative_amount"});
  mount(target(), false, true);
  await waitFor(() => expect(fixture.trend).toHaveBeenCalled());
  await userEvent.click(screen.getByText("Apply synthetic action"));
  await waitFor(() => expect(screen.getByTestId("outcome")).toHaveTextContent("opened"));
  expect(document.getElementById("analysis-scope-id")).toHaveValue(fixture.account);
  expect(document.getElementById("analysis-from")).toHaveTextContent("2026-02-01");
  expect(document.getElementById("return-trend-display")).toHaveValue("linked_rate");
});

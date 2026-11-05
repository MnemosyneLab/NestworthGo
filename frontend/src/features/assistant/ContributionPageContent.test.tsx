import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import i18n from "@/i18n";
import { createTestQueryClient } from "@/test/queryClient";
import { ContributionTab } from "@/features/insights/ContributionTab";
import { useAnalysisStore } from "@/stores/analysis";
import { AssistantPageProvider } from "./PageContext";
import { ModelConversation } from "./ModelConversation";
import { capturePageContent } from "./pageContent";

const fixture = vi.hoisted(() => ({ fetching: false, data: {
  available: true, status: "ok", rows: [{key: "synthetic", label: "Synthetic row", amount: {amount: "12.34", currency: "USD"}, status: "ok", ratedDays: 1, totalDays: 1}],
  summary: {total: {amount: "12.34", currency: "USD"}, positive: {amount: "12.34", currency: "USD"}, negative: {amount: "0", currency: "USD"}},
} }));
const api = vi.hoisted(() => ({Status: vi.fn(), PendingPageReads: vi.fn(), DeliverPageContent: vi.fn(), PublishPage: vi.fn(), GrantNavigation: vi.fn(), ConsumeNavigation: vi.fn(), List: vi.fn(), Inspect: vi.fn(), Messages: vi.fn(), Start: vi.fn(), Cancel: vi.fn(), Create: vi.fn(), Delete: vi.fn()}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service", () => api);
vi.mock("@/queries/returnAnalysis", () => ({useContribution: () => ({data: fixture.data, isLoading: false, isError: false, isFetching: fixture.fetching}), useContributionItem: () => ({})}));
vi.mock("@/queries/accounts", () => ({useAccounts: () => ({data: []})}));
vi.mock("@/queries/investments", () => ({useInstruments: () => ({data: []})}));
vi.mock("@/features/insights/analysisProjectionContext", () => ({useAnalysisProjectionContext: () => ({request: {}, enabled: true, origin: {data: {timezone: "UTC"}}, scopeReady: true, rangeAvailable: true})}));
function RealContribution() {
  const session = useAnalysisStore();
  return <main data-assistant-page="return-analysis"><section data-assistant-module="contribution"><ContributionTab session={session} /></section></main>;
}
beforeEach(() => {
  useAnalysisStore.getState().reset(); fixture.fetching = false; vi.clearAllMocks();
  api.Status.mockResolvedValue({epoch: "host", revision: 1, available: true, config: {enabled: true, model: "mock"}});
  api.PendingPageReads.mockResolvedValue([]); api.PublishPage.mockResolvedValue({id: "lease"}); api.GrantNavigation.mockResolvedValue(undefined);
  api.List.mockResolvedValue([{id: "conversation", runCount: 1}]);
  api.Inspect.mockResolvedValue({id: "conversation", runs: [{id: "run", state: "succeeded", text: "Synthetic prior answer", prompt: "Earlier", created: "2026-11-05T00:00:00Z", model: "mock"}], runCount: 1, offset: 0});
});
const changes = [
  ["contribution-return-type", "realized"], ["contribution-group-by", "account"],
  ["contribution-sort", "amount_asc"], ["contribution-limit", "20"], ["contribution-search", "Synthetic"],
] as const;
it.each(changes)("captures real %s state and invalidates its snapshot without changing backend results", async (id, next) => {
  render(<QueryClientProvider client={createTestQueryClient()}><RealContribution /></QueryClientProvider>);
  const field = document.getElementById(id)!;
  const before = capturePageContent("return-analysis:");
  const beforeNode = before.nodes.find(n => n.id === `contribution/${id}`);
  expect(beforeNode).toBeDefined(); expect(beforeNode?.state).not.toContain("redacted");
  const summary = screen.getByLabelText(i18n.t("insights.contributionSummary")).textContent;
  expect(summary).toContain("12.34");
  fireEvent.change(field, {target: {value: next}});
  const after = capturePageContent("return-analysis:");
  const afterNode = after.nodes.find(n => n.id === `contribution/${id}`);
  const expected = field instanceof HTMLSelectElement ? field.selectedOptions[0].label : next;
  expect(afterNode?.value).toBe(expected); expect(afterNode?.value).not.toBe(beforeNode?.value);
  expect(after.id).not.toBe(before.id); // Existing backend rejects old-ID continuations.
  expect(screen.getByLabelText(i18n.t("insights.contributionSummary")).textContent).toBe(summary);
  expect(screen.getAllByText("Synthetic row").length).toBeGreaterThan(0);
});

it.each([...changes, ["contribution-loading", "true"]] as const)("blocks a granted navigation if real %s changes while consuming its intent", async (id, next) => {
  let resolve!: (value: {kind: string; accountId: string}) => void;
  api.ConsumeNavigation.mockImplementation(() => new Promise(done => {resolve = done;}));
  api.Start.mockImplementation(async q => {
    api.Inspect.mockResolvedValue({id: "conversation", runCount: 2, offset: 1, runs: [{id: "run", requestId: q.requestId, state: "succeeded", text: "Requested", prompt: q.prompt, model: "mock", created: "2026-11-05T00:00:00Z"}], navigation: {id: "intent", leaseId: q.pageLeaseId, requestId: q.requestId}});
    return {state: "accepted"};
  });
  const navigate = vi.fn();
  render(<QueryClientProvider client={createTestQueryClient()}><AssistantPageProvider householdId="synthetic" incarnation="synthetic" surface="desktop" viewKey="return-analysis:" blocked={() => false} onNavigate={navigate}><RealContribution /><ModelConversation /></AssistantPageProvider></QueryClientProvider>);
  const user = userEvent.setup(); await screen.findByText("Synthetic prior answer");
  await user.type(screen.getByLabelText("Message"), "Open overview");
  await user.click(screen.getByText("Page navigation"));
  await user.click(screen.getByRole("button", {name: "Send and allow one page change"}));
  await waitFor(() => expect(api.ConsumeNavigation).toHaveBeenCalledTimes(1));
  if (id === "contribution-loading") {
    act(() => { fixture.fetching = true; useAnalysisStore.getState().setReturnView({cursor: "2026-10"}); });
  } else fireEvent.change(document.getElementById(id)!, {target: {value: next}});
  await act(async () => resolve({kind: "overview", accountId: ""}));
  await screen.findByText(/No page change was applied/); expect(navigate).not.toHaveBeenCalled();
});


it("captures the real roleless contribution container's busy transitions with unchanged rows and controls", () => {
 const client=createTestQueryClient();
 const element=()=> <QueryClientProvider client={client}><RealContribution /></QueryClientProvider>;
 const view=render(element());const root=document.querySelector("main");
 const before=capturePageContent("return-analysis:");
 const busyNode=before.nodes.find(n=>n.role==="group" && n.state.includes("busy=false"));
 expect(busyNode).toBeDefined();
 const summary=screen.getByLabelText(i18n.t("insights.contributionSummary")).textContent;
 fixture.fetching=true;view.rerender(element());
 const during=capturePageContent("return-analysis:");
 expect(document.querySelector("main")).toBe(root);
 expect(during.nodes.find(n=>n.id===busyNode!.id)?.state).toContain("busy=true");
 expect(during.id).not.toBe(before.id);
 expect(screen.getByLabelText(i18n.t("insights.contributionSummary")).textContent).toBe(summary);
 fixture.fetching=false;view.rerender(element());
 const after=capturePageContent("return-analysis:");
 expect(after.nodes.find(n=>n.id===busyNode!.id)?.state).toContain("busy=false");
 expect(after.id).not.toBe(during.id);
});

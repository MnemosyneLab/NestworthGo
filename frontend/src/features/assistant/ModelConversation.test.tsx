import { useLayoutEffect, useState } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider, onlineManager } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { AssistantPageProvider, useAssistantPage, usePublishAssistantPage } from "./PageContext";

import { ModelConversation } from "./ModelConversation";
import i18n from "@/i18n";
const api = vi.hoisted(() => ({
  Reviews: vi.fn(),
  SetWritePermission: vi.fn(),
  SetFullAccess: vi.fn(),
  ReviewAction: vi.fn(),
  PendingPageReads: vi.fn(),
  DeliverPageContent: vi.fn(),
  PublishPage: vi.fn(),
  GrantNavigation: vi.fn(),
  ConsumeNavigation: vi.fn(),
  Status: vi.fn(),
  List: vi.fn(),
  Inspect: vi.fn(),
  Messages: vi.fn(),
  Start: vi.fn(),
  Cancel: vi.fn(),
  Create: vi.fn(),
  Delete: vi.fn(),
  Rename: vi.fn(),
}));
vi.mock(
  "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service",
  () => api,
);
const run = {
  id: "run",
  requestId: "saved-request",
  prompt: "Synthetic saved question",
  text: "Synthetic prior answer",
  evidence: [
    {
      source: "get_account_snapshot",
      capturedAt: "2026-11-04T10:00:00Z",
      hasMore: true,
      missing: "reported",
    },
  ],
  state: "succeeded",
  model: "mock",
  created: "2026-11-04T10:00:00Z",
};
beforeEach(() => {
  vi.clearAllMocks();
  api.Reviews.mockResolvedValue([]);
  api.PendingPageReads.mockResolvedValue([]);
  api.DeliverPageContent.mockResolvedValue(undefined);
  api.PublishPage.mockResolvedValue({ id: "host-page-lease" });
  api.GrantNavigation.mockResolvedValue(undefined);
  api.ConsumeNavigation.mockResolvedValue({ kind: "overview", accountId: "" });
  api.Status.mockResolvedValue({
    epoch: "host",
    revision: 3,
    available: true,
    credentials: "present",
    config: { provider: "openai-responses", model: "mock", enabled: true },
  });
  api.List.mockResolvedValue([{ id: "conversation", runCount: 1 }]);
  api.Inspect.mockResolvedValue({
    id: "conversation",
    runs: [run],
    runCount: 1,
    offset: 0,
  });
  api.Messages.mockResolvedValue({
    id: "conversation",
    runs: [run],
    runCount: 2,
    offset: 0,
  });
  api.Start.mockResolvedValue({ ...run, state: "accepted" });
  api.Cancel.mockResolvedValue(undefined);
  api.Create.mockResolvedValue({ id: "new-conversation" });
  api.Delete.mockResolvedValue(undefined);
  api.Rename.mockResolvedValue(undefined);
});
function mount(viewKey = "overview:", client = createTestQueryClient()) {
  return render(
    <QueryClientProvider client={client}>
      <AssistantPageProvider
        householdId="synthetic"
        incarnation="synthetic"
        surface="desktop"
        viewKey={viewKey}
        blocked={() => false}
        onNavigate={vi.fn()}
      >
        <ModelConversation />
      </AssistantPageProvider>
    </QueryClientProvider>,
  );
}
it("restores history without replay and sends only the question and selector", async () => {
  const user = userEvent.setup();
  mount();
  await screen.findByText("Synthetic prior answer");
  expect(api.Start).not.toHaveBeenCalled();
  await user.click(screen.getByText(/Data reads · 1/));
  expect(screen.getByText(/Account snapshot/)).toBeVisible();
  expect(screen.getByText(/This response advertised more pages/)).toBeVisible();
  expect(screen.getByText(/Read started/)).toBeVisible();
  expect(screen.queryByText(/Read completed/)).not.toBeInTheDocument();
  await user.type(screen.getByLabelText("Message"), "Read my current page");
  await user.click(screen.getByRole("button", { name: "Send to model" }));
  await waitFor(() => expect(api.Start).toHaveBeenCalledTimes(1));
  expect(api.Start.mock.calls[0][0]).toMatchObject({
    epoch: "host",
    revision: 3,
    conversationId: "conversation",
    prompt: "Read my current page",
    page: { kind: "overview", accountId: "" },
  });
  expect(api.GrantNavigation).not.toHaveBeenCalled();
  expect(api.ConsumeNavigation).not.toHaveBeenCalled();
  expect(Object.keys(api.Start.mock.calls[0][0]).sort()).toEqual([
    "conversationId",
    "epoch",
    "page",
    "pageLeaseId",
    "prompt",
    "requestId",
    "revision",
  ]);
  expect(screen.getByLabelText("Message")).toHaveValue("");
});
it("shows active feedback, blocks duplicate sends and stops the original run", async () => {
  api.Inspect.mockResolvedValue({
    id: "conversation",
    runs: [{ ...run, state: "running", text: "" }],
    runCount: 1,
    offset: 0,
  });
  const user = userEvent.setup();
  mount();
  await screen.findByText(/Running · reading related data/);
  await user.type(screen.getByLabelText("Message"), "Second message");
  expect(screen.getByRole("button", { name: "Send to model" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "Stop model run" }));
  await waitFor(() =>
    expect(api.Cancel).toHaveBeenCalledWith("host", "conversation", "run"),
  );
  expect(api.Start).not.toHaveBeenCalled();
});
it("does not retry uncertain Start and exposes only a fixed error", async () => {
  api.Start.mockRejectedValue(new Error("synthetic-secret-provider-error"));
  const user = userEvent.setup();
  mount("settings:");
  await screen.findByText("Synthetic prior answer");
  await user.type(screen.getByLabelText("Message"), "question");
  await user.click(screen.getByRole("button", { name: "Send to model" }));
  await screen.findByText(/Connection or request failed/);
  expect(screen.queryByText(/synthetic-secret/)).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Try again" }));
  expect(api.Start).toHaveBeenCalledTimes(1);
  expect(api.Start.mock.calls[0][0].page).toEqual({ kind: "", accountId: "" });
});
it("keeps an off runtime closed and prevents byte-overflow submission", async () => {
  const view = mount();
  await screen.findByText("Synthetic prior answer");
  fireEvent.change(screen.getByLabelText("Message"), {
    target: { value: "家".repeat(5500) },
  });
  expect(screen.getByRole("button", { name: "Send to model" })).toBeDisabled();
  view.unmount();
  api.Status.mockResolvedValue({
    epoch: "host",
    revision: 3,
    available: false,
    credentials: "present",
  });
  mount();
  await screen.findByText(/AI is off or unavailable/);
  expect(screen.getByLabelText("Message")).toBeDisabled();
  expect(api.Start).not.toHaveBeenCalled();
});
it("shows consecutive messages together without turn navigation", async () => {
  api.Inspect.mockResolvedValue({ id: "conversation", runs: [{ ...run, id: "latest", prompt: "Latest question", text: "Latest answer" }], runCount: 2, offset: 1 });
  mount();
  await screen.findByText("Latest answer");
  await screen.findByText("Synthetic prior answer");
  expect(api.Messages).toHaveBeenCalledWith("host", "conversation", 0);
  const exchanges = screen.getAllByRole("article");
  expect(exchanges[0]).toHaveTextContent("Synthetic saved question");
  expect(exchanges[1]).toHaveTextContent("Latest question");
  expect(screen.queryByRole("button", { name: "Previous turn" })).not.toBeInTheDocument();
  expect(api.Start).not.toHaveBeenCalled();
});

afterEach(() => {
  vi.restoreAllMocks();
  onlineManager.setOnline(true);
});
it("loads local status/history/pages while the browser is offline", async () => {
  vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
  onlineManager.setOnline(false);
  api.Inspect.mockResolvedValue({
    id: "conversation",
    runs: [run],
    runCount: 2,
    offset: 1,
  });
  mount();
  await screen.findByText("Synthetic prior answer");
  expect(api.Status).toHaveBeenCalled();
  expect(api.List).toHaveBeenCalled();
  expect(api.Inspect).toHaveBeenCalled();
  await waitFor(() =>
    expect(api.Messages).toHaveBeenCalledWith("host", "conversation", 0),
  );
  expect(screen.getByRole("button", { name: "Send to model" })).toBeDisabled();
  expect(api.Start).not.toHaveBeenCalled();
});
it("keeps local Stop available during a browser offline hint", async () => {
  vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
  onlineManager.setOnline(false);
  api.Inspect.mockResolvedValue({
    id: "conversation",
    runs: [{ ...run, state: "running", text: "" }],
    runCount: 1,
    offset: 0,
  });
  const user = userEvent.setup();
  mount();
  const stop = await screen.findByRole("button", { name: "Stop model run" });
  expect(stop).toBeEnabled();
  await user.click(stop);
  await waitFor(() =>
    expect(api.Cancel).toHaveBeenCalledWith("host", "conversation", "run"),
  );
  expect(api.Start).not.toHaveBeenCalled();
});

it("executes one typed navigation only after the explicit per-message grant", async () => {
  const navigate = vi.fn();
  api.Start.mockImplementation(async (q) => {
    api.Inspect.mockResolvedValue({
      id: "conversation",
      runs: [{ ...run, requestId: q.requestId, state: "succeeded" }],
      runCount: 2,
      offset: 1,
      navigation: {
        id: "host-intent",
        leaseId: q.pageLeaseId,
        requestId: q.requestId,
        runId: "run",
        conversationId: "conversation",
      },
    });
    return { ...run, state: "accepted" };
  });
  const client = createTestQueryClient();
  render(
    <QueryClientProvider client={client}>
      <AssistantPageProvider
        householdId="synthetic"
        incarnation="synthetic"
        surface="desktop"
        viewKey="overview:"
        blocked={() => false}
        onNavigate={navigate}
      >
        <ModelConversation />
      </AssistantPageProvider>
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await screen.findByText("Synthetic prior answer");
  await user.type(screen.getByLabelText("Message"), "Open the account list");
  await user.click(screen.getByText("Page navigation"));
  await user.click(
    screen.getByRole("button", { name: "Send and allow one page change" }),
  );
  await waitFor(() =>
    expect(navigate).toHaveBeenCalledWith({ page: "overview" }),
  );
  expect(api.GrantNavigation).toHaveBeenCalledTimes(1);
  expect(api.ConsumeNavigation).toHaveBeenCalledTimes(1);
  expect(navigate).toHaveBeenCalledTimes(1);
  await client.invalidateQueries({ queryKey: ["aiConversation"] });
  expect(api.ConsumeNavigation).toHaveBeenCalledTimes(1);
});
it("does not navigate when the page changes during intent consumption", async () => {
  let finish!: (target: { kind: string; accountId: string }) => void;
  api.ConsumeNavigation.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  api.Start.mockImplementation(async (q) => {
    api.Inspect.mockResolvedValue({
      id: "conversation",
      runs: [{ ...run, requestId: q.requestId, state: "succeeded" }],
      runCount: 2,
      offset: 1,
      navigation: {
        id: "host-intent",
        leaseId: q.pageLeaseId,
        requestId: q.requestId,
        runId: "run",
        conversationId: "conversation",
      },
    });
    return { ...run, state: "accepted" };
  });
  const navigate = vi.fn();
  function Shell() {
    const [view, setView] = useState("overview:");
    return (
      <AssistantPageProvider
        householdId="synthetic"
        incarnation="synthetic"
        surface="desktop"
        viewKey={view}
        blocked={() => false}
        onNavigate={navigate}
      >
        <button onClick={() => setView("accounts:")}>
          Change synthetic page
        </button>
        <ModelConversation />
      </AssistantPageProvider>
    );
  }
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <Shell />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await screen.findByText("Synthetic prior answer");
  await user.type(screen.getByLabelText("Message"), "Open overview");
  await user.click(screen.getByText("Page navigation"));
  await user.click(
    screen.getByRole("button", { name: "Send and allow one page change" }),
  );
  await waitFor(() => expect(api.ConsumeNavigation).toHaveBeenCalledTimes(1));
  await user.click(
    screen.getByRole("button", { name: "Change synthetic page" }),
  );
  finish({ kind: "overview", accountId: "" });
  await screen.findByText(/No page change was applied/);
  expect(navigate).not.toHaveBeenCalled();
});

it.each([
  ["Delete conversation", "Delete"],
  ["New conversation", "Create"],
] as const)(
  "withdraws pending navigation as soon as %s starts, before IPC completes",
  async (label, operation) => {
    let consume!: (target: { kind: string; accountId: string }) => void;
    api.ConsumeNavigation.mockImplementation(
      () =>
        new Promise((resolve) => {
          consume = resolve;
        }),
    );
    let completeAction!: () => void;
    api[operation].mockImplementation(
      () =>
        new Promise((resolve) => {
          completeAction = () => resolve({ id: "new-conversation" });
        }),
    );
    api.Start.mockImplementation(async (q) => {
      api.Inspect.mockResolvedValue({
        id: "conversation",
        runs: [{ ...run, requestId: q.requestId, state: "succeeded" }],
        runCount: 2,
        offset: 1,
        navigation: {
          id: "host-intent",
          leaseId: q.pageLeaseId,
          requestId: q.requestId,
          runId: "run",
          conversationId: "conversation",
        },
      });
      return { ...run, state: "accepted" };
    });
    const navigate = vi.fn();
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <AssistantPageProvider
          householdId="synthetic"
          incarnation="synthetic"
          surface="desktop"
          viewKey="overview:"
          blocked={() => false}
          onNavigate={navigate}
        >
          <ModelConversation />
        </AssistantPageProvider>
      </QueryClientProvider>,
    );
    const user = userEvent.setup();
    await screen.findByText("Synthetic prior answer");
    await user.type(screen.getByLabelText("Message"), "Open overview");
    await user.click(screen.getByText("Page navigation"));
    await user.click(
      screen.getByRole("button", { name: "Send and allow one page change" }),
    );
    await waitFor(() => expect(api.ConsumeNavigation).toHaveBeenCalledTimes(1));
    const action = screen.getByRole("button", { name: label });
    await waitFor(() => expect(action).toBeEnabled());
    await user.click(action);
    await waitFor(() => expect(api[operation]).toHaveBeenCalledTimes(1));
    // The host already approved this intent, but its IPC response arrives while
    // the conversation mutation is still pending. It must not dispatch.
    await act(async () => consume({ kind: "overview", accountId: "" }));
    expect(navigate).not.toHaveBeenCalled();
    await screen.findByText(/No page change was applied/);
    await act(async () => completeAction());
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "New conversation" }),
      ).toBeEnabled(),
    );
    expect(navigate).not.toHaveBeenCalled();
  },
);

it.each([
  ["provider_authentication", "Check the saved API key and model access"],
  ["provider_rate_limited", "Check usage and billing"],
  ["provider_request_rejected", "Check the model name and access"],
  ["provider_unavailable", "provider is temporarily unavailable"],
  ["provider_connection", "Check your network or proxy"],
  ["provider_response_rejected", "response was incomplete or unsupported"],
  ["model_history_rejected", "Start a new conversation"],
  ["tool_result_rejected", "Try a narrower financial question"],
  ["assistant_budget_exceeded", "run reached a resource limit"],
  ["assistant_budget_exceeded.model_rounds", "model round limit"],
  ["assistant_budget_exceeded.tool_calls", "tool call limit"],
  ["assistant_budget_exceeded.history_bytes", "context limit"],
  ["assistant_budget_exceeded.history_messages", "context limit"],
  ["assistant_budget_exceeded.prompt_bytes", "input size limit"],
  ["assistant_budget_exceeded.text_bytes", "display size limit"],
  ["assistant_budget_exceeded.provider_payload", "payload limit"],
  ["assistant_budget_exceeded.tool_result_bytes", "result size limit"],
  ["assistant_budget_exceeded.read_evidence", "read evidence limit"],
  ["private-canary-unknown-code", "No new model history was saved"],
])("shows safe actionable diagnostic for %s", async (code, message) => {
  api.Inspect.mockResolvedValue({
    id: "conversation",
    runs: [{ ...run, state: "failed", text: "", evidence: [], code }],
    runCount: 1,
    offset: 0,
  });
  mount();
  expect(await screen.findByRole("alert")).toHaveTextContent(message);
  expect(screen.queryByText(/private-canary/)).not.toBeInTheDocument();
  expect(api.Start).not.toHaveBeenCalled();
});

it.each([
  ["provider_response_rejected.stream_function_status", true],
  ["provider_response_rejected.response_item", true],
  ["provider_response_rejected.sdk_conversion", true],
  ["provider_response_rejected.stream_part", true],
  ["provider_response_rejected.private-canary", false],
  ["provider_response_rejected.stream_part private-canary", false],
  ["assistant_budget_exceeded.model_rounds", true],
  ["assistant_budget_exceeded.tool_calls", true],
  ["assistant_budget_exceeded.private-canary", false],
  ["assistant_budget_exceeded.model_rounds private-canary", false],
])("only renders fixed diagnostic rules: %s", async (code, recognized) => {
  api.Inspect.mockResolvedValue({
    id: "conversation",
    runs: [{ ...run, state: "failed", text: "", evidence: [], code }],
    runCount: 1,
    offset: 0,
  });
  mount();
  const alert = await screen.findByRole("alert");
  if (recognized) {
    expect(alert).toHaveTextContent(`Diagnostic: ${code}`);
    expect(alert).toHaveTextContent("Include this code and the app version");
  } else {
    expect(alert).not.toHaveTextContent("Diagnostic:");
  }
  expect(alert).not.toHaveTextContent("private-canary");
  expect(api.Start).not.toHaveBeenCalled();
});


it("fulfills a tool capture automatically with the latest supported page, without navigation approval", async () => {
 function EnabledPage() {
  const page=useAssistantPage();
  const enablePage=page?.setEnabled;
  useLayoutEffect(()=>{enablePage?.(true)},[enablePage]);
  return <><main data-assistant-page="return-analysis"><section data-assistant-module="filters"><label htmlFor="analysis-from">From</label><input id="analysis-from" defaultValue="2026-10-01" /></section></main><ModelConversation /></>;
 }
 api.PendingPageReads.mockResolvedValueOnce([{id:"capture-first"}]).mockResolvedValue([]);
 render(<QueryClientProvider client={createTestQueryClient()}><AssistantPageProvider householdId="synthetic" incarnation="synthetic" surface="desktop" viewKey="return-analysis:" blocked={()=>false} onNavigate={vi.fn()}><EnabledPage /></AssistantPageProvider></QueryClientProvider>);
 await waitFor(()=>expect(api.DeliverPageContent).toHaveBeenCalledTimes(1));
 expect(api.DeliverPageContent.mock.calls[0][2].nodes).toEqual(expect.arrayContaining([expect.objectContaining({id:"filters/analysis-from",value:"2026-10-01"})]));
 fireEvent.change(screen.getByLabelText("From"),{target:{value:"2026-10-02"}});
 api.PendingPageReads.mockResolvedValueOnce([{id:"capture-second"}]);
 await waitFor(()=>expect(api.DeliverPageContent).toHaveBeenCalledTimes(2));
 expect(api.DeliverPageContent.mock.calls[1][2].nodes).toEqual(expect.arrayContaining([expect.objectContaining({id:"filters/analysis-from",value:"2026-10-02"})]));
 expect(api.GrantNavigation).not.toHaveBeenCalled(); expect(api.Start).not.toHaveBeenCalled();
});

it("revokes analysis navigation when a live filter changes during intent consumption", async () => {
  let finish!: (target: { kind: string; accountId: string }) => void;
  api.ConsumeNavigation.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  api.Start.mockImplementation(async (q) => {
    api.Inspect.mockResolvedValue({
      id: "conversation",
      runs: [{ ...run, requestId: q.requestId, state: "succeeded" }],
      runCount: 2,
      offset: 1,
      navigation: {
        id: "host-intent",
        leaseId: q.pageLeaseId,
        requestId: q.requestId,
        runId: "run",
        conversationId: "conversation",
      },
    });
    return { ...run, state: "accepted" };
  });
  const navigate = vi.fn();
  function Shell() {
    return <AssistantPageProvider householdId="synthetic" incarnation="synthetic" surface="desktop" viewKey="return-analysis:" blocked={()=>false} onNavigate={navigate}>
      <main data-assistant-page="return-analysis"><label htmlFor="analysis-from">From</label><input id="analysis-from" defaultValue="2026-10-01" /></main>
      <ModelConversation />
    </AssistantPageProvider>;
  }
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <Shell />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await screen.findByText("Synthetic prior answer");
  await user.type(screen.getByLabelText("Message"), "Open overview");
  await user.click(
    screen.getByRole("button", { name: "Send and allow one page change" }),
  );
  await waitFor(() => expect(api.ConsumeNavigation).toHaveBeenCalledTimes(1));
  fireEvent.change(screen.getByLabelText("From"), {target:{value:"2026-10-02"}});
  finish({ kind: "overview", accountId: "" });
  await screen.findByText(/No page change was applied/);
  expect(navigate).not.toHaveBeenCalled();
});


it.each(["return-analysis", "overview", "accounts", "available-funds", "portfolio", "history"])("captures %s with the page guide off and no conversation UI mounted", async (page) => {
 api.PendingPageReads.mockResolvedValueOnce([{id:"collapsed-sidebar-read"}]).mockResolvedValue([]);
 render(<QueryClientProvider client={createTestQueryClient()}><AssistantPageProvider householdId="synthetic" incarnation="synthetic" surface="desktop" viewKey={`${page}:`} blocked={()=>false} onNavigate={vi.fn()}><main data-assistant-page={page}><p>Latest underlying page</p></main></AssistantPageProvider></QueryClientProvider>);
 await waitFor(()=>expect(api.DeliverPageContent).toHaveBeenCalledTimes(1));
 expect(JSON.stringify(api.DeliverPageContent.mock.calls[0][2].nodes)).toContain("Latest underlying page");
 expect(api.GrantNavigation).not.toHaveBeenCalled();expect(api.Start).not.toHaveBeenCalled();

});

it("loads older messages in bounded batches and retains them when a new run arrives", async () => {
  const saved = (n: number) => ({ ...run, id: `run-${n}`, prompt: `Question ${n}`, text: `Answer ${n}`, evidence: [] });
  api.Inspect.mockResolvedValue({ id: "conversation", runs: [saved(7)], runCount: 8, offset: 7 });
  api.Messages.mockImplementation(async (_epoch, _id, offset) => ({ id: "conversation", runs: [saved(offset)], runCount: 8, offset }));
  const user = userEvent.setup();
  mount();
  await screen.findByText("Answer 2");
  expect(api.Messages).toHaveBeenCalledTimes(5);
  expect(screen.queryByText("Answer 0")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Load earlier messages" }));
  await screen.findByText("Answer 0");
  expect(screen.getByText("Answer 7")).toBeVisible();
  api.Start.mockImplementation(async () => {
    api.Inspect.mockResolvedValue({ id: "conversation", runs: [saved(8)], runCount: 9, offset: 8 });
    return saved(8);
  });
  await user.type(screen.getByLabelText("Message"), "Next question{Enter}");
  await screen.findByText("Answer 8");
  expect(screen.getByText("Answer 0")).toBeVisible();
  expect(screen.getByText("Answer 7")).toBeVisible();
  expect(screen.getAllByRole("article")).toHaveLength(9);
});

it("retries only failed history reads without replaying the model request", async () => {
  api.Inspect.mockResolvedValue({ id: "conversation", runs: [{ ...run, id: "latest", text: "Latest answer" }], runCount: 2, offset: 1 });
  api.Messages.mockRejectedValueOnce(new Error("private-history-payload"));
  const user = userEvent.setup();
  mount();
  await screen.findByText(/Earlier messages could not be loaded/);
  expect(screen.getByText("Latest answer")).toBeVisible();
  expect(screen.queryByText(/private-history-payload/)).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Try again" }));
  await screen.findByText("Synthetic prior answer");
  expect(api.Start).not.toHaveBeenCalled();
});

it("keeps privacy and navigation details keyboard accessible without granting on disclosure", async () => {
  const user = userEvent.setup();
  mount();
  await screen.findByText("Synthetic prior answer");
  const info = screen.getByRole("button", { name: "AI provider and privacy" });
  info.focus();
  await user.keyboard("{Enter}");
  expect(await screen.findByText(/Sending a message sends your question/)).toBeVisible();
  await user.keyboard("{Escape}");
  expect(info).toHaveFocus();
  const options = screen.getByText("Page navigation");
  await user.click(options);
  expect(screen.getByRole("button", { name: "Send and allow one page change" })).toBeVisible();
  expect(api.GrantNavigation).not.toHaveBeenCalled();
  expect(api.Start).not.toHaveBeenCalled();
});

it("displays only allowlisted tool names and never a raw source identifier", async () => {
  api.Inspect.mockResolvedValue({ id: "conversation", runs: [{ ...run, evidence: [{ ...run.evidence[0], source: "private_source_canary" }] }], runCount: 1, offset: 0 });
  const user = userEvent.setup();
  mount();
  await user.click(await screen.findByText(/Data reads · 1/));
  expect(screen.getByText("Ledger read")).toBeVisible();
  expect(screen.queryByText(/private_source_canary/)).not.toBeInTheDocument();
});


it("navigates to a page target with a draft without sending it to the model", async () => {
  const navigate = vi.fn();
  const observedAt = Date.now();
  function Publisher() {
    const setEnabled = useAssistantPage()?.setEnabled;
    useLayoutEffect(() => { setEnabled?.(true); }, [setEnabled]);
    usePublishAssistantPage({
      viewKey: "accounts:", page: "accounts", scope: { kind: "household", label: "Synthetic household" },
      timezone: "UTC", basis: "base", currency: "USD", observedAt, state: "available",
      targets: [{ label: "Synthetic checking", destination: { page: "accounts", accountId: "11111111-1111-4111-8111-111111111111" } }], metrics: [],
    });
    return null;
  }
  render(<QueryClientProvider client={createTestQueryClient()}>
    <AssistantPageProvider householdId="synthetic" incarnation="synthetic" surface="desktop" viewKey="accounts:" blocked={() => false} onNavigate={navigate}>
      <Publisher /><ModelConversation />
    </AssistantPageProvider>
  </QueryClientProvider>);
  const user = userEvent.setup();
  await screen.findByText("Synthetic prior answer");
  await user.type(screen.getByLabelText("Message"), "Do not send this draft");
  await user.click(screen.getByText("Page navigation"));
  const target = screen.getByRole("button", { name: "Synthetic checking" });
  expect(target).toHaveAttribute("type", "button");
  await user.click(target);
  expect(navigate).toHaveBeenCalledWith({ page: "accounts", accountId: "11111111-1111-4111-8111-111111111111" });
  expect(screen.getByLabelText("Message")).toHaveValue("Do not send this draft");
  expect(api.Start).not.toHaveBeenCalled();
  expect(api.GrantNavigation).not.toHaveBeenCalled();
});


it("retains observed answers while history is pending or failed and isolates other sessions", async () => {
  let rejectHistory!: (reason: Error) => void;
  api.Messages.mockImplementation(() => new Promise((_resolve, reject) => { rejectHistory = reject; }));
  api.List.mockResolvedValue([{ id: "conversation", updated: "2026-11-05", runCount: 1 }, { id: "other", updated: "2026-11-04", runCount: 1 }]);
  let current = { id: "conversation", runs: [run], runCount: 1, offset: 0 };
  api.Inspect.mockImplementation(async (_epoch, id) => id === "other"
    ? { id: "other", runs: [{ ...run, id: "other-run", prompt: "Other question", text: "Other session answer" }], runCount: 1, offset: 0 }
    : current);
  api.Start.mockImplementation(async q => {
    current = { id: "conversation", runs: [{ ...run, id: "new-run", prompt: q.prompt, text: "New answer" }], runCount: 2, offset: 1 };
    return current.runs[0];
  });
  const user = userEvent.setup();
  mount();
  await screen.findByText("Synthetic prior answer");
  await user.type(screen.getByLabelText("Message"), "Next question");
  await user.click(screen.getByRole("button", { name: "Send to model" }));
  await screen.findByText("New answer");
  expect(screen.getByText("Synthetic prior answer")).toBeVisible();
  expect(screen.getByText("Loading messages…")).toBeVisible();
  await act(async () => rejectHistory(new Error("private-history-error")));
  await screen.findByText(/Earlier messages could not be loaded/);
  expect(screen.getByText("Synthetic prior answer")).toBeVisible();
  expect(screen.getByText("New answer")).toBeVisible();
  expect(screen.getAllByRole("article")[0]).toHaveTextContent("Synthetic prior answer");
  await user.selectOptions(screen.getByLabelText("Conversations"), "other");
  await screen.findByText("Other session answer");
  expect(screen.queryByText("Synthetic prior answer")).not.toBeInTheDocument();
  expect(screen.queryByText("New answer")).not.toBeInTheDocument();
});

it("announces a new successful reply once without moving focus or announcing loaded history", async () => {
  const client = createTestQueryClient();
  let current = { id: "conversation", runs: [run], runCount: 1, offset: 0 };
  api.Inspect.mockImplementation(async () => current);
  api.Start.mockImplementation(async q => {
    current = { id: "conversation", runs: [{ ...run, id: "new-run", requestId: q.requestId, state: "running", text: "" }], runCount: 2, offset: 1 };
    return current.runs[0];
  });
  const user = userEvent.setup();
  mount("overview:", client);
  await screen.findByText("Synthetic prior answer");
  const notification = screen.getByRole("status");
  expect(notification).toHaveAttribute("aria-live", "polite");
  expect(notification).toHaveAttribute("aria-atomic", "true");
  expect(notification).toBeEmptyDOMElement();
  await user.type(screen.getByLabelText("Message"), "New question");
  await user.click(screen.getByRole("button", { name: "Send to model" }));
  await screen.findByRole("button", { name: "Stop model run" });
  const input = screen.getByLabelText("Message");
  await waitFor(() => expect(input).toBeEnabled());
  input.focus();
  current = { ...current, runs: [{ ...current.runs[0], state: "succeeded", text: "Fresh successful answer" }] };
  await client.invalidateQueries({ queryKey: ["aiConversation"] });
  await screen.findByText("AI reply received for message 2.");
  expect(input).toHaveFocus();
  const mutations = vi.fn();
  const observer = new MutationObserver(mutations);
  observer.observe(notification, { childList: true, characterData: true, subtree: true });
  await client.invalidateQueries({ queryKey: ["aiConversation"] });
  await client.invalidateQueries({ queryKey: ["aiConversation"] });
  expect(mutations).not.toHaveBeenCalled();
  expect(input).toHaveFocus();
  observer.disconnect();
});


it("announces an immediate first reply from a newly created conversation", async () => {
  api.List.mockResolvedValue([]);
  api.Start.mockImplementation(async q => {
    const reply = { ...run, id: "first-run", requestId: q.requestId, text: "Immediate first reply" };
    api.Inspect.mockResolvedValue({ id: "new-conversation", runs: [reply], runCount: 1, offset: 0 });
    return reply;
  });
  const user = userEvent.setup();
  mount();
  await waitFor(() => expect(screen.getByLabelText("Message")).toBeEnabled());
  await user.type(screen.getByLabelText("Message"), "First question");
  await user.click(screen.getByRole("button", { name: "Send to model" }));
  await screen.findByText("Immediate first reply");
  expect(await screen.findByText("AI reply received for message 1.")).toHaveAttribute("aria-live", "polite");

});

it("enables reviewed writes only through the separate session permission switch", async () => {
 const user=userEvent.setup();mount();
 await screen.findByText("Synthetic prior answer");
 expect(api.SetWritePermission).not.toHaveBeenCalled();
 await user.click(screen.getByRole("checkbox",{name:/Allow cash change proposals/}));
 await waitFor(()=>expect(api.SetWritePermission).toHaveBeenCalledExactlyOnceWith("host",3,true));
 expect(api.Start).not.toHaveBeenCalled();
 expect(api.ReviewAction).not.toHaveBeenCalled();
});

 it("requires explicit Full opt-in and never invokes a model or human confirmation",async()=>{
 const user=userEvent.setup();mount();await screen.findByText("Synthetic prior answer");
 const control=screen.getByRole("checkbox",{name:/Full for this app session/});expect(control).not.toBeChecked();
 control.focus();await user.keyboard(" ");
 await waitFor(()=>expect(api.SetFullAccess).toHaveBeenCalledExactlyOnceWith("host",3,true));
 expect(api.SetWritePermission).not.toHaveBeenCalled();expect(api.Start).not.toHaveBeenCalled();expect(api.ReviewAction).not.toHaveBeenCalled();
 });

it("displays generated titles and saves manual names using keyboard controls", async () => {
  const user = userEvent.setup();
  api.List.mockResolvedValue([{ id: "conversation", title: "资产趋势", titleSource: "luna", runCount: 1 }]);
  api.Inspect.mockResolvedValue({ id: "conversation", title: "资产趋势", titleSource: "luna", runs: [run], runCount: 1, offset: 0 });
  mount();
  await screen.findByRole("option", { name: "资产趋势" });
  await user.click(screen.getByRole("button", { name: "Rename conversation" }));
  const input = screen.getByRole("textbox", { name: "Conversation title" });
  expect(input).toHaveFocus();
  await user.clear(input);
  await user.type(input, "我的会话{Enter}");
  await waitFor(() => expect(api.Rename).toHaveBeenCalledWith("host", "conversation", "我的会话"));
  await waitFor(() => expect(screen.queryByRole("textbox", { name: "Conversation title" })).not.toBeInTheDocument());
});

it("cancels title editing without writing and preserves a failed rename for retry", async () => {
  const user = userEvent.setup();
  mount();
  await screen.findByText("Synthetic prior answer");
  await user.click(screen.getByRole("button", { name: "Rename conversation" }));
  await user.type(screen.getByRole("textbox", { name: "Conversation title" }), "Discard{Escape}");
  expect(api.Rename).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Rename conversation" }));
  api.Rename.mockRejectedValueOnce(new Error("synthetic storage failure"));
  await user.type(screen.getByRole("textbox", { name: "Conversation title" }), "Keep draft{Enter}");
  await screen.findByRole("alert");
  expect(screen.getByRole("textbox", { name: "Conversation title" })).toHaveValue("Keep draft");
});

it.each([
  ["en", "Rename conversation", "Conversation title", "Cancel rename"],
  ["zh-CN", "重命名会话", "会话标题", "取消重命名"],
  ["zh-TW", "重新命名對話", "對話標題", "取消重新命名"],
])("offers accessible rename controls in %s", async (locale, renameLabel, titleLabel, cancelLabel) => {
  await i18n.changeLanguage(locale);
  const user = userEvent.setup();
  mount();
  await screen.findByText("Synthetic prior answer");
  await user.click(screen.getByRole("button", { name: renameLabel }));
  expect(screen.getByRole("textbox", { name: titleLabel })).toHaveFocus();
  await user.click(screen.getByRole("button", { name: cancelLabel }));
  expect(api.Rename).not.toHaveBeenCalled();
});

it.each(["😀", "𠀀"])("accepts 64 Unicode code points when typing %s titles", async character => {
  const user = userEvent.setup();
  mount();
  await screen.findByText("Synthetic prior answer");
  await user.click(screen.getByRole("button", { name: "Rename conversation" }));
  const input = screen.getByRole("textbox", { name: "Conversation title" });
  const title = character.repeat(64);
  await user.type(input, title);
  expect(input).toHaveValue(title);
  await user.type(input, character);
  expect(input).toHaveValue(title);
  await user.click(screen.getByRole("button", { name: "Save title" }));
  await waitFor(() => expect(api.Rename).toHaveBeenCalledWith("host", "conversation", title));
});

it("limits mixed Unicode titles to 64 code points on paste", async () => {
  const user = userEvent.setup();
  mount();
  await screen.findByText("Synthetic prior answer");
  await user.click(screen.getByRole("button", { name: "Rename conversation" }));
  const input = screen.getByRole("textbox", { name: "Conversation title" });
  const title = "A".repeat(62) + "😀𠀀";
  await user.paste(title + "extra");
  expect(input).toHaveValue(title);
  await user.click(screen.getByRole("button", { name: "Save title" }));
  await waitFor(() => expect(api.Rename).toHaveBeenCalledWith("host", "conversation", title));
});

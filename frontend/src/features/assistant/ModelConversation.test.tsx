import { BusinessPageRoot } from "@/lib/businessCapture";
import { capturePageContent } from "./pageContent";
import { useLayoutEffect, useState } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider, onlineManager } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { AssistantPageProvider, useAssistantPage, usePublishAssistantPage } from "./PageContext";

import { ModelConversation } from "./ModelConversation";
import i18n from "@/i18n";
const api = vi.hoisted(() => ({
  Reviews: vi.fn(),
  SetFullAccess: vi.fn(),
  SelectModel: vi.fn(),
  PendingPageReads: vi.fn(),
  DeliverPageContent: vi.fn(),
  PublishPage: vi.fn(),

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
  () => ({ ...api, Usage: async () => (await import("@/test/aiUsage")).emptyUsageReport }),
);
const run = {
  id: "run",
  requestId: "saved-request",
  prompt: "Synthetic saved question",
  text: "Synthetic prior answer",
  evidence: [
    {
      source: "get_account_valuations",
      capturedAt: "2026-11-04T10:00:00Z",
      hasMore: true,
      missing: "reported",
    },
  ],
  state: "succeeded",
  model: "mock",
  created: "2026-11-04T10:00:00Z",
};
const configuredStatus = {
  epoch: "host", revision: 3, available: true, credentials: "present",
  config: { provider: "openai-responses", model: "mock", enabled: true },
};
beforeEach(() => {
  vi.clearAllMocks();
  api.Reviews.mockResolvedValue([]);
  api.SelectModel.mockResolvedValue(undefined);
  api.PendingPageReads.mockResolvedValue([]);
  api.DeliverPageContent.mockResolvedValue(undefined);
  api.PublishPage.mockResolvedValue({ id: "host-page-lease" });
  api.ConsumeNavigation.mockResolvedValue({ kind: "overview", accountId: "" });
  api.Status.mockResolvedValue(configuredStatus);
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
const historyTrigger = () => screen.getByRole("button", { name: new RegExp(`^${i18n.t("aiChat.history")}:`) });
async function openRename(user: ReturnType<typeof userEvent.setup>) {
  const rename = new RegExp(`^${i18n.t("aiChat.rename")}:`);
  if (!screen.queryByRole("button", { name: rename })) await user.click(historyTrigger());
  await user.click(await screen.findByRole("button", { name: rename }));
}
async function choosePermission(user: ReturnType<typeof userEvent.setup>, label: string) {
  await user.click(screen.getByRole("button", { name: new RegExp(`^${i18n.t("aiChat.permission")}:`) }));
  await user.click(await screen.findByRole("menuitemradio", { name: new RegExp(`^${label}`) }));
}
it("restores history without replay and sends only the question and selector", async () => {
  const user = userEvent.setup();
  mount();
  await screen.findByText("Synthetic prior answer");
  expect(api.Start).not.toHaveBeenCalled();
  await user.click(screen.getByText(/Data reads · 1/));
  expect(screen.getByText(/Account valuations/)).toBeVisible();
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
  expect(api.ConsumeNavigation).not.toHaveBeenCalled();
  expect(Object.keys(api.Start.mock.calls[0][0]).sort()).toEqual([
    "conversationId",
    "epoch",
    "model",
    "page",
    "pageLeaseId",
    "prompt",
    "reasoningEffort",
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
    reason: "disabled",
  });
  mount();
  await screen.findByText(/AI assistant is turned off/);
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

it("executes one typed navigation automatically with a current page lease", async () => {
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
  await user.click(
    screen.getByRole("button", { name: "Send to model" }),
  );
  await waitFor(() =>
    expect(navigate).toHaveBeenCalledWith({ page: "overview" }),
  );
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
  await user.click(
    screen.getByRole("button", { name: "Send to model" }),
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
    await user.click(
      screen.getByRole("button", { name: "Send to model" }),
    );
    await waitFor(() => expect(api.ConsumeNavigation).toHaveBeenCalledTimes(1));
    if (operation === "Delete") {
      await user.click(screen.getByRole("button", { name: /^Conversations:/ }));
      await user.click(await screen.findByRole("button", { name: /^Delete conversation:/ }));
      await user.click(await screen.findByRole("button", { name: "Delete" }));
    } else {
      const action = screen.getByRole("button", { name: label });
      await waitFor(() => expect(action).toBeEnabled());
      await user.click(action);
    }
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
  ["tool_result_rejected", "Check its result before retrying"],
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
 expect(api.DeliverPageContent.mock.calls[1][2].nodes).toEqual(expect.arrayContaining([expect.objectContaining({id:"filters/analysis-from",value:"2026-10-02"})])); expect(api.Start).not.toHaveBeenCalled();
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
    screen.getByRole("button", { name: "Send to model" }),
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
 expect(JSON.stringify(api.DeliverPageContent.mock.calls[0][2].nodes)).toContain("Latest underlying page");expect(api.Start).not.toHaveBeenCalled();

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
  expect(screen.queryByText("Page navigation")).not.toBeInTheDocument();
});

it("displays only allowlisted tool names and never a raw source identifier", async () => {
  api.Inspect.mockResolvedValue({ id: "conversation", runs: [{ ...run, evidence: [{ ...run.evidence[0], source: "private_source_canary" }] }], runCount: 1, offset: 0 });
  const user = userEvent.setup();
  mount();
  await user.click(await screen.findByText(/Data reads · 1/));
  expect(screen.getByText("Tool call")).toBeVisible();
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
  expect(screen.queryByRole("button", { name: "Synthetic checking" })).not.toBeInTheDocument();
  expect(navigate).not.toHaveBeenCalled();
  expect(screen.getByLabelText("Message")).toHaveValue("Do not send this draft");
  expect(api.Start).not.toHaveBeenCalled();
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
  await user.click(screen.getByRole("button", { name: /^Conversations:/ }));
  await user.click(await screen.findByRole("button", { name: /^Conversation 2/ }));
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

 it("requires explicit Full opt-in and never invokes a model or human confirmation",async()=>{
 const user=userEvent.setup();mount();await screen.findByText("Synthetic prior answer");
 render(<BusinessPageRoot view="synthetic:" accessIdentity="synthetic"><p>Ordinary business content</p></BusinessPageRoot>);
 const captured=capturePageContent("synthetic:");expect(JSON.stringify(captured)).not.toContain("Read & Write · Edit the ledger");expect(JSON.stringify(captured)).not.toContain("Allow cash change proposals");expect(captured.nodes.some(node=>node.name==="Permissions")).toBe(false);
 expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
 const control=screen.getByRole("button",{name:"Permissions: Read only"});
 control.focus();expect(control).toHaveFocus();await choosePermission(user,"Read & write");
 await waitFor(()=>expect(api.SetFullAccess).toHaveBeenCalledExactlyOnceWith("host",3,true));
 expect(api.Start).not.toHaveBeenCalled();
 });

it("displays generated titles and saves manual names using keyboard controls", async () => {
  const user = userEvent.setup();
  api.List.mockResolvedValue([{ id: "conversation", title: "资产趋势", titleSource: "luna", runCount: 1 }]);
  api.Inspect.mockResolvedValue({ id: "conversation", title: "资产趋势", titleSource: "luna", runs: [run], runCount: 1, offset: 0 });
  mount();
  await screen.findByRole("button", { name: /^Conversations: 资产趋势/ });
  await openRename(user);
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
  await openRename(user);
  await user.type(screen.getByRole("textbox", { name: "Conversation title" }), "Discard{Escape}");
  expect(api.Rename).not.toHaveBeenCalled();
  await openRename(user);
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
  await user.click(historyTrigger());
  await user.click(await screen.findByRole("button", { name: new RegExp(`^${renameLabel}:`) }));
  expect(screen.getByRole("textbox", { name: titleLabel })).toHaveFocus();
  await user.click(screen.getByRole("button", { name: cancelLabel }));
  expect(api.Rename).not.toHaveBeenCalled();
});

it.each(["😀", "𠀀"])("accepts 64 Unicode code points when typing %s titles", async character => {
  const user = userEvent.setup();
  mount();
  await screen.findByText("Synthetic prior answer");
  await openRename(user);
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
  await openRename(user);
  const input = screen.getByRole("textbox", { name: "Conversation title" });
  const title = "A".repeat(62) + "😀𠀀";
  await user.paste(title + "extra");
  expect(input).toHaveValue(title);
  await user.click(screen.getByRole("button", { name: "Save title" }));
  await waitFor(() => expect(api.Rename).toHaveBeenCalledWith("host", "conversation", title));
});

it.each([
  ["en", "Can edit the ledger without per-change confirmation."],
  ["zh-CN", "可直接修改账本，无需逐笔确认。"],
  ["zh-TW", "可直接修改帳本，無需逐筆確認。"],
])("shows only read-only and read-write modes with inline results in %s", async (locale, label) => {
  await i18n.changeLanguage(locale);
  api.Status.mockResolvedValue({ ...configuredStatus, fullAccess: true });
  api.Reviews.mockResolvedValue([{ id: "receipt", source: "ai_full", state: "succeeded", summary: "Duplicate result" }]);
  api.Inspect.mockResolvedValue({ id: "conversation", offset: 0, runCount: 1, runs: [{ ...run,
    tools: [{ source: "execute_ledger_change", state: "succeeded", input: '{"amount":"12.34567890"}', output: '{"status":"succeeded"}' }],
  }] });
  const user = userEvent.setup();
  mount();
  await screen.findByText("Synthetic prior answer");
  expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  const checkbox = screen.getByRole("button", { name: `${i18n.t("aiChat.permission")}: ${i18n.t("agent.readWrite")}` });
  expect(checkbox.className).toContain("bg-warning/15");
  await user.click(checkbox);
  expect(await screen.findAllByRole("menuitemradio")).toHaveLength(2);
  expect(screen.getByRole("menuitemradio", { name: new RegExp(`^${i18n.t("agent.readWrite")}`) })).toHaveAttribute("aria-checked", "true");
  expect(screen.getByRole("menuitemradio", { name: new RegExp(`^${i18n.t("agent.readOnly")}`) })).toHaveAttribute("aria-checked", "false");
  expect(screen.getByText(label)).toBeVisible();
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("menu")).not.toBeInTheDocument());
  expect(screen.queryByText("Duplicate result")).not.toBeInTheDocument();
  expect(screen.queryByRole("region", { name: /Ledger change review|账本变更审阅|帳本變更審閱/ })).not.toBeInTheDocument();
  const output = screen.getByText('{"status":"succeeded"}');
  await user.click(output.closest("details")!.querySelector("summary")!);
  expect(output).toBeVisible();
  await user.click(output.closest("details")!.querySelector("summary")!);
  expect(output).not.toBeVisible();
  checkbox.focus();
  await choosePermission(user, i18n.t("agent.readOnly"));
  await waitFor(() => expect(api.SetFullAccess).toHaveBeenCalledExactlyOnceWith("host", 3, false));

  expect(api.Start).not.toHaveBeenCalled();
});

it.each([false, true])("Full permission enabled=%s can only be revoked during a run", async enabled => {
  api.Status.mockResolvedValue({ ...configuredStatus, fullAccess: enabled });
  api.Inspect.mockResolvedValue({ id: "conversation", offset: 0, runCount: 1, runs: [{ ...run, state: "running" }] });
  mount();
  await screen.findByText("Synthetic prior answer");
  const checkbox = screen.getByRole("button", { name: /^Permissions:/ });
  if (enabled) {
    expect(checkbox).toBeEnabled();
    await choosePermission(userEvent.setup(), "Read only");
    await waitFor(() => expect(api.SetFullAccess).toHaveBeenCalledWith("host", 3, false));
  } else {
    expect(checkbox).toBeDisabled();
    expect(api.SetFullAccess).not.toHaveBeenCalled();
  }
});

it("keeps the authoritative permission and reports a failed toggle", async () => {
  api.SetFullAccess.mockRejectedValueOnce(new Error("permission save failed"));
  mount();
  await screen.findByText("Synthetic prior answer");
  await choosePermission(userEvent.setup(), "Read & write");
  await screen.findByRole("alert");
  expect(screen.getByRole("button", { name: "Permissions: Read only" })).toBeVisible();
  expect(api.Start).not.toHaveBeenCalled();
});

it.each([["succeeded", "Recorded"], ["failed", "Failed"]])("shows a recovered %s receipt on its failed tool call without replaying the write", async (state, label) => {
  api.Inspect.mockResolvedValue({ id: "conversation", offset: 0, runCount: 1, runs: [{ ...run,
    state: "failed", text: "", tools: [{ source: "execute_ledger_change", state: "failed", input: '{"amount":"12.34"}', output: '{"error":"lost reply"}' }],
  }] });
  const result = { id: "receipt", source: "ai_full", runId: "run", toolIndex: 0, toolName: "execute_ledger_change", state: "unknown" };
  let recoveredReceipt = false;
  api.Reviews.mockImplementation(async () => [{ ...result, state: recoveredReceipt ? state : "unknown" }]);
  mount();
  await screen.findByText("Read & Write ledger change · Outcome unknown");
  recoveredReceipt = true;
  const recovered = await screen.findByText(`Read & Write ledger change · ${label}`, {}, { timeout: 3000 });
  await userEvent.click(recovered.closest("summary")!);
  expect(screen.getByText(`Operation result · ${label}`)).toBeVisible();
  expect(screen.getByText('{"error":"lost reply"}')).toBeVisible();
  expect(api.Start).not.toHaveBeenCalled();
});


it.each(["stable", "publication-race", "capture-budget", "publication-failure"])("sends an ordinary question with optional page context: %s", async scenario => {
 const user=userEvent.setup();
 render(<QueryClientProvider client={createTestQueryClient()}>
  <AssistantPageProvider householdId="synthetic" incarnation="synthetic" surface="desktop" viewKey="overview:" blocked={()=>false} onNavigate={vi.fn()}>
   <BusinessPageRoot view="overview:" accessIdentity="synthetic"><div data-testid="optional-page-state">Initial page</div></BusinessPageRoot>
   <ModelConversation/>
  </AssistantPageProvider>
 </QueryClientProvider>);
 await screen.findByText("Synthetic prior answer");
 await waitFor(()=>expect(api.PublishPage).toHaveBeenCalled());
 api.PublishPage.mockClear();
 const content=screen.getByTestId("optional-page-state");
 if(scenario==="capture-budget") {
  let parent=content;
  for(let i=0;i<130;i++){const child=document.createElement("div");parent.append(child);parent=child;}
  expect(()=>capturePageContent("overview:")).toThrow("page_content_budget");
 }
 if(scenario==="publication-race") api.PublishPage.mockImplementationOnce(async()=>{
  await Promise.resolve();content.textContent="An asynchronous query changed the page";
  return {id:"raced-lease"};
 });
 if(scenario==="publication-failure") api.PublishPage.mockRejectedValueOnce(new Error("optional page publication unavailable"));
 await user.type(screen.getByLabelText("Message"),"Explain the difference between saving and investing");
 await user.click(screen.getByRole("button",{name:"Send to model"}));
 await waitFor(()=>expect(api.Start).toHaveBeenCalledTimes(1));
 expect(api.Start.mock.calls[0][0]).toMatchObject({prompt:"Explain the difference between saving and investing",pageLeaseId:scenario==="stable" ? "host-page-lease" : ""});
 await waitFor(()=>expect(screen.getByLabelText("Message")).toHaveValue(""));
 expect(api.ConsumeNavigation).not.toHaveBeenCalled();
 expect(screen.queryByText(i18n.t("aiChat.error"))).not.toBeInTheDocument();
 if(scenario==="capture-budget") expect(api.PublishPage).not.toHaveBeenCalled();
});

it("persists explicit model/effort choices and sends the selected values",async()=>{
 const user=userEvent.setup();
 let selection={model:"gpt-6-sol",reasoningEffort:"max"};
 api.Status.mockImplementation(async()=>({epoch:"host",revision:3,available:true,credentials:"present",selection,config:{provider:"openai-responses",model:"gpt-6-sol",enabledModels:["gpt-6-sol","gpt-6.1-sol"],enabled:true},modelCatalog:[{id:"gpt-6-sol",efforts:["none","high","max"]},{id:"gpt-6.1-sol",efforts:["high","max"]}]}));
 api.SelectModel.mockImplementation(async(_epoch,_revision,next)=>{selection=next;});
 mount();await screen.findByText("Synthetic prior answer");
 await user.click(screen.getByRole("button",{name:/^Model ID:/}));await user.click(await screen.findByRole("button",{name:"gpt-6.1-sol"}));
 await waitFor(()=>expect(screen.getByRole("button",{name:/Reasoning effort: High$/})).toBeInTheDocument());
 expect(api.SelectModel).toHaveBeenCalledWith("host",3,{model:"gpt-6.1-sol",reasoningEffort:"high"});
 await waitFor(()=>expect(screen.getByRole("button",{name:/^Model ID:/})).not.toBeDisabled());
 if(!screen.queryByRole("button",{name:"Maximum"}))await user.click(screen.getByRole("button",{name:/^Model ID:/}));
 await user.click(await screen.findByRole("button",{name:"Maximum"}));
 await waitFor(()=>expect(screen.getByRole("button",{name:/Reasoning effort: Maximum$/})).toBeInTheDocument());
 await user.type(screen.getByLabelText("Message"),"Synthetic model choice");
 await user.click(screen.getByRole("button",{name:"Send to model"}));
 await waitFor(()=>expect(api.Start).toHaveBeenCalledWith(expect.objectContaining({model:"gpt-6.1-sol",reasoningEffort:"max"})));
 expect(api.SetFullAccess).not.toHaveBeenCalled();
});
it("blocks a removed selection without replacing it or sending",async()=>{
 api.Status.mockResolvedValue({epoch:"host",revision:4,available:true,credentials:"present",selection:{model:"removed-model",reasoningEffort:""},config:{provider:"openai-responses",model:"gpt-6-sol",enabledModels:["gpt-6-sol"],enabled:true},modelCatalog:[]});
 const user=userEvent.setup();mount();await screen.findByText("Synthetic prior answer");await user.type(screen.getByLabelText("Message"),"Synthetic question");
 expect(screen.getByRole("button",{name:"Model ID: removed-model"})).toBeVisible();expect(screen.getByRole("button",{name:"Send to model"})).toBeDisabled();expect(api.SelectModel).not.toHaveBeenCalled();expect(api.Start).not.toHaveBeenCalled();
});

it("fills the draft from a suggested question without sending it", async () => {
  api.List.mockResolvedValue([]);
  const user = userEvent.setup();
  mount();
  const suggestion = await screen.findByRole("button", { name: i18n.t("aiChat.suggestionChange") });
  expect(screen.getByRole("group", { name: i18n.t("aiChat.suggestionsLabel") })).toBeVisible();
  await user.click(suggestion);
  expect(screen.getByLabelText("Message")).toHaveValue(i18n.t("aiChat.suggestionChange"));
  expect(screen.getByLabelText("Message")).toHaveFocus();
  expect(api.Start).not.toHaveBeenCalled();
  expect(api.Create).not.toHaveBeenCalled();
});

it("deletes a non-selected conversation without clearing the current selection", async () => {
  api.List.mockResolvedValue([{ id: "conversation", title: "Current", updated: "2026-11-05", runCount: 1 }, { id: "other", title: "Other", updated: "2026-11-04", runCount: 1 }]);
  const user = userEvent.setup();
  mount();
  await screen.findByText("Synthetic prior answer");
  await user.click(screen.getByRole("button", { name: /^Conversations:/ }));
  await user.click(await screen.findByRole("button", { name: "Delete conversation: Other" }));
  await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Delete" }));
  await waitFor(() => expect(api.Delete).toHaveBeenCalledExactlyOnceWith("host", "other"));
  expect(screen.getByRole("button", { name: "Conversations: Current" })).toBeVisible();
  expect(screen.getByText("Synthetic prior answer")).toBeVisible();
});


it("closes the permission menu when a background refresh fails", async () => {
  const user = userEvent.setup();
  const client = createTestQueryClient();
  mount("overview:", client);
  await screen.findByText("Synthetic prior answer");
  await user.click(screen.getByRole("button", { name: /^Permissions:/ }));
  await screen.findByRole("menuitemradio", { name: /^Read & write/ });
  api.Status.mockRejectedValue(new Error("Synthetic refresh failure"));
  await act(async () => { await client.invalidateQueries({ queryKey: ["aiCredentialStatus"] }); });
  await waitFor(() => expect(screen.getByRole("button", { name: /^Permissions:/ })).toBeDisabled());
  await waitFor(() => expect(screen.queryByRole("menu")).not.toBeInTheDocument());
  await user.keyboard("{ArrowDown}{Enter}");
  expect(api.SetFullAccess).not.toHaveBeenCalled();
});

it("blocks an already-open delete confirmation after a background refresh fails", async () => {
  const user = userEvent.setup();
  const client = createTestQueryClient();
  mount("overview:", client);
  await screen.findByText("Synthetic prior answer");
  await user.click(historyTrigger());
  await user.click(await screen.findByRole("button", { name: /^Delete conversation:/ }));
  const dialog = await screen.findByRole("alertdialog");
  api.Status.mockRejectedValue(new Error("Synthetic refresh failure"));
  await act(async () => { await client.invalidateQueries({ queryKey: ["aiCredentialStatus"] }); });
  const confirm = within(dialog).getByRole("button", { name: "Delete" });
  await waitFor(() => expect(confirm).toBeDisabled());
  await user.click(confirm);
  expect(api.Delete).not.toHaveBeenCalled();
  await user.click(within(dialog).getByRole("button", { name: "Keep" }));
  await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
});

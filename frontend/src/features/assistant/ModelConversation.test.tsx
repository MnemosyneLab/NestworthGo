import { useState } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider, onlineManager } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { AssistantPageProvider } from "./PageContext";
import { ModelConversation } from "./ModelConversation";
const api = vi.hoisted(() => ({
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
}));
vi.mock(
  "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service",
  () => api,
);
const run = {
  id: "run",
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
});
function mount(viewKey = "overview:") {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
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
  await user.click(screen.getByText("Data evidence"));
  expect(screen.getByText(/Account snapshot/)).toBeVisible();
  expect(screen.getByText(/This response advertised more pages/)).toBeVisible();
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
it("retrieves paged history and returns to the latest turn", async () => {
  api.Inspect.mockResolvedValue({
    id: "conversation",
    runs: [{ ...run, text: "Latest answer" }],
    runCount: 2,
    offset: 1,
  });
  const user = userEvent.setup();
  mount();
  await screen.findByText("Latest answer");
  await user.click(screen.getByRole("button", { name: "Previous turn" }));
  await screen.findByText("Synthetic prior answer");
  expect(api.Messages).toHaveBeenCalledWith("host", "conversation", 0);
  await user.click(screen.getByRole("button", { name: "Latest turn" }));
  await screen.findByText("Latest answer");
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
  const user = userEvent.setup();
  mount();
  await screen.findByText("Synthetic prior answer");
  expect(api.Status).toHaveBeenCalled();
  expect(api.List).toHaveBeenCalled();
  expect(api.Inspect).toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Previous turn" }));
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

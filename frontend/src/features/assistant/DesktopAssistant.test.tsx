import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createTestQueryClient } from "@/test/queryClient";
import { AssistantPageProvider } from "./PageContext";
import { useDesktopAssistant } from "./DesktopAssistant";

const api = vi.hoisted(() => ({ Status: vi.fn(), List: vi.fn(), Inspect: vi.fn(), Reviews: vi.fn(), Start: vi.fn() }));
const voice = vi.hoisted(() => ({ cancel: vi.fn() }));
const status = api.Status;
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service", () => ({ ...api, Usage: async () => (await import("@/test/aiUsage")).emptyUsageReport }));
vi.mock("./voiceRecorder", () => ({
  recordingType: () => ({ mimeType: "audio/mp4", format: "mp4" }),
  VoiceRecorder: class { async start() {} stop() {} cancel = voice.cancel; },
}));
function Shell() { const assistant = useDesktopAssistant(); return <div>{assistant.trigger}<main hidden={assistant.expanded}>Synthetic business page<button>Business action</button></main>{assistant.panel}</div>; }
beforeEach(() => {
  vi.resetAllMocks();
  status.mockResolvedValue({ epoch: "synthetic", revision: 1, available: true });
  api.List.mockResolvedValue([{ id: "one", title: "Recent", updated: "2026-11-06" }, { id: "two", title: "Earlier", updated: "2026-11-05" }]);
  api.Inspect.mockImplementation(async (_epoch: string, id: string) => ({ id, runs: [], runCount: 0 }));
  api.Reviews.mockResolvedValue([]);
});
afterEach(() => { vi.unstubAllGlobals(); });

function responsiveViewport(initial = true) {
  let wide = initial;
  const listeners = new Set<() => void>();
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: query === "(min-width: 1280px)" && wide,
    addEventListener: (_event: string, listener: () => void) => listeners.add(listener),
    removeEventListener: (_event: string, listener: () => void) => listeners.delete(listener),
  }));
  return (next: boolean) => act(() => { wide = next; for (const listener of [...listeners]) listener(); });
}

function mount() {
  const client = createTestQueryClient();
  render(<QueryClientProvider client={client}><Shell /></QueryClientProvider>);
  return client;
}

it("leaves closed surfaces idle, expands, collapses with focus, and pauses offline", async () => {
  vi.stubGlobal("matchMedia", (query: string) => ({ matches: query === "(min-width: 1280px)", addEventListener: () => {}, removeEventListener: () => {} }));
  status.mockResolvedValue({ version: 1, surface: "desktop", pageGuide: true, provider: "unavailable", credentials: "unavailable", conversations: "memory", modelRuns: false, financialWrites: false, reason: "activation_incomplete" });
  render(<QueryClientProvider client={createTestQueryClient()}><AssistantPageProvider householdId="synthetic" incarnation="generation" surface="desktop" viewKey="overview:" blocked={() => false} onNavigate={vi.fn()}><Shell /></AssistantPageProvider></QueryClientProvider>);
  // The desktop host reads local availability metadata even with the panel
  // closed; it still mounts no conversation and performs no model submission.
  expect(status).toHaveBeenCalled();
  expect(screen.queryByTestId("model-conversation")).not.toBeInTheDocument();
  const user = userEvent.setup(); await user.click(screen.getByRole("button", { name: "Open AI assistant" }));
  await waitFor(() => expect(status).toHaveBeenCalled());
  expect(screen.getByRole("heading", { name: "AI assistant" })).toHaveFocus();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  const dock = screen.getByRole("complementary", { name: "AI assistant" });
  expect(dock).toHaveClass("border-border");
  expect(dock.querySelector("header")).toHaveClass("border-border");
  await user.click(screen.getByRole("button", { name: "Business action" }));
  expect(screen.getByRole("button", { name: "Business action" })).toHaveFocus();
  await user.click(screen.getByRole("button", { name: "Expand workspace" }));
  expect(screen.getByText("Synthetic business page")).not.toBeVisible();
  act(() => window.dispatchEvent(new Event("offline")));
  // Match the browser's online state for the event-driven transport fence.
  vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
  act(() => window.dispatchEvent(new Event("offline")));
  expect(screen.getByText(/Network appears offline/)).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Collapse assistant" }));
  expect(screen.getByRole("button", { name: "Open AI assistant" })).toHaveFocus();
  expect(screen.getByText("Synthetic business page")).toBeVisible();
  vi.restoreAllMocks();
});


it("uses the shared sheet below the dock breakpoint and restores focus on Escape", async () => {
  status.mockResolvedValue({ version: 1, surface: "desktop", pageGuide: true, provider: "unavailable", credentials: "unavailable", conversations: "memory", modelRuns: false, financialWrites: false, reason: "activation_incomplete" });
  const user = userEvent.setup();
  render(<QueryClientProvider client={createTestQueryClient()}><Shell /></QueryClientProvider>);
  const trigger = screen.getByRole("button", { name: "Open AI assistant" });
  trigger.focus();
  await user.keyboard("{Enter}");
  expect(await screen.findByRole("dialog", { name: "AI assistant" })).toBeVisible();
  expect(screen.queryByRole("complementary")).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  await waitFor(() => expect(trigger).toHaveFocus());
});

it("keeps the draft and selected conversation across layout changes and Sheet dismissal", async () => {
  const resize = responsiveViewport();
  const client = mount();
  expect(api.List).not.toHaveBeenCalled();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Open AI assistant" }));
  await screen.findByRole("option", { name: "Earlier" });
  await user.selectOptions(screen.getByRole("combobox", { name: "Conversations" }), "two");
  fireEvent.change(screen.getByLabelText("Message"), { target: { value: "Unsent synthetic draft" } });
  resize(false);
  expect(await screen.findByRole("dialog", { name: "AI assistant" })).toBeVisible();
  expect(screen.getByLabelText("Message")).toHaveValue("Unsent synthetic draft");
  expect(screen.getByRole("combobox", { name: "Conversations" })).toHaveValue("two");
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await user.click(screen.getByRole("button", { name: "Open AI assistant" }));
  expect(screen.getByLabelText("Message")).toHaveValue("Unsent synthetic draft");
  expect(screen.getByRole("combobox", { name: "Conversations" })).toHaveValue("two");
  resize(true);
  expect(screen.getByLabelText("Message")).toHaveValue("Unsent synthetic draft");
  expect(screen.getByRole("combobox", { name: "Conversations" })).toHaveValue("two");
  // A replacement runtime still discards the old ledger's ephemeral state.
  await user.click(screen.getByRole("button", { name: "Collapse assistant" }));
  act(() => client.setQueryData(["aiCredentialStatus"], { epoch: "replacement", revision: 1, available: true }));
  await user.click(screen.getByRole("button", { name: "Open AI assistant" }));
  expect(screen.getByLabelText("Message")).toHaveValue("");
  expect(screen.getByRole("combobox", { name: "Conversations" })).not.toHaveValue("two");
  expect(api.Start).not.toHaveBeenCalled();
});

it("retains the submission guard while an accepted Start response crosses a layout change", async () => {
  const resize = responsiveViewport();
  let finish!: () => void;
  api.Start.mockImplementation(() => new Promise<void>(resolve => { finish = resolve; }));
  mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Open AI assistant" }));
  await screen.findByRole("option", { name: "Recent" });
  fireEvent.change(screen.getByLabelText("Message"), { target: { value: "Send once" } });
  await user.click(screen.getByRole("button", { name: "Send to model" }));
  await waitFor(() => expect(api.Start).toHaveBeenCalledOnce());
  resize(false);
  expect(screen.getByLabelText("Message")).toHaveValue("Send once");
  expect(screen.getByRole("button", { name: "Send to model" })).toBeDisabled();
  fireEvent.submit(screen.getByLabelText("Message").closest("form")!);
  expect(api.Start).toHaveBeenCalledOnce();
  await act(async () => finish());
  await waitFor(() => expect(screen.getByLabelText("Message")).toHaveValue(""));
  expect(api.Start).toHaveBeenCalledOnce();
});

it("cancels audio when the composer hides or remounts without discarding its draft", async () => {
  const resize = responsiveViewport();
  mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Open AI assistant" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Voice input" })).toBeEnabled());
  fireEvent.change(screen.getByLabelText("Message"), { target: { value: "Keep typed text" } });
  await user.click(screen.getByRole("button", { name: "Voice input" }));
  resize(false);
  expect(voice.cancel).toHaveBeenCalledOnce();
  expect(screen.getByLabelText("Message")).toHaveValue("Keep typed text");
  await user.click(screen.getByRole("button", { name: "Voice input" }));
  await user.keyboard("{Escape}");
  await waitFor(() => expect(voice.cancel).toHaveBeenCalledTimes(2));
  expect(api.Start).not.toHaveBeenCalled();
});

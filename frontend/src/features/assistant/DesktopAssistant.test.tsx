import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createTestQueryClient } from "@/test/queryClient";
import { AssistantPageProvider } from "./PageContext";
import { useDesktopAssistant } from "./DesktopAssistant";
import { useUiStore } from "@/stores/ui";
import i18n from "@/i18n";

const api = vi.hoisted(() => ({ Status: vi.fn(), List: vi.fn(), Inspect: vi.fn(), Reviews: vi.fn(), Start: vi.fn() }));
const voice = vi.hoisted(() => ({ cancel: vi.fn() }));
const status = api.Status;
const configuredStatus = {
  epoch: "synthetic", revision: 1, available: true,
  config: { provider: "openai-responses", model: "mock", enabledModels: ["mock"], enabled: true },
  selection: { model: "mock", reasoningEffort: "" },
};
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service", () => ({ ...api, Usage: async () => (await import("@/test/aiUsage")).emptyUsageReport }));
vi.mock("./voiceRecorder", () => ({
  recordingType: () => ({ mimeType: "audio/mp4", format: "mp4" }),
  VoiceRecorder: class { async start() {} stop() {} cancel = voice.cancel; },
}));
function Shell() { const assistant = useDesktopAssistant(); return <div data-testid="assistant-workspace">{assistant.trigger}<main hidden={assistant.expanded}>Synthetic business page<button>Business action</button></main>{assistant.panel}</div>; }
beforeEach(() => {
  vi.resetAllMocks();
  useUiStore.setState({ assistantWidth: useUiStore.getInitialState().assistantWidth });
  status.mockResolvedValue(configuredStatus);
  api.List.mockResolvedValue([{ id: "one", title: "Recent", updated: "2026-11-06" }, { id: "two", title: "Earlier", updated: "2026-11-05" }]);
  api.Inspect.mockImplementation(async (_epoch: string, id: string) => ({ id, runs: [], runCount: 0 }));
  api.Reviews.mockResolvedValue([]);
});
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });

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

function workspaceSize(initial: number) {
  let width = initial;
  const observers = new Set<() => void>();
  const measure = HTMLElement.prototype.getBoundingClientRect;
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    return this.dataset.testid === "assistant-workspace" ? new DOMRect(0, 0, width, 900) : measure.call(this);
  });
  vi.stubGlobal("ResizeObserver", class {
    constructor(private callback: () => void) {}
    observe(element: HTMLElement) { if (element.dataset.testid === "assistant-workspace") observers.add(this.callback); }
    disconnect() { observers.delete(this.callback); }
  });
  return (next: number) => act(() => { width = next; for (const update of observers) update(); });
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
  await screen.findByRole("button", { name: "Conversations: Recent" });
  await user.click(screen.getByRole("button", { name: "Conversations: Recent" }));
  await user.click(await screen.findByRole("button", { name: /^Earlier/ }));
  fireEvent.change(screen.getByLabelText("Message"), { target: { value: "Unsent synthetic draft" } });
  resize(false);
  expect(await screen.findByRole("dialog", { name: "AI assistant" })).toBeVisible();
  expect(screen.getByLabelText("Message")).toHaveValue("Unsent synthetic draft");
  expect(screen.getByRole("button", { name: "Conversations: Earlier" })).toBeVisible();
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await user.click(screen.getByRole("button", { name: "Open AI assistant" }));
  expect(screen.getByLabelText("Message")).toHaveValue("Unsent synthetic draft");
  expect(screen.getByRole("button", { name: "Conversations: Earlier" })).toBeVisible();
  resize(true);
  expect(screen.getByLabelText("Message")).toHaveValue("Unsent synthetic draft");
  expect(screen.getByRole("button", { name: "Conversations: Earlier" })).toBeVisible();
  // A replacement runtime still discards the old ledger's ephemeral state.
  await user.click(screen.getByRole("button", { name: "Collapse assistant" }));
  act(() => client.setQueryData(["aiCredentialStatus"], { ...configuredStatus, epoch: "replacement" }));
  await user.click(screen.getByRole("button", { name: "Open AI assistant" }));
  expect(screen.getByLabelText("Message")).toHaveValue("");
  expect(await screen.findByRole("button", { name: "Conversations: Recent" })).toBeVisible();
  expect(api.Start).not.toHaveBeenCalled();
});

it("retains the submission guard while an accepted Start response crosses a layout change", async () => {
  const resize = responsiveViewport();
  let finish!: () => void;
  api.Start.mockImplementation(() => new Promise<void>(resolve => { finish = resolve; }));
  mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Open AI assistant" }));
  await screen.findByRole("button", { name: "Conversations: Recent" });
  fireEvent.change(screen.getByLabelText("Message"), { target: { value: "Send once" } });
  const send = screen.getByRole("button", { name: "Send to model" });
  expect(send).toBeEnabled();
  await user.click(send);
  await waitFor(() => expect(api.Start).toHaveBeenCalledOnce());
  expect(api.Start).toHaveBeenCalledWith(expect.objectContaining({ model: "mock", reasoningEffort: "" }));
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

it("resizes the dock without remounting the draft or recording and restores its preferred width", async () => {
  const resizeViewport = responsiveViewport();
  const resizeWorkspace = workspaceSize(1216);
  mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Open AI assistant" }));
  await screen.findByRole("button", { name: "Conversations: Recent" });
  await user.click(screen.getByRole("button", { name: "Conversations: Recent" }));
  await user.click(await screen.findByRole("button", { name: /^Earlier/ }));
  const input = screen.getByLabelText("Message");
  fireEvent.change(input, { target: { value: "Keep this draft while resizing" } });
  await user.click(screen.getByRole("button", { name: "Voice input" }));
  const dock = screen.getByRole("complementary", { name: "AI assistant" });
  const handle = screen.getByRole("separator", { name: "AI assistant" });
  handle.setPointerCapture = vi.fn();
  handle.hasPointerCapture = () => false;
  fireEvent.pointerDown(handle, { pointerId: 1, button: 0, clientX: 1000, isPrimary: true });
  fireEvent.pointerMove(handle, { pointerId: 1, clientX: 880 });
  fireEvent.pointerUp(handle, { pointerId: 1 });
  expect(dock).toHaveStyle({ width: "600px" });
  expect(screen.getByLabelText("Message")).toBe(input);
  expect(input).toHaveValue("Keep this draft while resizing");
  expect(screen.getByRole("button", { name: "Conversations: Earlier" })).toBeVisible();
  expect(voice.cancel).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Expand workspace" }));
  expect(screen.queryByRole("separator")).not.toBeInTheDocument();
  expect(dock.style.width).toBe("");
  await user.click(screen.getByRole("button", { name: "Return to page" }));
  expect(dock).toHaveStyle({ width: "600px" });
  expect(screen.getByLabelText("Message")).toBe(input);
  expect(voice.cancel).not.toHaveBeenCalled();
  // Window or navigation-rail changes clamp the rendered width, not the saved
  // preference, leaving at least 480px of this workspace for the business page.
  resizeWorkspace(1056);
  expect(dock).toHaveStyle({ width: "576px" });
  expect(screen.getByRole("separator")).toHaveAttribute("aria-valuemax", "576");
  expect(useUiStore.getState().assistantWidth).toBe(600);
  resizeWorkspace(1216);
  expect(dock).toHaveStyle({ width: "600px" });
  screen.getByRole("separator").focus();
  await user.keyboard("{Enter}");
  expect(screen.getByRole("button", { name: "Open AI assistant" })).toHaveFocus();
  expect(voice.cancel).toHaveBeenCalledOnce();
  await user.keyboard("{Enter}");
  expect(dock).toHaveStyle({ width: "600px" });
  resizeViewport(false);
  expect(screen.queryByRole("separator")).not.toBeInTheDocument();
  expect(await screen.findByRole("dialog", { name: "AI assistant" })).toBeVisible();
  expect(screen.getByLabelText("Message")).toHaveValue("Keep this draft while resizing");
  resizeViewport(true);
  expect(screen.getByRole("complementary", { name: "AI assistant" })).toHaveStyle({ width: "600px" });
  expect(screen.getByRole("button", { name: "Conversations: Earlier" })).toBeVisible();
  expect(api.Start).not.toHaveBeenCalled();
});

it.each(["before", "during"])("restores the saved width on Escape when the workspace shrinks %s a drag", async timing => {
  responsiveViewport();
  const resizeWorkspace = workspaceSize(timing === "before" ? 1056 : 1216);
  useUiStore.setState({ assistantWidth: 600 });
  mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Open AI assistant" }));
  const input = screen.getByLabelText("Message");
  await waitFor(() => expect(input).toBeEnabled());
  fireEvent.change(input, { target: { value: "Keep draft while cancelling" } });
  const dock = screen.getByRole("complementary", { name: "AI assistant" });
  const handle = screen.getByRole("separator", { name: "AI assistant" });
  expect(dock).toHaveStyle({ width: timing === "before" ? "576px" : "600px" });
  expect(useUiStore.getState().assistantWidth).toBe(600);
  handle.setPointerCapture = vi.fn();
  handle.hasPointerCapture = () => false;
  fireEvent.pointerDown(handle, { pointerId: 1, button: 0, clientX: 1000, isPrimary: true });
  fireEvent.pointerMove(handle, { pointerId: 1, clientX: 1080 });
  expect(dock).toHaveStyle({ width: timing === "before" ? "496px" : "520px" });
  if (timing === "during") resizeWorkspace(1056);
  await user.keyboard("{Escape}");
  expect(useUiStore.getState().assistantWidth).toBe(600);
  expect(dock).toHaveStyle({ width: "576px" });
  expect(handle).toHaveAttribute("aria-valuenow", "576");
  resizeWorkspace(1216);
  expect(dock).toHaveStyle({ width: "600px" });
  expect(screen.getByLabelText("Message")).toBe(input);
  expect(input).toHaveValue("Keep draft while cancelling");
  expect(document.body.style.cursor).toBe("");
  expect(api.Start).not.toHaveBeenCalled();
});

it.each([
  ["en", "AI assistant", "Resize AI assistant", "Width: 480 pixels"],
  ["zh-CN", "AI 助手", "调整 AI 助手宽度", "宽度 480 像素"],
  ["zh-TW", "AI 助手", "調整 AI 助手寬度", "寬度 480 像素"],
])("labels the adjustable width in %s", async (locale, label, title, valueText) => {
  responsiveViewport();
  await i18n.changeLanguage(locale);
  mount();
  await userEvent.click(screen.getByRole("button", { name: i18n.t("assistant.open") }));
  const handle = screen.getByRole("separator", { name: label });
  expect(handle).toHaveAttribute("title", title);
  expect(handle).toHaveAttribute("aria-valuetext", valueText);
  expect(handle).toHaveAttribute("aria-controls", "desktop-assistant");
});


it("starts wider, clamps to a small workspace and preserves the preferred width", async () => {
  const resizeViewport = responsiveViewport();
  const resizeWorkspace = workspaceSize(1216);
  mount();
  await userEvent.click(screen.getByRole("button", { name: "Open AI assistant" }));
  const dock = screen.getByRole("complementary", { name: "AI assistant" });
  expect(dock).toHaveStyle({ width: "480px" });
  resizeWorkspace(880);
  expect(dock).toHaveStyle({ width: "400px" });
  expect(useUiStore.getState().assistantWidth).toBe(480);
  resizeViewport(false);
  expect(await screen.findByRole("dialog", { name: "AI assistant" })).toBeVisible();
  expect(screen.queryByRole("separator")).not.toBeInTheDocument();
  resizeWorkspace(1216);
  resizeViewport(true);
  expect(screen.getByRole("complementary", { name: "AI assistant" })).toHaveStyle({ width: "480px" });
});

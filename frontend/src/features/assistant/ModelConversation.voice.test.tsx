import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { createTestQueryClient } from "@/test/queryClient";
import { ModelConversation } from "./ModelConversation";
import i18n from "@/i18n";

const api = vi.hoisted(() => ({ Status: vi.fn(), List: vi.fn(), Inspect: vi.fn(), Reviews: vi.fn(), Start: vi.fn(), Transcribe: vi.fn() }));
const voice = vi.hoisted(() => ({ result: (_: string) => {}, cancel: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service", () => ({ ...api, Usage: async () => (await import("@/test/aiUsage")).emptyUsageReport }));
vi.mock("./voiceRecorder", () => ({
  recordingType: () => ({ mimeType: "audio/mp4", format: "mp4" }),
  VoiceRecorder: class {
    constructor(_: unknown, __: unknown, ___: unknown, result: typeof voice.result) { voice.result = result; }
    async start() {} stop() {} cancel = voice.cancel;
  },
}));
beforeEach(() => {
  vi.clearAllMocks();
  api.Status.mockResolvedValue({ epoch: "synthetic", revision: 1, available: true,
    config: { provider: "openai-responses", model: "mock", enabled: true } });
  api.List.mockResolvedValue([{ id: "one", title: "First" }, { id: "two", title: "Second" }]);
  api.Inspect.mockResolvedValue({ id: "one", title: "First", runs: [], runCount: 0 });
  api.Reviews.mockResolvedValue([]);
  api.Start.mockResolvedValue({ id: "synthetic-run", state: "accepted" });
});
function mount() {
  const client = createTestQueryClient();
  const tree = (visible: boolean) => <QueryClientProvider client={client}><ModelConversation visible={visible} /></QueryClientProvider>;
  const result = render(tree(true));
  return { ...result, hide: () => result.rerender(tree(false)) };
}
it("appends to the latest editable draft without submitting or replacing it", async () => {
  mount();
  const input = screen.getByLabelText("Message");
  await waitFor(() => expect(input).toBeEnabled());
  fireEvent.change(input, { target: { value: "Existing question" } });
  fireEvent.click(screen.getByRole("button", { name: "Voice input" }));
  fireEvent.change(input, { target: { value: "Edited while recording" } });
  act(() => voice.result("语音问题"));
  expect(input).toHaveValue("Edited while recording 语音问题");
  expect(input).toHaveFocus();
  expect(api.Start).not.toHaveBeenCalled();
  fireEvent.change(input, { target: { value: "User corrected transcript" } });
  expect(input).toHaveValue("User corrected transcript");
});
it("cancels recording when the retained dock is hidden", async () => {
  const view = mount();
  await waitFor(() => expect(screen.getByRole("button", { name: "Voice input" })).toBeEnabled());
  fireEvent.click(screen.getByRole("button", { name: "Voice input" }));
  view.hide(); expect(voice.cancel).toHaveBeenCalled(); expect(api.Start).not.toHaveBeenCalled();
});
it("keeps an oversized combined draft editable with shared validation", async () => {
  mount(); const input = screen.getByLabelText("Message");
  await waitFor(() => expect(input).toBeEnabled());
  fireEvent.change(input, { target: { value: "a".repeat(16380) } });
  fireEvent.click(screen.getByRole("button", { name: "Voice input" }));
  act(() => voice.result("more text"));
  expect(input).toHaveAttribute("aria-invalid", "true");
  expect(screen.getByRole("button", { name: "Send to model" })).toBeDisabled();
  expect(api.Start).not.toHaveBeenCalled();
});

it.each(["en", "zh-CN", "zh-TW"])("keeps the microphone immediately left of the icon-only send action in %s", async language => {
  const user = userEvent.setup();
  await i18n.changeLanguage(language);
  mount();
  await waitFor(() => expect(screen.getByRole("button", { name: i18n.t("voice.start") })).toBeEnabled());
  const microphone = screen.getByRole("button", { name: i18n.t("voice.start") });
  const send = screen.getByRole("button", { name: i18n.t("aiChat.send") });
  expect(microphone.textContent).toBe("");
  expect(send.textContent).toBe("");
  expect(send.querySelector("svg")).toBeInTheDocument();
  expect(microphone.parentElement?.nextElementSibling).toBe(send);
  expect(send.parentElement).toHaveClass("flex", "shrink-0");
  expect(send.parentElement).not.toHaveClass("flex-wrap");
  expect(send).toBeDisabled();
  expect(screen.getByText(i18n.t("voice.privacy"))).toHaveClass("sr-only");
  fireEvent.change(screen.getByLabelText(i18n.t("aiChat.message")), { target: { value: "Synthetic icon-only submission" } });
  expect(send).toBeEnabled();
  await user.click(send);
  await waitFor(() => expect(api.Start).toHaveBeenCalledOnce());
  expect(api.Start).toHaveBeenCalledWith(expect.objectContaining({ prompt: "Synthetic icon-only submission", model: "mock" }));
});

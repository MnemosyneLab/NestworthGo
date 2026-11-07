import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { VoiceInput } from "./VoiceInput";
import i18n from "@/i18n";
import { toast } from "sonner";
const mock = vi.hoisted(() => ({ start: vi.fn(), stop: vi.fn(), cancel: vi.fn(), supported: true,
  state: (_: string) => {}, result: (_: string) => {}, failure: (_: string) => {} }));
vi.mock("./voiceTranscription", () => ({ transcribeVoice: vi.fn() }));
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));
vi.mock("./voiceRecorder", () => ({
  recordingType: () => mock.supported ? { mimeType: "audio/mp4", format: "mp4" } : undefined,
  VoiceRecorder: class {
    constructor(state: typeof mock.state, failure: typeof mock.failure, _: unknown, result: typeof mock.result) { mock.state = state; mock.failure = failure; mock.result = result; }
    start = mock.start; stop = mock.stop; cancel = mock.cancel;
  },
}));
beforeEach(() => { mock.supported = true; vi.clearAllMocks(); void i18n.changeLanguage("en"); });
afterEach(() => { void i18n.changeLanguage("en"); });
it("offers icon-only keyboard recording, stop and cancel, and only emits draft text", async () => {
  const onText = vi.fn(); const user = userEvent.setup();
  render(<VoiceInput epoch="e" revision={1} disabled={false} onText={onText} />);
  const start = screen.getByRole("button", { name: "Voice input" });
  expect(start.textContent).toBe("");
  expect(start.querySelector("svg")).toBeInTheDocument();
  expect(start).toHaveAccessibleDescription(i18n.t("voice.privacy"));
  expect(screen.getByText(/Stopping sends audio to OpenAI/)).toHaveClass("sr-only");
  await user.tab(); await user.keyboard("{Enter}"); expect(mock.start).toHaveBeenCalledOnce();
  act(() => mock.state("recording"));
  const stop = screen.getByRole("button", { name: "Stop and transcribe" });
  expect(stop).toHaveAttribute("aria-pressed", "true");
  expect(stop.textContent).toBe("");
  expect(stop.previousElementSibling).toBe(screen.getByRole("button", { name: "Cancel" }));
  await user.click(stop); expect(mock.stop).toHaveBeenCalledOnce();
  act(() => mock.state("transcribing")); expect(screen.getByRole("status")).toHaveTextContent("Transcribing");
  expect(onText).not.toHaveBeenCalled(); act(() => mock.result("draft text")); expect(onText).toHaveBeenCalledWith("draft text");
  const cancel = screen.getByRole("button", { name: "Cancel" });
  expect(cancel.textContent).toBe("");
  cancel.focus(); await user.keyboard("{Enter}"); expect(mock.cancel).toHaveBeenCalled();
  expect(screen.getByRole("button", { name: "Voice input" })).toHaveAttribute("aria-pressed", "false");
  await user.click(screen.getByRole("button", { name: "Voice input" }));
  expect(mock.start).toHaveBeenCalledTimes(2);
});
it("cancels on disabled state, runtime replacement and unmount", async () => {
  const props = { epoch: "e", revision: 1, disabled: false, onText: vi.fn() };
  const view = render(<VoiceInput {...props} />);
  fireEvent.click(screen.getByRole("button", { name: "Voice input" }));
  view.rerender(<VoiceInput {...props} disabled />); expect(mock.cancel).toHaveBeenCalled();
  expect(screen.getByRole("button", { name: "Voice input" })).toBeDisabled();
  view.rerender(<VoiceInput {...props} epoch="new" />); fireEvent.click(screen.getByRole("button", { name: "Voice input" }));
  mock.cancel.mockClear(); view.unmount(); expect(mock.cancel).toHaveBeenCalledOnce();
});
it("shows localized denied-permission recovery and unsupported clients", async () => {
  await i18n.changeLanguage("zh-CN");
  const view = render(<VoiceInput epoch="e" revision={1} disabled={false} onText={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: "语音输入" }));
  act(() => mock.failure("permissionDenied"));
  expect(toast.error).toHaveBeenCalledWith(i18n.t("voice.permissionDenied"));
  expect(screen.getByRole("button", { name: "语音输入" })).toHaveAttribute("title", i18n.t("voice.permissionDenied"));
  expect(screen.getByRole("button", { name: "语音输入" })).toHaveAccessibleDescription(`${i18n.t("voice.privacy")} ${i18n.t("voice.permissionDenied")}`);
  mock.supported = false; view.rerender(<VoiceInput epoch="e" revision={2} disabled={false} onText={vi.fn()} />);
  expect(screen.getByRole("button", { name: "语音输入" })).toBeDisabled();
  expect(screen.getByText(/此客户端不支持语音输入/)).toHaveClass("sr-only");
  expect(screen.getByRole("button", { name: "语音输入" })).toHaveAttribute("title", i18n.t("voice.unsupported"));
  expect(screen.getByRole("button", { name: "语音输入" }).parentElement).toHaveAttribute("title", i18n.t("voice.unsupported"));
});

it.each(["permission", "transcribing"])("keeps %s compact and cancellable without starting twice", async state => {
  const user = userEvent.setup();
  render(<VoiceInput epoch="e" revision={1} disabled={false} onText={vi.fn()} />);
  await user.click(screen.getByRole("button", { name: "Voice input" }));
  act(() => mock.state(state));
  const control = screen.getByRole("button", { name: i18n.t(`voice.${state}`) });
  expect(control).toBeDisabled();
  expect(control).toHaveAttribute("aria-busy", "true");
  expect(control.textContent).toBe("");
  expect(screen.getByRole("status")).toHaveClass("sr-only");
  await user.click(control);
  expect(mock.start).toHaveBeenCalledOnce();
  await user.click(screen.getByRole("button", { name: "Cancel" }));
  expect(mock.cancel).toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();
});

it.each(["en", "zh-CN", "zh-TW"])("keeps localized names and descriptions without visible labels in %s", async language => {
  await i18n.changeLanguage(language);
  render(<VoiceInput epoch="e" revision={1} disabled={false} onText={vi.fn()} />);
  const start = screen.getByRole("button", { name: i18n.t("voice.start") });
  expect(start.textContent).toBe("");
  expect(start).toHaveAccessibleDescription(i18n.t("voice.privacy"));
  fireEvent.click(start);
  act(() => mock.state("recording"));
  expect(screen.getByRole("button", { name: i18n.t("voice.stop") }).textContent).toBe("");
  expect(screen.getByRole("button", { name: i18n.t("voice.cancel") }).textContent).toBe("");
  expect(screen.getByRole("status")).toHaveTextContent(i18n.t("voice.recording"));
});

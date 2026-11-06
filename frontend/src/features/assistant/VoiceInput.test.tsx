import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { VoiceInput } from "./VoiceInput";
import i18n from "@/i18n";
const mock = vi.hoisted(() => ({ start: vi.fn(), stop: vi.fn(), cancel: vi.fn(), supported: true,
  state: (_: string) => {}, result: (_: string) => {}, failure: (_: string) => {} }));
vi.mock("./voiceTranscription", () => ({ transcribeVoice: vi.fn() }));
vi.mock("./voiceRecorder", () => ({
  recordingType: () => mock.supported ? { mimeType: "audio/mp4", format: "mp4" } : undefined,
  VoiceRecorder: class {
    constructor(state: typeof mock.state, failure: typeof mock.failure, _: unknown, result: typeof mock.result) { mock.state = state; mock.failure = failure; mock.result = result; }
    start = mock.start; stop = mock.stop; cancel = mock.cancel;
  },
}));
beforeEach(() => { mock.supported = true; vi.clearAllMocks(); void i18n.changeLanguage("en"); });
afterEach(() => { void i18n.changeLanguage("en"); });
it("offers keyboard recording, stop and cancel, discloses upload, and only emits draft text", async () => {
  const onText = vi.fn(); const user = userEvent.setup();
  render(<VoiceInput epoch="e" revision={1} disabled={false} onText={onText} />);
  expect(screen.getByText(/Stopping sends audio to OpenAI/)).toBeInTheDocument();
  await user.tab(); await user.keyboard("{Enter}"); expect(mock.start).toHaveBeenCalledOnce();
  act(() => mock.state("recording")); await user.click(screen.getByRole("button", { name: "Stop and transcribe" })); expect(mock.stop).toHaveBeenCalledOnce();
  act(() => mock.state("transcribing")); expect(screen.getByRole("status")).toHaveTextContent("Transcribing");
  expect(onText).not.toHaveBeenCalled(); act(() => mock.result("draft text")); expect(onText).toHaveBeenCalledWith("draft text");
  await user.click(screen.getByRole("button", { name: "Cancel" })); expect(mock.cancel).toHaveBeenCalled();
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
  act(() => mock.failure("permissionDenied")); expect(screen.getByRole("alert")).toHaveTextContent("麦克风权限被拒绝");
  mock.supported = false; view.rerender(<VoiceInput epoch="e" revision={2} disabled={false} onText={vi.fn()} />);
  expect(screen.getByRole("button", { name: "语音输入" })).toBeDisabled();
  expect(screen.getByText(/此客户端不支持语音输入/)).toBeInTheDocument();
});

import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { MAX_AUDIO_BYTES, VoiceRecorder } from "./voiceRecorder";
// jsdom lacks Blob.arrayBuffer; keep this deterministic test double browser-typed.
class MemoryBlob {
  private bytes: Uint8Array;
  size: number;
  constructor(parts: (string | MemoryBlob)[]) {
    const chunks = parts.map(part => typeof part === "string" ? new TextEncoder().encode(part) : part.bytes);
    this.size = chunks.reduce((size, chunk) => size + chunk.length, 0);
    this.bytes = new Uint8Array(this.size);
    let offset = 0;
    for (const chunk of chunks) { this.bytes.set(chunk, offset); offset += chunk.length; }
  }
  async arrayBuffer() { return this.bytes.buffer; }
}
class FakeRecorder {
  static isTypeSupported = () => true;
  static latest: FakeRecorder;
  state = "inactive";
  ondataavailable: ((event: { data: Blob }) => void) | null = null;
  onstop: (() => void) | null = null;
  onerror: (() => void) | null = null;
  constructor() { FakeRecorder.latest = this; }
  start() { this.state = "recording"; }
  stop() { this.state = "inactive"; this.ondataavailable?.({ data: new Blob(["synthetic-audio"]) }); this.onstop?.(); }
}
const track = { stop: vi.fn(), onended: null as (() => void) | null };
const stream = { getTracks: () => [track], getAudioTracks: () => [track] };
const getUserMedia = vi.fn();
function setup(transcribe = vi.fn().mockResolvedValue("你好")) {
  const state = vi.fn(), failure = vi.fn(), result = vi.fn();
  return { recorder: new VoiceRecorder(state, failure, transcribe, result), state, failure, result, transcribe };
}
beforeEach(() => {
  vi.useFakeTimers(); vi.stubGlobal("MediaRecorder", FakeRecorder); vi.stubGlobal("Blob", MemoryBlob);
  Object.defineProperty(navigator, "mediaDevices", { configurable: true, value: { getUserMedia } });
  getUserMedia.mockResolvedValue(stream); track.stop.mockClear();
});
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); vi.clearAllMocks(); });
it("stops tracks and returns text only after stopping", async () => {
  const s = setup(); await s.recorder.start(); expect(s.transcribe).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1500); s.recorder.stop(); await vi.advanceTimersByTimeAsync(0);
  expect(track.stop).toHaveBeenCalledTimes(1);
  expect(s.transcribe).toHaveBeenCalledWith(expect.objectContaining({ format: "webm", durationMs: 1500, audio: btoa("synthetic-audio") }), expect.any(AbortSignal));
  expect(s.result).toHaveBeenCalledWith("你好");
});
it("disposes a late permission result", async () => {
  let resolve!: (value: unknown) => void;
  getUserMedia.mockReturnValue(new Promise(r => { resolve = r; }));
  const s = setup(); const pending = s.recorder.start(); s.recorder.cancel(); resolve(stream); await pending;
  expect(track.stop).toHaveBeenCalledOnce(); expect(s.transcribe).not.toHaveBeenCalled();
});
it("rejects short and oversized recordings without uploading", async () => {
  const short = setup(); await short.recorder.start(); short.recorder.stop(); expect(short.failure).toHaveBeenCalledWith("tooShort");
  const large = setup(); await large.recorder.start(); FakeRecorder.latest.ondataavailable?.({ data: { size: MAX_AUDIO_BYTES + 1 } as Blob });
  expect(large.failure).toHaveBeenCalledWith("tooLarge"); expect(large.transcribe).not.toHaveBeenCalled();
});
it("automatically stops at two minutes", async () => {
  const s = setup(); await s.recorder.start(); await vi.advanceTimersByTimeAsync(120_000);
  expect(s.transcribe).toHaveBeenCalledWith(expect.objectContaining({ durationMs: 120_000 }), expect.any(AbortSignal));
});
it("cancels upload and ignores late text", async () => {
  let resolve!: (text: string) => void;
  const s = setup(vi.fn().mockReturnValue(new Promise<string>(r => { resolve = r; })));
  await s.recorder.start(); await vi.advanceTimersByTimeAsync(1000); s.recorder.stop(); await vi.advanceTimersByTimeAsync(0);
  s.recorder.cancel(); resolve("stale"); await vi.advanceTimersByTimeAsync(0);
  expect(s.transcribe.mock.calls[0][1].aborted).toBe(true); expect(s.result).not.toHaveBeenCalled();
});
it("handles permission denial and disconnected devices", async () => {
  getUserMedia.mockRejectedValueOnce(new DOMException("denied", "NotAllowedError"));
  const denied = setup(); await denied.recorder.start(); expect(denied.failure).toHaveBeenCalledWith("permissionDenied");
  const lost = setup(); await lost.recorder.start(); track.onended?.(); expect(lost.failure).toHaveBeenCalledWith("device"); expect(track.stop).toHaveBeenCalled();
});
it("sanitizes provider failure and releases resources", async () => {
  const s = setup(vi.fn().mockRejectedValue(new Error("private response")));
  await s.recorder.start(); await vi.advanceTimersByTimeAsync(1000); s.recorder.stop(); await vi.advanceTimersByTimeAsync(0);
  expect(s.failure).toHaveBeenCalledWith("failed"); expect(s.result).not.toHaveBeenCalled();
});
it("falls back to MP4 and fails closed on unsupported media APIs", async () => {
  vi.spyOn(FakeRecorder, "isTypeSupported").mockImplementation((type?: string) => type === "audio/mp4");
  const s = setup(); await s.recorder.start(); await vi.advanceTimersByTimeAsync(1000); s.recorder.stop(); await vi.advanceTimersByTimeAsync(0);
  expect(s.transcribe).toHaveBeenCalledWith(expect.objectContaining({ format: "mp4" }), expect.any(AbortSignal));
  vi.restoreAllMocks(); vi.stubGlobal("MediaRecorder", undefined);
  const unsupported = setup(); await unsupported.recorder.start(); expect(unsupported.failure).toHaveBeenCalledWith("unsupported");
});
it("times out a permission request and a stalled provider without late output", async () => {
  getUserMedia.mockReturnValueOnce(new Promise(() => {}));
  const permission = setup(); void permission.recorder.start(); await vi.advanceTimersByTimeAsync(30_000);
  expect(permission.failure).toHaveBeenCalledWith("device");
  const upload = setup(vi.fn().mockReturnValue(new Promise(() => {})));
  await upload.recorder.start(); await vi.advanceTimersByTimeAsync(1000); upload.recorder.stop(); await vi.advanceTimersByTimeAsync(65_000);
  expect(upload.failure).toHaveBeenCalledWith("failed"); expect(upload.transcribe.mock.calls[0][1].aborted).toBe(true);
});

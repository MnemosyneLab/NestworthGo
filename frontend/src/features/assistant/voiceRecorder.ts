export const MAX_AUDIO_BYTES = 8 * 1024 * 1024;
export const MAX_RECORDING_MS = 120_000;
export type Recording = { audio: string; format: string; durationMs: number };
export type VoiceState = "idle" | "permission" | "recording" | "transcribing";
export type VoiceError = "unsupported" | "permissionDenied" | "device" | "tooShort" | "tooLarge" | "failed";

export function recordingType(): { mimeType: string; format: string } | undefined {
  if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === "undefined" || typeof MediaRecorder.isTypeSupported !== "function") return;
  return [
    { mimeType: "audio/webm;codecs=opus", format: "webm" },
    { mimeType: "audio/mp4", format: "mp4" },
    { mimeType: "audio/webm", format: "webm" },
  ].find(({ mimeType }) => MediaRecorder.isTypeSupported(mimeType));
}

// One ephemeral recording. Cancel also handles permission resolving after disposal.
export class VoiceRecorder {
  private stream?: MediaStream;
  private recorder?: MediaRecorder;
  private chunks: Blob[] = [];
  private size = 0;
  private started = 0;
  private disposed = false;
  private timer?: ReturnType<typeof setTimeout>;
  private watchdog?: ReturnType<typeof setTimeout>;
  private abort = new AbortController();
  constructor(
    private state: (state: VoiceState) => void,
    private failure: (error: VoiceError) => void,
    private transcribe: (recording: Recording, signal: AbortSignal) => Promise<string>,
    private result: (text: string) => void,
  ) {}
  async start() {
    const type = recordingType();
    if (!type) { this.fail("unsupported"); return; }
    this.state("permission");
    this.watchdog = setTimeout(() => this.fail("device"), 30_000);
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false });
      if (this.disposed) { stream.getTracks().forEach(track => track.stop()); return; }
      clearTimeout(this.watchdog);
      this.stream = stream;
      const recorder = new MediaRecorder(stream, { mimeType: type.mimeType, audioBitsPerSecond: 64_000 });
      this.recorder = recorder;
      recorder.ondataavailable = event => {
        if (this.disposed) return;
        this.size += event.data.size;
        if (this.size > MAX_AUDIO_BYTES) { this.fail("tooLarge"); return; }
        if (event.data.size) this.chunks.push(event.data);
      };
      recorder.onerror = () => this.fail("device");
      recorder.onstop = () => { void this.finish(type); };
      stream.getAudioTracks().forEach(track => { track.onended = () => this.fail("device"); });
      this.started = Date.now();
      recorder.start(250);
      this.state("recording");
      this.timer = setTimeout(() => this.stop(), MAX_RECORDING_MS);
    } catch (error) {
      if (!this.disposed) this.fail(error instanceof DOMException && error.name === "NotAllowedError" ? "permissionDenied" : "device");
    }
  }
  stop() {
    if (this.disposed || this.recorder?.state !== "recording") return;
    clearTimeout(this.timer);
    this.state("transcribing");
    this.watchdog = setTimeout(() => this.fail("failed"), 65_000);
    try { this.recorder.stop(); } catch { this.fail("device"); }
    this.releaseTracks();
  }
  private releaseTracks() {
    this.stream?.getTracks().forEach(track => { track.onended = null; track.stop(); });
    this.stream = undefined;
  }
  private async finish(type: { mimeType: string; format: string }) {
    if (this.disposed) return;
    clearTimeout(this.timer);
    this.releaseTracks();
    const durationMs = Math.min(Date.now() - this.started, MAX_RECORDING_MS);
    if (durationMs < 1000 || !this.size) { this.fail("tooShort"); return; }
    try {
      const bytes = new Uint8Array(await new Blob(this.chunks, { type: type.mimeType }).arrayBuffer());
      this.chunks = [];
      if (this.disposed) return;
      // Bounded slices avoid argument-stack overflow for recordings near the limit.
      let binary = "";
      for (let i = 0; i < bytes.length; i += 8192) binary += String.fromCharCode(...bytes.subarray(i, i + 8192));
      bytes.fill(0);
      const text = await this.transcribe({ audio: btoa(binary), format: type.format, durationMs }, this.abort.signal);
      if (this.disposed) return;
      this.result(text);
      this.cancel();
      this.state("idle");
    } catch { if (!this.disposed) this.fail("failed"); }
  }
  private fail(error: VoiceError) {
    if (this.disposed) return;
    this.cancel();
    this.state("idle");
    this.failure(error);
  }
  cancel() {
    this.disposed = true;
    this.abort.abort();
    clearTimeout(this.timer);
    clearTimeout(this.watchdog);
    if (this.recorder) {
      this.recorder.ondataavailable = null;
      this.recorder.onstop = null;
      this.recorder.onerror = null;
      try { if (this.recorder.state !== "inactive") this.recorder.stop(); } catch { /* Tracks must still be released after a recorder failure. */ }
    }
    this.releaseTracks();
    this.chunks = [];
    this.recorder = undefined;
  }
}

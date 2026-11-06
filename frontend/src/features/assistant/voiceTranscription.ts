import { callCancellableService } from "@/lib/wailsCancellation";
import * as API from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service";
import type { Recording } from "./voiceRecorder";

export function transcribeVoice(recording: Recording, epoch: string, revision: number, signal: AbortSignal) {
  return callCancellableService(signal, () => API.Transcribe({ ...recording, epoch, revision }));
}

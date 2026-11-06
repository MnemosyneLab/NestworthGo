import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Mic, Square, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { transcribeVoice } from "./voiceTranscription";
import { VoiceRecorder, recordingType, type VoiceError, type VoiceState } from "./voiceRecorder";

type Props = { epoch: string; revision: number; disabled: boolean; onText: (text: string) => void };
export function VoiceInput(props: Props) {
  return <VoiceInputSession key={`${props.epoch}:${props.revision}:${props.disabled}`} {...props} />;
}
function VoiceInputSession({ epoch, revision, disabled, onText }: Props) {
  const { t } = useTranslation();
  const [state, setState] = useState<VoiceState>("idle");
  const [error, setError] = useState<VoiceError | "">("");
  const recorder = useRef<VoiceRecorder>();
  const currentText = useRef(onText);
  useEffect(() => { currentText.current = onText; }, [onText]);
  useEffect(() => () => recorder.current?.cancel(), []);
  useEffect(() => {
    const cancel = () => { recorder.current?.cancel(); setState("idle"); };
    const hide = () => { if (document.hidden) cancel(); };
    window.addEventListener("pagehide", cancel);
    document.addEventListener("visibilitychange", hide);
    return () => { window.removeEventListener("pagehide", cancel); document.removeEventListener("visibilitychange", hide); };
  }, []);
  function start() {
    if (disabled || state !== "idle") return;
    setError("");
    recorder.current?.cancel();
    recorder.current = new VoiceRecorder(setState, setError,
      (recording, signal) => transcribeVoice(recording, epoch, revision, signal),
      text => currentText.current(text));
    void recorder.current.start();
  }
  const supported = !!recordingType();
  return <div className="space-y-1">
    <div className="flex flex-wrap items-center gap-2">
      <Button type="button" size="sm" variant="ghost" disabled={disabled || !supported || state === "permission" || state === "transcribing"}
        onClick={() => state === "recording" ? recorder.current?.stop() : start()}>
        {state === "recording" ? <Square className="size-4" aria-hidden="true" /> : <Mic className="size-4" aria-hidden="true" />}
        {t(state === "recording" ? "voice.stop" : "voice.start")}
      </Button>
      {state !== "idle" && <Button type="button" size="sm" variant="ghost" onClick={() => { recorder.current?.cancel(); setState("idle"); }}>
        <X className="size-4" aria-hidden="true" />{t("voice.cancel")}
      </Button>}
      <span role={state === "idle" ? undefined : "status"} aria-live="polite" aria-atomic="true" className="text-xs text-muted-foreground">{state !== "idle" ? t(`voice.${state}`) : ""}</span>
    </div>
    <p className="text-xs text-muted-foreground">{t(supported ? "voice.privacy" : "voice.unsupported")}</p>
    {error && <p role="alert" className="text-xs text-destructive">{t(`voice.${error}`)}</p>}
  </div>;
}

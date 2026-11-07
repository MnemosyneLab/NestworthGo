import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { LoaderCircle, Mic, Square, X } from "lucide-react";
import { toast } from "sonner";
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
  const descriptionId = useId();
  const errorId = useId();
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
    recorder.current = new VoiceRecorder(setState, failure => {
      setError(failure);
      toast.error(t(`voice.${failure}`));
    },
      (recording, signal) => transcribeVoice(recording, epoch, revision, signal),
      text => currentText.current(text));
    void recorder.current.start();
  }
  const supported = !!recordingType();
  const waiting = state === "permission" || state === "transcribing";
  const label = t(state === "recording" ? "voice.stop" : waiting ? `voice.${state}` : "voice.start");
  const iconClass = "size-11 shrink-0 xl:size-9";
  return <div className="flex shrink-0 items-center gap-1" title={!supported ? t("voice.unsupported") : undefined}>
    {state !== "idle" && <Button type="button" size="icon" variant="ghost" className={iconClass}
      aria-label={t("voice.cancel")} title={t("voice.cancel")}
      onClick={() => { recorder.current?.cancel(); setState("idle"); }}>
      <X className="size-4" aria-hidden="true" />
    </Button>}
    <Button type="button" size="icon" variant="ghost" className={iconClass}
      aria-label={label} title={error ? t(`voice.${error}`) : supported ? label : t("voice.unsupported")}
      aria-pressed={state === "recording"} aria-busy={waiting}
      aria-describedby={error ? `${descriptionId} ${errorId}` : descriptionId}
      disabled={disabled || !supported || waiting}
      onClick={() => state === "recording" ? recorder.current?.stop() : start()}>
      {waiting ? <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        : state === "recording" ? <Square className="size-4 text-destructive" aria-hidden="true" />
        : <Mic className={error ? "size-4 text-destructive" : "size-4"} aria-hidden="true" />}
    </Button>
    <span role={state === "idle" ? undefined : "status"} aria-live="polite" aria-atomic="true" className="sr-only">{state !== "idle" ? t(`voice.${state}`) : ""}</span>
    <span id={descriptionId} className="sr-only">{t(supported ? "voice.privacy" : "voice.unsupported")}</span>
    {error && <span id={errorId} className="sr-only">{t(`voice.${error}`)}</span>}
  </div>;
}

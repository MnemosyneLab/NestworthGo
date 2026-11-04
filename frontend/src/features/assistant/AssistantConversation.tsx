import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { Card } from "@/components/ui/card";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { formatAmount } from "@/lib/money";
import { formatTimestamp } from "@/lib/time";
import { isClosedStatus, type Metric, type PageContextV1 } from "./contracts";
import { useAssistantPage } from "./PageContext";

type Message = { id: number; context: PageContextV1; metric: Metric };

/** A local, deterministic page guide in the future conversation surface. It
 * does not claim to be a model, send prompts or create financial proposals. */
export function AssistantConversation({ surface, status, loading, disconnected, retry }: {
  surface: "desktop" | "mobile"; status: unknown; loading: boolean; disconnected: boolean; retry: () => void;
}) {
  const { t, i18n } = useTranslation();
  const page = useAssistantPage();
  const context = page?.context;
  const [selected, setSelected] = useState("");
  const [messages, setMessages] = useState<Message[]>([]);
  const [run, setRun] = useState<"idle" | "running" | "stopped" | "stale">("idle");
  const [notice, setNotice] = useState("");
  const [reviewOpen, setReviewOpen] = useState(false);
  const nextId = useRef(0);
  const pending = useRef<AbortController | null>(null);
  const currentContext = useRef(context);
  useLayoutEffect(() => { currentContext.current = context; }, [context]);
  const [clock, setClock] = useState(() => Date.now());
  useEffect(() => {
    if (!context) return;
    const timer = window.setTimeout(() => setClock(Date.now()), Math.max(1, Date.parse(context.expiresAt) - Date.now()));
    return () => window.clearTimeout(timer);
  }, [context]);
  useEffect(() => () => pending.current?.abort(), []);
  useEffect(() => { if (disconnected) pending.current?.abort(); }, [disconnected]);
  const expired = !context || Date.parse(context.expiresAt) <= clock;
  const ready = !loading && !disconnected && isClosedStatus(status, surface);
  const chosen = context?.metrics.find(m => m.ref === selected);
  const explain = async () => {
    if (!ready || expired || !context || Date.parse(context.expiresAt) <= Date.now() || pending.current) return;
    const metric = chosen ?? (context.metrics.length === 1 ? context.metrics[0] : undefined);
    if (!metric) { setNotice("needsSelection"); return; }
    const controller = new AbortController();
    pending.current = controller;
    setRun("running"); setNotice("");
    // Yield for cancellation and page/authority changes. No network/model call.
    await new Promise<void>(resolve => window.setTimeout(resolve, 0));
    if (pending.current !== controller) return;
    pending.current = null;
    if (controller.signal.aborted) { setRun("stopped"); return; }
    if (currentContext.current?.contextId !== context.contextId || Date.parse(context.expiresAt) <= Date.now()) { setRun("stale"); return; }
    setMessages(previous => [...previous.slice(-19), { id: ++nextId.current, context: structuredClone(context), metric: structuredClone(metric) }]);
    setRun("idle");
  };
  const state = disconnected ? "disconnected" : loading ? "loading" : !ready ? "unavailable" : run;
  return <div className="flex h-full min-h-0 min-w-0 flex-col" data-testid="assistant-conversation">
    <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
      <p className="rounded-lg bg-muted p-3 text-sm text-muted-foreground">{t("assistant.stage")}</p>
      <p role="status" className="text-sm">{t(`assistant.state.${state}`)}</p>
      {(disconnected || (!loading && !ready)) && <Button variant="outline" onClick={retry}>{t("common.retryAction")}</Button>}
      <Card className="space-y-3 p-4">
        <h3 className="font-semibold">{t("assistant.currentPage")}</h3>
        <p className="text-sm text-muted-foreground">{context?.scope.label ?? t("assistant.noContext")}</p>
        {context && <p className="text-xs text-muted-foreground">{t("assistant.snapshot", { time: formatTimestamp(context.capturedAt, context.range.timezone, i18n.language) })}</p>}
        {expired && <p role="status" className="text-sm text-warning-foreground">{t("assistant.expired")}</p>}
        <Label htmlFor={`assistant-metric-${surface}`}>{t("assistant.metric")}</Label>
        <NativeSelect id={`assistant-metric-${surface}`} value={chosen?.ref ?? ""} onChange={e => setSelected(e.target.value)} disabled={!ready || expired || run === "running"}>
          <option value="">{t("assistant.chooseMetric")}</option>
          {context?.metrics.map(m => <option key={m.ref} value={m.ref}>{m.label}</option>)}
        </NativeSelect>
        <Button className="h-auto min-h-10 whitespace-normal" onClick={() => void explain()} disabled={!ready || expired || !context?.metrics.length || run === "running"}>{t("assistant.explain")}</Button>
        {notice && <p role="status" className="text-sm">{t(`assistant.${notice}`)}</p>}
        {context && context.targets.length > 0 && <div className="space-y-2">
          <p className="text-sm font-medium">{t("assistant.openAccount")}</p>
          <p className="text-xs text-muted-foreground">{t("assistant.explicitNavigation")}</p>
          {context.targets.map(target => <Button key={target.ref} variant="outline" className="h-auto min-h-11 w-full justify-start whitespace-normal text-left" disabled={!ready || expired} onClick={() => {
            const result = page?.navigate(target.ref, context);
            if (result !== "opened") setNotice(result === "stale_context" ? "expired" : "navigationBlocked");
          }}>{target.label}</Button>)}
        </div>}
      </Card>
      {messages.length === 0 && <p className="py-4 text-center text-sm text-muted-foreground">{t("assistant.empty")}</p>}
      <div aria-label={t("assistant.conversation")} className="space-y-3">
        {messages.map(message => <Card key={message.id} className="space-y-3 p-4">
          <p className="text-xs font-medium text-muted-foreground">{t("assistant.localGuide")}</p>
          <h3 className="font-semibold">{message.metric.label}</h3>
          <p className="num break-words text-xl">{message.metric.value === null ? t("assistant.state.unavailable") : formatAmount(message.metric.value, message.metric.unit)}</p>
          <p className="text-sm">{t(`assistant.basis.${message.metric.basis}`)}</p>
          <p className="text-sm">{t(`assistant.metricState.${message.metric.status}`)}</p>
          <details className="text-sm">
            <summary className="cursor-pointer rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">{t("assistant.evidence")}</summary>
            <dl className="mt-2 space-y-2 text-muted-foreground">
              <div><dt>{t("assistant.scope")}</dt><dd>{message.context.scope.label}</dd></div>
              <div><dt>{t("assistant.currency")}</dt><dd>{message.metric.unit || t("assistant.state.unavailable")}</dd></div>
              <div><dt>{t("assistant.snapshotLabel")}</dt><dd>{formatTimestamp(message.context.capturedAt, message.context.range.timezone, i18n.language)}</dd></div>
              <div><dt>{t("assistant.timezone")}</dt><dd>{message.context.range.timezone}</dd></div>
              <div><dt>{t("assistant.missing")}</dt><dd>{t(message.metric.missingReasons.length ? "assistant.hasMissing" : "assistant.noMissing")}</dd></div>
            </dl>
            <p className="mt-2">{t("assistant.oldEvidence")}</p>
          </details>
        </Card>)}
      </div>
      <Button variant="outline" className="h-auto min-h-11 whitespace-normal" onClick={() => setReviewOpen(true)}>{t("assistant.review")}</Button>
      <details className="space-y-2 rounded-lg border p-3 text-sm">
        <summary className="cursor-pointer rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">{t("assistant.settings")}</summary>
        <p>{t("assistant.providerUnavailable")}</p><p>{t("assistant.secureStoreUnavailable")}</p><p>{t("assistant.memoryOnly")}</p>
      </details>
    </div>
    <div className="shrink-0 space-y-2 border-t bg-card p-4">
      {run === "running" && <Button variant="outline" onClick={() => { pending.current?.abort(); setRun("stopped"); }}>{t("assistant.stop")}</Button>}
      <Label htmlFor={`assistant-composer-${surface}`}>{t("assistant.composer")}</Label>
      <Input id={`assistant-composer-${surface}`} disabled placeholder={t("assistant.providerUnavailable")} aria-describedby={`assistant-composer-help-${surface}`} />
      <p id={`assistant-composer-help-${surface}`} className="text-xs text-muted-foreground">{t("assistant.noSending")}</p>
    </div>
    <Sheet open={reviewOpen} onOpenChange={setReviewOpen}>
      <SheetContent className="space-y-4">
        <SheetHeader><SheetTitle>{t("assistant.review")}</SheetTitle></SheetHeader>
        <p>{t("assistant.reviewUnavailable")}</p>
        <Button disabled>{t("assistant.confirmUnavailable")}</Button>
      </SheetContent>
    </Sheet>
  </div>;
}

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useQueries } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ArrowDown, Check, ChevronDown, LoaderCircle, Sparkles } from "lucide-react";
import * as API from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service";
import { Button } from "@/components/ui/button";
import { callService } from "@/lib/wails";
import { formatTimestamp } from "@/lib/time";
import { AssistantMarkdown } from "./AssistantMarkdown";
import { useAssistantPage } from "./PageContext";
type Conversation = Awaited<ReturnType<typeof API.Inspect>>;
const diagnosticKeys: Record<string, string> = {
  timeout: "aiChat.timeout",
  provider_authentication: "aiChat.providerAuthentication",
  provider_rate_limited: "aiChat.providerRateLimited",
  provider_request_rejected: "aiChat.providerRequestRejected",
  provider_unavailable: "aiChat.providerUnavailable",
  provider_connection: "aiChat.providerConnection",
  provider_response_rejected: "aiChat.providerResponseRejected",
  model_history_rejected: "aiChat.modelHistoryRejected",
  tool_result_rejected: "aiChat.toolResultRejected",
  assistant_budget_exceeded: "aiChat.runBudgetExceeded",
  "assistant_budget_exceeded.model_rounds": "aiChat.budgetModelRounds",
  "assistant_budget_exceeded.tool_calls": "aiChat.budgetToolCalls",
  "assistant_budget_exceeded.history_bytes": "aiChat.budgetHistory",
  "assistant_budget_exceeded.history_messages": "aiChat.budgetHistory",
  "assistant_budget_exceeded.prompt_bytes": "aiChat.budgetPrompt",
  "assistant_budget_exceeded.text_bytes": "aiChat.budgetText",
  "assistant_budget_exceeded.provider_payload": "aiChat.budgetProviderPayload",
  "assistant_budget_exceeded.tool_result_bytes": "aiChat.budgetToolResult",
  "assistant_budget_exceeded.read_evidence": "aiChat.budgetReadEvidence",
};
// Only exact, code-owned diagnostic rules may be rendered; never reflect an
// unknown backend code (which could contain data from another implementation).
const responseRules = [
  "request_contract",
  "response_headers",
  "response_body",
  "response_terminal",
  "response_item",
  "stream_frame",
  "stream_order",
  "stream_lifecycle",
  "stream_item_added",
  "stream_item_done",
  "stream_function_status",
  "stream_function_arguments",
  "stream_function_identity",
  "stream_arguments",
  "stream_part",
  "stream_terminal",
  "stream_event",
  "stream_eof",
  "sdk_conversion",
  "engine_inventory",
  "engine_empty_text",
];
const responseDiagnostic = (code: string) =>
  responseRules.some((rule) => code === `provider_response_rejected.${rule}`);

const budgetDiagnostic = (code: string) =>
  code.startsWith("assistant_budget_exceeded.") && Object.prototype.hasOwnProperty.call(diagnosticKeys, code);
const active = (state: string) => state === "accepted" || state === "running";
// Host evidence is a completed read trace, not live execution telemetry.
const sources = new Set(["read_page_content", "compare_financial_context", "compare_financial_attribution", "get_financial_comparison_page", "get_financial_context", "get_financial_context_page", "get_financial_context_item", "get_overview", "list_accounts", "get_account_snapshot", "get_read_page", "get_read_item"]);
export function ConversationTimeline({ epoch, id, latest, busy, onCancel, submittedRequestId = "" }: {
  epoch: string; id: string; latest?: Conversation; submittedRequestId?: string; busy: boolean; onCancel: (runID: string) => void;
}) {
  const { t, i18n } = useTranslation();
  const page = useAssistantPage();
  const [first, setFirst] = useState(() => Math.max(0, (latest?.offset ?? 0) - 5));
  const [following, setFollowing] = useState(true);
  const viewport = useRef<HTMLDivElement>(null);
 const content = useRef<HTMLDivElement>(null);
 const followingRef = useRef(true);
  const anchor = useRef<{ element: HTMLElement; offset: number } | null>(null);
  // This component is keyed by runtime epoch and conversation ID by its owner.
  // Retain observed latest attempts while their positional history fetch catches
  // up; a pending/failed read must never erase an already displayed answer.
  const [observed, setObserved] = useState(() => ({
    latest: undefined as Conversation | undefined,
    retained: new Map<string, { run: NonNullable<Conversation["runs"]>[number]; offset: number }>(),
  }));
  if (latest && latest !== observed.latest) {
    const retained = new Map(observed.retained);
    latest.runs?.forEach((run, index) => retained.set(run.id, { run, offset: latest.offset + index }));
    setObserved({ latest, retained });
  }
  const announcement = useRef<HTMLParagraphElement>(null);
  const announced = useRef(new Set<string>());
  const initialized = useRef(false);
  useEffect(() => {
    if (!latest) return;
    for (const run of latest.runs ?? []) {
      if (run.state !== "succeeded" || announced.current.has(run.id)) continue;
      announced.current.add(run.id);
      // Populate the dedicated live region only after it exists in the DOM,
      // including an immediate first reply. React owns no children in this node.
      if ((initialized.current || (!!submittedRequestId && run.requestId === submittedRequestId)) && announcement.current)
        announcement.current.textContent = t("aiChat.replyReceived", { number: latest.offset + 1 });
    }
    initialized.current = true;
  }, [latest, t, submittedRequestId]);
  const offset = latest?.offset ?? 0;
  const history = useQueries({ queries: Array.from({ length: offset - first }, (_, index) => {
    const position = first + index;
    return { queryKey: ["aiConversation", epoch, id, position],
      queryFn: () => callService(() => API.Messages(epoch, id, position)),
      enabled: !!epoch && !!id, networkMode: "always" as const, retry: false, staleTime: Infinity };
  }) });
  const loading = history.some(query => query.isPending);
  const failed = history.some(query => query.isError);
  const visible = new Map(observed.retained);
  for (const conversation of [...history.map(query => query.data), latest]) {
    conversation?.runs?.forEach((run, index) => visible.set(run.id, { run, offset: conversation.offset + index }));
  }
  const runs = [...visible.values()].sort((a, b) => a.offset - b.offset).map(entry => entry.run);
  const signature = `${latest?.progress?.revision ?? 0}:` + runs.map(run => `${run.id}:${run.state}:${run.text?.length}:${run.evidence?.length}:${run.tools?.length}`).join("|");
  useLayoutEffect(() => {
    const el = viewport.current;
    if (!el) return;
    if (anchor.current) {
      if (!loading) {
        const saved = anchor.current;
        if (saved.element.isConnected && el.contains(saved.element))
          el.scrollTop += saved.element.getBoundingClientRect().top - el.getBoundingClientRect().top - saved.offset;
        anchor.current = null;
      }
    } else if (following) el.scrollTop = el.scrollHeight;
  }, [signature, loading, following]);
  useEffect(() => {
    if (!viewport.current || !content.current || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(() => {
      const el = viewport.current;
      if (el && followingRef.current && !anchor.current) el.scrollTop = el.scrollHeight;
    });
    observer.observe(viewport.current);
    observer.observe(content.current);
    return () => observer.disconnect();
  }, []);
  const timestamp = (value: string) => formatTimestamp(value, page?.context?.range.timezone ?? "UTC", i18n.language);
  return <div className="relative flex min-h-0 flex-1 flex-col">
    <p ref={announcement} role="status" aria-live="polite" aria-atomic="true" className="sr-only" />
    <div ref={viewport} className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4 [overflow-anchor:none]"
      role="region" aria-label={t("aiChat.transcript")} tabIndex={0}
      onScroll={event => { const el = event.currentTarget;
        if (anchor.current) anchor.current.offset = anchor.current.element.getBoundingClientRect().top - el.getBoundingClientRect().top;
        followingRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48; setFollowing(followingRef.current); }}>
      <div ref={content} className="mx-auto max-w-2xl space-y-6">
        {first > 0 && <Button variant="ghost" className="w-full text-muted-foreground" disabled={loading} onClick={() => {
          if (viewport.current) {
            const el = viewport.current;
            const top = el.getBoundingClientRect().top;
            const element = Array.from(el.querySelectorAll<HTMLElement>("article")).find(article => article.getBoundingClientRect().bottom > top);
            if (element) anchor.current = { element, offset: element.getBoundingClientRect().top - top };
          }
          followingRef.current = false; setFollowing(false); setFirst(value => Math.max(0, value - 5));
        }}>{t("aiChat.loadEarlier")}</Button>}
        {loading && <p role="status" className="text-center text-xs text-muted-foreground">{t("aiChat.loadingHistory")}</p>}
        {failed && <div role="alert" className="text-sm text-destructive">{t("aiChat.historyError")} <Button variant="ghost" size="sm" onClick={() => history.filter(q => q.isError).forEach(q => void q.refetch())}>{t("common.retryAction")}</Button></div>}
        {runs.map(run => { const tools = active(run.state) && latest?.progress?.runId === run.id ? latest.progress.tools : run.tools; return <article key={run.id} className="space-y-4" aria-label={t("aiChat.exchange")}>
          <div className="ml-8 flex justify-end"><div className="max-w-full rounded-2xl rounded-tr-sm bg-muted px-4 py-3">
            <span className="sr-only">{t("aiChat.you")}</span><p className="whitespace-pre-wrap break-words text-sm leading-relaxed [overflow-wrap:anywhere]">{run.prompt}</p>
          </div></div>
          <div className="flex min-w-0 gap-3"><Sparkles className="mt-1 size-4 shrink-0 text-primary" aria-hidden="true" />
            <div className="min-w-0 flex-1 space-y-3"><span className="sr-only">{t("assistant.title")}</span>
              {!!run.evidence?.length && <details className="group rounded-lg border border-border text-xs text-muted-foreground">
                <summary className="flex min-h-10 cursor-pointer list-none items-center gap-2 rounded-lg px-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
                  <Check className="size-3.5 shrink-0" aria-hidden="true" /><span className="flex-1">{t("aiChat.toolReads", { total: run.evidence.length })}</span><ChevronDown className="size-3.5 transition-transform group-open:rotate-180" aria-hidden="true" />
                </summary>
                <div className="space-y-3 border-t border-border p-3"><p>{t("aiChat.traceHelp")}</p>
                  {run.evidence.map((entry, index) => <div key={index} className="space-y-1">
                    <p className="font-medium text-foreground">{t(sources.has(entry.source) ? `aiChat.source.${entry.source}` : "aiChat.unknownRead")}</p>
                    <p>{t("aiChat.readStarted")} · {timestamp(String(entry.capturedAt))}</p>
                    <p>{t(entry.hasMore ? "aiChat.morePages" : "aiChat.responseEnd")}</p><p>{t(entry.missing === "reported" ? "aiChat.missingReported" : "aiChat.missingUnknown")}</p>
                  </div>)}
                </div>
              </details>}
              {!!tools?.length && <div className="space-y-2 text-xs text-muted-foreground">
                {tools.map((tool, index) => <details key={index} className="rounded-lg border border-border">
                  <summary className="flex min-h-10 cursor-pointer items-center gap-2 rounded-lg px-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                    {tool.state === "running" ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <ChevronDown className="size-3.5" aria-hidden="true" />}
                    <span role="status" aria-live="polite" aria-atomic="true">{t(sources.has(tool.source) || ["get_current_page", "get_navigation_targets", "open_page"].includes(tool.source) ? `aiChat.source.${tool.source}` : "aiChat.unknownRead")} · {t(`aiChat.toolState.${tool.state}`)}</span>
                  </summary>
                  <div className="space-y-2 border-t border-border p-3">
                    <p>{t("aiChat.toolInput")}</p><pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded bg-muted p-2 text-foreground [overflow-wrap:anywhere]">{tool.input || (tool.inputTruncated ? t("aiChat.detailOmitted") : "")}</pre>
                    {tool.inputTruncated && !!tool.input && <p>{t("aiChat.detailTruncated")}</p>}
                    <p>{t("aiChat.toolOutput")}</p>{tool.output ? <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded bg-muted p-2 text-foreground [overflow-wrap:anywhere]">{tool.output}</pre> : <p>{t(tool.outputTruncated ? "aiChat.detailOmitted" : "aiChat.noToolOutput")}</p>}
                    {tool.outputTruncated && !!tool.output && <p>{t("aiChat.detailTruncated")}</p>}
                    <p>{t("aiChat.liveDetailHelp")}</p>
                  </div>
                </details>)}
              </div>}
              {run.text && <AssistantMarkdown text={run.text} />}
              {active(run.state) && latest?.progress?.runId === run.id && latest.progress.text && <div aria-busy="true"><p className="text-xs text-muted-foreground">{t("aiChat.preview")}</p><AssistantMarkdown text={latest.progress.text} /></div>}
              {run.state !== "succeeded" && <p role="status" className="flex items-center gap-2 text-xs text-muted-foreground">
                {active(run.state) && <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" aria-hidden="true" />}{t(`aiChat.run.${run.state}`)}
              </p>}
              {run.code && <p role="alert" className="text-sm text-destructive">{t(responseDiagnostic(run.code) ? "aiChat.providerResponseRejected" : (diagnosticKeys[run.code] ?? "aiChat.runError"))}{(responseDiagnostic(run.code) || budgetDiagnostic(run.code)) && <> {t("aiChat.responseDiagnostic", { code: run.code })}</>}</p>}
              {active(run.state) && <Button variant="outline" size="sm" disabled={busy} onClick={() => onCancel(run.id)}>{t("aiChat.stop")}</Button>}
              <details className="text-xs text-muted-foreground"><summary className="w-fit cursor-pointer rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">{t("aiChat.messageDetails")}</summary><p className="pt-2 break-words">{run.model} · {timestamp(String(run.created))}</p></details>
            </div>
          </div>
        </article>; })}
        {!latest ? <p role="status" className="text-sm text-muted-foreground">{t("aiChat.loadingHistory")}</p> : !runs.length && <div className="flex min-h-48 flex-col items-center justify-center gap-3 text-center"><Sparkles className="size-7 text-primary" aria-hidden="true" /><p className="text-sm text-muted-foreground">{t("aiChat.empty")}</p></div>}
      </div>
    </div>
    {!following && <Button variant="outline" size="sm" className="absolute bottom-3 left-1/2 -translate-x-1/2 shadow-sm" onClick={() => { anchor.current = null; followingRef.current = true; setFollowing(true); }}><ArrowDown className="size-3.5" aria-hidden="true" />{t("aiChat.jumpLatest")}</Button>}
  </div>;
}

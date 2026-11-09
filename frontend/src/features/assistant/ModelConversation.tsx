import { ModelSelector } from "./ModelSelector";
import { configuredModels } from "./modelSelection";
import { ConversationUsage } from "./AIUsage";
import { SecurityOperationRegion } from "@/lib/businessCapture";
import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as API from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ChoiceMenu } from "@/components/ui/choice-menu";
import { Notice } from "@/components/ui/notice";
import { callService } from "@/lib/wails";
import { InfoPopover } from "@/components/ui/info-popover";
import { ArrowUp, ShieldAlert, ShieldCheck } from "lucide-react";
import { ConversationSwitcher } from "./ConversationSwitcher";
import { AssistantEmptyState } from "./AssistantEmptyState";
import { useOperationResults } from "./useOperationResults";
import { ConversationTimeline } from "./ConversationTimeline";
import { utf8ByteLength } from "@/lib/utf8";
import { useAssistantPage } from "./PageContext";
import { capturePageContent } from "./pageContent";
import { VoiceInput } from "./VoiceInput";
import { pausePageOperations } from "./pageOperationAuthority";

function samePageContent(generation: string | null, viewKey: string, hostGeneration: string) {
  if (generation === null) return true;
  try { return capturePageContent(viewKey, hostGeneration).id === generation; } catch { return false; }
}

const active = (state: string) => state === "accepted" || state === "running";

/** Desktop IPC only. No secrets, transcript or provider payload in browser
 * storage. Refresh/reconnect reads state; it never replays Start. */
export function ModelConversation({ visible = true }: { visible?: boolean }) {
  return useModelConversation({ visible });
}

/** The desktop shell owns this controller across dock/Sheet remounts. Keep the
 * draft together with its conversation and in-flight admission guard, in memory
 * only. The rendered voice control still cancels on hide or unmount. */
export function useModelConversation({ visible = true, enabled = true }: { visible?: boolean; enabled?: boolean }) {
  const { t } = useTranslation();
  const page = useAssistantPage();
  const client = useQueryClient();
  const actionClass = "min-h-11 whitespace-normal xl:min-h-9";
  const [offline, setOffline] = useState(() => !navigator.onLine);
  useEffect(() => {
    const update = () => setOffline(!navigator.onLine);
    window.addEventListener("online", update);
    window.addEventListener("offline", update);
    return () => {
      window.removeEventListener("online", update);
      window.removeEventListener("offline", update);
    };
  }, []);
  const [selected, setSelected] = useState("");
  const [prompt, setPrompt] = useState("");
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const [submission, setSubmission] = useState<{ conversationId: string; requestId: string } | null>(null);
  const pending = useRef(false);
  const navigationLease = useRef<{
    requestID: string;
    leaseID: string;
    stamp: string;
    contentGeneration: string | null;
  } | null>(null);
  const [navigationState, setNavigationState] = useState("");
  const navigationAttempt = useRef<string | null>(null);
  const navigationCompletion = useRef<string | null>(null);
  const status = useQuery({
    queryKey: ["aiCredentialStatus"],
    queryFn: () => callService(() => API.Status()),
    enabled,
    refetchInterval: 2000,
    networkMode: "always",
    retry: false,
  });
  const epoch = enabled ? status.data?.epoch ?? "" : "";
  const list = useQuery({
    queryKey: ["aiConversations", epoch],
    queryFn: () => callService(() => API.List(epoch)),
    refetchInterval: 2000,
    enabled: !!epoch,
    networkMode: "always",
    retry: false,
  });
  const recent = [...(list.data ?? [])].sort((a, b) =>
    String(b.updated).localeCompare(String(a.updated)),
  )[0];
  const id = selected || recent?.id || "";
  const latest = useQuery({
    queryKey: ["aiConversation", epoch, id, "latest"],
    queryFn: () => callService(() => API.Inspect(epoch, id)),
    enabled: !!epoch && !!id,
    networkMode: "always",
    retry: false,
    refetchInterval: (query) => query.state.data?.runs?.some((run) => active(run.state)) ? 200 : 1000,
  });
  const operations = useOperationResults(epoch, id, busy, latest.data?.runs?.[0]);
  const running = latest.data?.runs?.some((run) => active(run.state)) ?? false;
  const stale = status.isError || list.isError || latest.isError;
  // An epoch identifies a process/ledger incarnation. Never carry an old ID or
  // pending draft into a replacement runtime.
  const previousEpoch = useRef(epoch);
  useEffect(() => {
    if (previousEpoch.current && previousEpoch.current !== epoch) {
      setSelected("");
      setPrompt("");
      setSubmission(null);
    }
    previousEpoch.current = epoch;
  }, [epoch]);
  // Route, publication, config and host changes revoke the old page lease.
  // This is local IPC metadata only; no provider call or replay is performed.
  const modelStamp = page?.modelStamp ?? "";
  const configRevision = status.data?.revision ?? 0;
  const modelAvailable = status.data?.available ?? false;
  const enabledModels = configuredModels(status.data?.config);
  const modelSelection = status.data?.selection ?? {model:status.data?.config?.model ?? "unconfigured", reasoningEffort:""};
  const selectionValid = enabledModels.includes(modelSelection.model);
  useEffect(() => {
    if (navigationCompletion.current) setNavigationState("navigationUnverified");
    navigationCompletion.current = null;
  }, [epoch, configRevision, modelAvailable, id]);
  useEffect(() => () => { navigationCompletion.current = null; }, []);
  useEffect(() => {
    if (navigationAttempt.current) setNavigationState("navigationStopped");
    navigationLease.current = null;
    navigationAttempt.current = null;
    if (!epoch || !modelAvailable || !modelStamp) return;
    const [kind, accountId = ""] = (page?.viewKey ?? "").split(":");
    const selection =
      kind === "overview"
        ? { kind: "overview", accountId: "" }
        : kind === "accounts"
          ? { kind: accountId ? "account" : "accounts", accountId }
          : { kind: "", accountId: "" };
    void callService(() =>
      API.PublishPage(epoch, configRevision, selection),
    ).catch(() => {});
  }, [modelStamp, epoch, configRevision, modelAvailable, page?.viewKey]);
  useEffect(() => {
    const grant = navigationLease.current;
    const run = latest.data?.runs?.[0];
    if (
      !grant ||
      !run ||
      run.requestId !== grant.requestID ||
      active(run.state)
    )
      return;
    navigationLease.current = null;
    const intent = latest.data?.navigation;
    if (!intent) return;
    if (
      run.state !== "succeeded" ||
      !intent ||
      intent.leaseId !== grant.leaseID ||
      intent.requestId !== grant.requestID ||
      !page ||
      !samePageContent(grant.contentGeneration, page.viewKey, page.modelStamp)
    ) {
      setNavigationState("navigationStopped");
      return;
    }
    navigationAttempt.current = grant.requestID;
    navigationCompletion.current = grant.requestID;
    void page
      .navigateModel(grant.stamp, () =>
        callService(() =>
          API.ConsumeNavigation(epoch, id, run.id, intent.id, intent.leaseId),
        ).then((target) => {
          if (navigationAttempt.current !== grant.requestID || !samePageContent(grant.contentGeneration, page.viewKey, page.modelStamp))
            throw new Error("assistant_navigation_withdrawn");
          return target;
        }),
      )
      .then((result) => {
        if (navigationCompletion.current !== grant.requestID) return;
        navigationCompletion.current = null;
        setNavigationState(result === "opened" ? "navigationOpened" : result === "unverified" ? "navigationUnverified" : "navigationStopped");
      })
      .catch(() => {
        if (navigationCompletion.current === grant.requestID) {
          navigationCompletion.current = null;
          setNavigationState("navigationStopped");
        }
      });
  }, [latest.data, page, epoch, id]);
  /** Resolves true only when `work` completed without an error. */
  async function action(work: () => Promise<unknown>): Promise<boolean> {
    if (pending.current) return false;
    pending.current = true;
    let succeeded = false;
    if (navigationCompletion.current) setNavigationState("navigationUnverified");
    navigationCompletion.current = null;
    // Revoke locally at admission, before Create/Delete (or any other action)
    // can await IPC. A previously admitted Consume response may still be in flight.
    if (navigationAttempt.current)
      setNavigationState("navigationStopped");
    navigationAttempt.current = null;
    navigationLease.current = null;
    setBusy(true);
    setFailed(false);
    try {
      await work();
      succeeded = true;
    } catch {
      navigationAttempt.current = null;
      if (navigationLease.current) {
        navigationLease.current = null;
        setNavigationState("navigationStopped");
      }
      setFailed(true);
    } finally {
      // Reconcile persisted state before releasing the admission guard, including
      // an uncertain Start response. No operation is replayed by this refresh.
      await Promise.allSettled([
        status.refetch(),
        list.refetch(),
        client.invalidateQueries({ queryKey: ["aiConversation", epoch] }),
      ]);
      pending.current = false;
      setBusy(false);
    }
    return succeeded;
  }
  function cancelRun(runID: string) {
    if (navigationCompletion.current) setNavigationState("navigationUnverified");
    navigationCompletion.current = null;
    navigationAttempt.current = null;
    if (navigationLease.current) {
      navigationLease.current = null;
      setNavigationState("navigationStopped");
    }
    return callService(() => API.Cancel(epoch, id, runID));
  }
  function changeFullAccess(enabled: boolean) {
    const resume = pausePageOperations(client);
    void action(() => callService(() => API.SetFullAccess(epoch, configRevision, enabled))).finally(resume);
  }
  async function send() {
    if (
      !status.data?.available ||
      !selectionValid ||
      offline ||
      stale ||
      running ||
      !prompt.trim() ||
      utf8ByteLength(prompt) > 16384
    )
      return;
    const allowNavigation = !!page?.canModelNavigate(page.modelStamp);
    navigationAttempt.current = null;
    const text = prompt;
    const stamp = page?.modelStamp ?? "";
    await action(async () => {
      const current = id || (await callService(() => API.Create(epoch))).id;
      setSelected(current);
      // Only a selector crosses IPC. No frontend metrics/labels/permission claims.
      const [kind, accountId = ""] = (page?.viewKey ?? "").split(":");
      const selection =
        kind === "overview"
          ? { kind: "overview", accountId: "" }
          : kind === "accounts"
            ? { kind: accountId ? "account" : "accounts", accountId }
            : { kind: "", accountId: "" };
      const requestId = crypto.randomUUID();
      let pageLeaseId = "";
      // Page context is optional for conversation admission. A capture budget,
      // publication failure or changing page may disable this turn's deferred
      // navigation, but must not discard an unrelated question or replay Start.
      navigationLease.current = null;
      setNavigationState("");
      if (allowNavigation && page) {
        try {
          const content = capturePageContent(page.viewKey, page.modelStamp);
          const contentGeneration = content.page !== "unsupported" ? content.id : null;
          const lease = await callService(() =>
            API.PublishPage(epoch, status.data!.revision, selection),
          );
          if (page.canModelNavigate(stamp) && samePageContent(contentGeneration, page.viewKey, page.modelStamp)) {
            pageLeaseId = lease.id;
            navigationLease.current = { requestID: requestId, leaseID: lease.id, stamp, contentGeneration };
          }
        } catch { /* On-demand page tools retain their own fresh capture checks. */ }
      }
      setSubmission({ conversationId: current, requestId });
      await callService(() =>
        API.Start({
          epoch,
          revision: status.data!.revision,
          conversationId: current,
          requestId,
          pageLeaseId,
          prompt: text,
          model: modelSelection.model,
          reasoningEffort: modelSelection.reasoningEffort,
          page: selection,
        }),
      );
      setPrompt("");
    });
  }
  // Fallback ordinals follow the backend list order so they stay stable when
  // the display is sorted by recency; no identifier is ever shown.
  const switcherConversations = (list.data ?? []).map((c, index) => {
    const savedTitle = (c.id === id ? latest.data?.title : "") || c.title || "";
    return { id: c.id, updated: String(c.updated ?? ""), savedTitle, title: savedTitle || t("aiChat.session", { number: index + 1 }) };
  }).sort((a, b) => b.updated.localeCompare(a.updated));
  const suggest = (text: string) => { setPrompt(text); document.getElementById("ai-model-prompt")?.focus(); };
  const permissionFull = !!status.data?.fullAccess;
  return (
    <div
      className="flex h-full min-h-0 flex-col"
      data-testid="model-conversation"
    >
      <ConversationSwitcher key={epoch} currentId={id} disabled={busy} mutationDisabled={busy || running || stale || !epoch}
        conversations={switcherConversations}
        onSelect={next => {
          navigationCompletion.current = null;
          navigationLease.current = null;
          navigationAttempt.current = null;
          setNavigationState("");
          setSubmission(null);
          setSelected(next);
          setFailed(false);
        }}
        createDisabled={busy || stale || !epoch}
        onCreate={() => void action(async () => {
          const c = await callService(() => API.Create(epoch));
          setSelected(c.id);
        })}
        onRename={(target, title) => action(() => callService(() => API.Rename(epoch, target, title)))}
        onDelete={target => void action(async () => {
          await callService(() => API.Delete(epoch, target));
          if (target === id) setSelected("");
        })} />
      {(status.isPending || offline || !status.data?.available) && <Notice role="status" className="m-3">
        {t(status.isPending ? "assistant.state.loading" : offline ? "aiChat.offlineLocal" : "aiChat.disabled")}
      </Notice>}
      {(failed || stale) && (
        <Notice tone="warning" role="alert" className="m-3">
          {t("aiChat.error")}
          <Button
            className={actionClass}
            variant="outline"
            onClick={() => void action(async () => {})}
          >
            {t("common.retryAction")}
          </Button>
        </Notice>
      )}
      {latest.data ? <ConversationTimeline key={`${epoch}:${id}`} epoch={epoch} id={id} latest={latest.data}
        operations={operations.data ?? []} operationsError={operations.isError}
        submittedRequestId={submission?.conversationId === id ? submission.requestId : ""}
        onSuggest={status.data?.available && !stale ? suggest : undefined}
        busy={busy} onCancel={(runID) => void action(() => cancelRun(runID))} /> : <div className="min-h-0 flex-1 overflow-y-auto p-4">{id
          ? <p role="status" className="text-sm text-muted-foreground">{t("aiChat.loadingHistory")}</p>
          : <AssistantEmptyState onSuggest={status.data?.available && !stale ? suggest : undefined} disabled={busy} />}</div>}
      {id && <ConversationUsage epoch={epoch} id={id} runID={latest.data?.runs?.[0]?.id ?? ""} stamp={`${latest.data?.updated ?? ""}:${latest.data?.runs?.[0]?.state ?? ""}:${latest.data?.titleSource ?? ""}`} />}
      <form
        className="shrink-0 space-y-2 border-t border-border p-3"
        onSubmit={(event) => {
          event.preventDefault();
          void send();
        }}
      >
        <div className="rounded-2xl border border-border bg-card shadow-xs transition-[border-color,box-shadow] focus-within:border-primary/50 focus-within:ring-2 focus-within:ring-primary/30">
          <Label className="sr-only" htmlFor="ai-model-prompt">{t("aiChat.message")}</Label>
          <Input
            className="min-h-11 rounded-2xl border-0 bg-transparent px-4 text-base shadow-none focus-visible:border-transparent focus-visible:ring-0 xl:min-h-10 xl:text-sm"
            id="ai-model-prompt"
            value={prompt}
            maxBytes={16384}
            byteCountVisibility="sr-only"
            placeholder={t("aiChat.placeholder")}
            onKeyDown={(event) => { if (event.key === "Enter" && event.nativeEvent.isComposing) event.preventDefault(); }}
            autoComplete="off"
            onChange={(event) => setPrompt(event.target.value)}
            disabled={busy || !status.data?.available || stale}
          />
          <div className="flex items-end gap-2 px-2 pb-2">
            <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1.5">
              <ModelSelector models={enabledModels} catalog={status.data?.modelCatalog ?? []} selection={modelSelection}
                disabled={busy || running || stale || !epoch}
                onChange={selection => void action(() => callService(() => API.SelectModel(epoch, configRevision, selection)))} />
              <SecurityOperationRegion className="contents">
                <ChoiceMenu label={t("aiChat.permission")} icon={permissionFull ? ShieldAlert : ShieldCheck} tone={permissionFull ? "warning" : "default"}
                  value={permissionFull ? "full" : "readOnly"}
                  options={[
                    { value: "readOnly", label: t("aiChat.readOnly"), description: t("aiChat.readOnlyHint") },
                    { value: "full", label: t("aiChat.full"), description: t("aiChat.fullHint") },
                  ]}
                  disabled={busy || (!permissionFull && (!status.data?.available || running || stale))}
                  onChange={value => changeFullAccess(value === "full")} />
              </SecurityOperationRegion>
              <InfoPopover label={t("assistant.settings")}><div className="space-y-2">
                {["scope", "privacy", "pageHint", "retention", "titlePolicy"].map(key => <p key={key}>{t(`aiChat.${key}`)}</p>)}
                <p>{t("aiModels.history")}</p>
                {latest.data?.titleSource && <p>{t(`aiChat.titleSource.${latest.data.titleSource}`)}</p>}
                {page?.refresh && <Button type="button" variant="ghost" size="sm" onClick={() => page.refresh?.()}>{t("assistant.refresh")}</Button>}
              </div></InfoPopover>
            </div>
            <div className="flex shrink-0 items-center gap-1">
              <VoiceInput key={`voice:${epoch}:${id}:${configRevision}`} epoch={epoch} revision={configRevision}
                disabled={!visible || busy || running || offline || stale || !modelAvailable}
                onText={text => { setPrompt(current => current ? `${current} ${text}` : text); document.getElementById("ai-model-prompt")?.focus(); }} />
              <Button
                size="icon"
                className="size-11 shrink-0 rounded-full xl:size-9"
                aria-label={t("aiChat.send")}
                title={t("aiChat.send")}
                type="submit"
                disabled={
                  busy ||
                  offline ||
                  running ||
                  !status.data?.available ||
                  !selectionValid ||
                  stale ||
                  !prompt.trim() ||
                  utf8ByteLength(prompt) > 16384
                }
              >
                <ArrowUp className="size-4" aria-hidden="true" />
              </Button>
            </div>
          </div>
        </div>
        {navigationState && (
          <p role="status" className="text-xs text-muted-foreground">
            {t(`aiChat.${navigationState}`)}
          </p>
        )}
      </form>
    </div>
  );
}

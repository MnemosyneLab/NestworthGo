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
import { NativeSelect } from "@/components/ui/select";
import { Notice } from "@/components/ui/notice";
import { callService } from "@/lib/wails";
import { InfoPopover } from "@/components/ui/info-popover";
import { Plus, Trash2, ArrowUp, Pencil } from "lucide-react";
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
  const [rename, setRename] = useState<{ id: string; title: string } | null>(null);
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
      setRename(null);
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
  async function action(work: () => Promise<unknown>) {
    if (pending.current) return;
    pending.current = true;
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
  return (
    <div
      className="flex h-full min-h-0 flex-col"
      data-testid="model-conversation"
    >
      <div className="flex shrink-0 items-center gap-1 border-b border-border p-3">
        <Label htmlFor="ai-history" className="sr-only">{t("aiChat.history")}</Label>
        <NativeSelect id="ai-history" className="min-h-11 min-w-0 flex-1 xl:min-h-9" value={id}
          disabled={busy || !list.data?.length}
          onChange={(event) => {
            navigationCompletion.current = null;
            navigationLease.current = null;
            navigationAttempt.current = null;
            setNavigationState("");
            setSubmission(null);
            setSelected(event.target.value);
            setFailed(false);
          }}>
          <option value="" disabled>{t("aiChat.history")}</option>
          {list.data?.map((c, index) => <option key={c.id} value={c.id}>{(c.id === id ? latest.data?.title : "") || c.title || t("aiChat.session", { number: index + 1 })}</option>)}
        </NativeSelect>
        <Button size="icon" variant="ghost" aria-label={t("aiChat.rename")} title={t("aiChat.rename")}
          disabled={busy || !id || stale} onClick={() => setRename({ id, title: latest.data?.title || list.data?.find(c => c.id === id)?.title || "" })}><Pencil className="size-4" aria-hidden="true" /></Button>
        <Button size="icon" variant="ghost" aria-label={t("aiChat.new")} title={t("aiChat.new")}
          disabled={busy || stale || !epoch} onClick={() => void action(async () => {
            const c = await callService(() => API.Create(epoch));
            setSelected(c.id);
          })}><Plus className="size-4" aria-hidden="true" /></Button>
        <Button size="icon" variant="ghost" aria-label={t("aiChat.delete")} title={t("aiChat.delete")}
          disabled={busy || running || !id || stale} onClick={() => void action(async () => {
            await callService(() => API.Delete(epoch, id));
            setSelected("");
          })}><Trash2 className="size-4" aria-hidden="true" /></Button>
      </div>
      {rename?.id === id && <form className="space-y-2 border-b border-border p-3" onSubmit={event => {
        event.preventDefault();
        const draft = rename;
        if (!draft.title.trim() || Array.from(draft.title.trim()).length > 64) return;
        void action(async () => { await callService(() => API.Rename(epoch, draft.id, draft.title)); setRename(null); });
      }}>
        <Label htmlFor="ai-conversation-title">{t("aiChat.titleLabel")}</Label>
        {/* Native maxlength counts UTF-16 units; 128 allows 64 supplementary code points. */}
        <Input id="ai-conversation-title" autoFocus value={rename.title} maxLength={128} disabled={busy}
          onChange={event => setRename({ id, title: Array.from(event.target.value).slice(0, 64).join("") })}
          onKeyDown={event => { if (event.key === "Escape") setRename(null); if (event.key === "Enter" && event.nativeEvent.isComposing) event.preventDefault(); }} />
        <div className="flex gap-2"><Button type="submit" disabled={busy || stale || !rename.title.trim()}>{t("aiChat.saveTitle")}</Button>
        <Button type="button" variant="ghost" disabled={busy} onClick={() => setRename(null)}>{t("aiChat.cancelTitle")}</Button></div>
      </form>}
      {(status.isPending || offline || !status.data?.available) && <Notice role="status" className="m-3">
        {t(status.isPending ? "assistant.state.loading" : offline ? "aiChat.offlineLocal" : "aiChat.disabled")}
      </Notice>}
        {(failed || stale) && (
          <Notice tone="warning" role="alert">
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
        busy={busy} onCancel={(runID) => void action(() => cancelRun(runID))} /> : <div className="min-h-0 flex-1 p-4"><p role="status" className="text-sm text-muted-foreground">{t(id ? "aiChat.loadingHistory" : "aiChat.empty")}</p></div>}
      {id && <ConversationUsage epoch={epoch} id={id} runID={latest.data?.runs?.[0]?.id ?? ""} stamp={`${latest.data?.updated ?? ""}:${latest.data?.runs?.[0]?.state ?? ""}:${latest.data?.titleSource ?? ""}`} />}
      <form
        className="shrink-0 space-y-3 border-t border-border p-3"
        onSubmit={(event) => {
          event.preventDefault();
          void send();
        }}
      >
        <ModelSelector models={enabledModels} catalog={status.data?.modelCatalog ?? []} selection={modelSelection}
          disabled={busy || running || stale || !epoch}
          onChange={selection => void action(() => callService(() => API.SelectModel(epoch, configRevision, selection)))} />
        <Label className="sr-only" htmlFor="ai-model-prompt">{t("aiChat.message")}</Label>
        <Input
          className="min-h-11 text-base xl:min-h-9 xl:text-sm"
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
        <VoiceInput key={`voice:${epoch}:${id}:${configRevision}`} epoch={epoch} revision={configRevision}
          disabled={!visible || busy || running || offline || stale || !modelAvailable}
          onText={text => { setPrompt(current => current ? `${current} ${text}` : text); document.getElementById("ai-model-prompt")?.focus(); }} />
        <div className="flex flex-wrap items-center gap-2">
        <InfoPopover label={t("assistant.settings")}><div className="space-y-2">
          {["scope", "privacy", "pageHint", "retention", "titlePolicy"].map(key => <p key={key}>{t(`aiChat.${key}`)}</p>)}
          <p>{t("aiModels.history")}</p>
          {latest.data?.titleSource && <p>{t(`aiChat.titleSource.${latest.data.titleSource}`)}</p>}
          {page?.refresh && <Button type="button" variant="ghost" size="sm" onClick={() => page.refresh?.()}>{t("assistant.refresh")}</Button>}
        </div></InfoPopover>
        <SecurityOperationRegion className="mr-auto min-w-0 flex-1">
          <Label htmlFor="ai-permission" className="sr-only">{t("aiChat.permission")}</Label>
          <NativeSelect id="ai-permission" className="min-h-11 max-w-full text-xs xl:min-h-9" title={t("aiChat.fullPermission")} aria-description={t("aiChat.fullPermission")}
            value={status.data?.fullAccess ? "full" : "readOnly"}
            disabled={busy || (!status.data?.fullAccess && (!status.data?.available || running || stale))}
            onChange={event => changeFullAccess(event.target.value === "full")}>
            <option value="readOnly">{t("aiChat.readOnly")}</option>
            <option value="full">{t("aiChat.full")}</option>
          </NativeSelect>
        </SecurityOperationRegion>
        <Button
          className={actionClass}
          aria-label={t("aiChat.send")}
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
          <ArrowUp className="size-4" aria-hidden="true" />{t("aiChat.send")}
        </Button>
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

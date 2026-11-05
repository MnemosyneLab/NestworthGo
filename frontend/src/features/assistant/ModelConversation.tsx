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
import { Plus, Trash2, ArrowUp, Sparkles } from "lucide-react";
import { ChangeReviews } from "./ChangeReviews";
import { ConversationTimeline } from "./ConversationTimeline";
import { utf8ByteLength } from "@/lib/utf8";
import { useAssistantPage } from "./PageContext";
import { capturePageContent } from "./pageContent";

function samePageContent(generation: string | null, viewKey: string, hostGeneration: string) {
  if (generation === null) return true;
  try { return capturePageContent(viewKey, hostGeneration).id === generation; } catch { return false; }
}

const active = (state: string) => state === "accepted" || state === "running";

/** Desktop IPC only. No secrets, transcript or provider payload in browser
 * storage. Refresh/reconnect reads state; it never replays Start. */
export function ModelConversation() {
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
  const navigationGrant = useRef<{
    requestID: string;
    leaseID: string;
    stamp: string;
    contentGeneration: string | null;
  } | null>(null);
  const [navigationState, setNavigationState] = useState("");
  const navigationAttempt = useRef<string | null>(null);
  const status = useQuery({
    queryKey: ["aiCredentialStatus"],
    queryFn: () => callService(() => API.Status()),
    refetchInterval: 2000,
    networkMode: "always",
    retry: false,
  });
  const epoch = status.data?.epoch ?? "";
  const list = useQuery({
    queryKey: ["aiConversations", epoch],
    queryFn: () => callService(() => API.List(epoch)),
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
  useEffect(() => {
    if (navigationGrant.current) setNavigationState("navigationStopped");
    navigationGrant.current = null;
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
    const grant = navigationGrant.current;
    const run = latest.data?.runs?.[0];
    if (
      !grant ||
      !run ||
      run.requestId !== grant.requestID ||
      active(run.state)
    )
      return;
    navigationGrant.current = null;
    const intent = latest.data?.navigation;
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
      .then((result) =>
        setNavigationState(
          result === "opened" ? "navigationOpened" : "navigationStopped",
        ),
      )
      .catch(() => setNavigationState("navigationStopped"));
  }, [latest.data, page, epoch, id]);
  async function action(work: () => Promise<unknown>) {
    if (pending.current) return;
    pending.current = true;
    // Revoke locally at admission, before Create/Delete (or any other action)
    // can await IPC. A previously approved Consume response may still be in flight.
    if (navigationAttempt.current || navigationGrant.current)
      setNavigationState("navigationStopped");
    navigationAttempt.current = null;
    navigationGrant.current = null;
    setBusy(true);
    setFailed(false);
    try {
      await work();
    } catch {
      navigationAttempt.current = null;
      if (navigationGrant.current) {
        navigationGrant.current = null;
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
    navigationAttempt.current = null;
    if (navigationGrant.current) {
      navigationGrant.current = null;
      setNavigationState("navigationStopped");
    }
    return callService(() => API.Cancel(epoch, id, runID));
  }
  async function send(allowNavigation = false) {
    if (
      !status.data?.available ||
      offline ||
      stale ||
      running ||
      !prompt.trim() ||
      utf8ByteLength(prompt) > 16384
    )
      return;
    if (allowNavigation && (!page || !page.canModelNavigate(page.modelStamp))) {
      setNavigationState("navigationStopped");
      return;
    }
    navigationAttempt.current = null;
    const text = prompt;
    const stamp = page?.modelStamp ?? "";
    await action(async () => {
      const content = allowNavigation && page ? capturePageContent(page.viewKey, page.modelStamp) : null;
      const contentGeneration = content && content.page !== "unsupported" ? content.id : null;
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
      if (allowNavigation) {
        const lease = await callService(() =>
          API.PublishPage(epoch, status.data!.revision, selection),
        );
        if (!page?.canModelNavigate(stamp))
          throw new Error("assistant_stale_page");
        await callService(() =>
          API.GrantNavigation(
            epoch,
            status.data!.revision,
            current,
            requestId,
            lease.id,
          ),
        );
        if (!page.canModelNavigate(stamp) || !samePageContent(contentGeneration, page.viewKey, page.modelStamp))
          throw new Error("assistant_stale_page");
        pageLeaseId = lease.id;
        navigationGrant.current = {
          requestID: requestId,
          leaseID: lease.id,
          stamp,
          contentGeneration,
        };
        setNavigationState("navigationPending");
      } else {
        navigationGrant.current = null;
        setNavigationState("");
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
            navigationGrant.current = null;
            navigationAttempt.current = null;
            setNavigationState("");
            setSubmission(null);
            setSelected(event.target.value);
            setFailed(false);
          }}>
          <option value="" disabled>{t("aiChat.history")}</option>
          {list.data?.map((c, index) => <option key={c.id} value={c.id}>{t("aiChat.session", { number: index + 1 })}</option>)}
        </NativeSelect>
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
        submittedRequestId={submission?.conversationId === id ? submission.requestId : ""}
        busy={busy} onCancel={(runID) => void action(() => cancelRun(runID))} /> : <div className="min-h-0 flex-1 p-4"><p role="status" className="text-sm text-muted-foreground">{t(id ? "aiChat.loadingHistory" : "aiChat.empty")}</p></div>}
      <ChangeReviews key={`reviews:${epoch}:${id}`} epoch={epoch} id={id} enabled={status.data?.financialWrites === true} busy={busy || running} runKey={`${latest.data?.runs?.[0]?.id}:${latest.data?.runs?.[0]?.state}`} />
      <form
        className="shrink-0 space-y-3 border-t border-border p-3"
        onSubmit={(event) => {
          event.preventDefault();
          void send();
        }}
      >
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
        <div className="flex flex-wrap items-center gap-2">
        <InfoPopover label={t("assistant.settings")}><div className="space-y-2">
          {["scope", "privacy", "pageHint", "retention"].map(key => <p key={key}>{t(`aiChat.${key}`)}</p>)}
          {page?.refresh && <Button type="button" variant="ghost" size="sm" onClick={() => page.refresh?.()}>{t("assistant.refresh")}</Button>}
        </div></InfoPopover>
        <span className="mr-auto flex items-center gap-1 text-xs text-muted-foreground"><Sparkles className="size-3" aria-hidden="true" />{t(status.data?.financialWrites ? "aiChat.writeControlled" : "aiChat.readOnly")}</span>
        <Button
          className={actionClass}
          aria-label={t("aiChat.send")}
          type="submit"
          disabled={
            busy ||
            offline ||
            running ||
            !status.data?.available ||
            stale ||
            !prompt.trim() ||
            utf8ByteLength(prompt) > 16384
          }
        >
          <ArrowUp className="size-4" aria-hidden="true" />{t("aiChat.send")}
        </Button>
        </div>
        <label className="flex items-start gap-2 text-xs text-muted-foreground">
          <input type="checkbox" checked={status.data?.financialWrites === true} disabled={busy || (!status.data?.financialWrites && (!status.data?.available || running))}
            onChange={event => {const enabled=event.target.checked;void action(()=>callService(()=>API.SetWritePermission(epoch,configRevision,enabled)));}} />
          {t("aiChat.writePermission")}
        </label>
        <details className="text-xs text-muted-foreground">
        <summary className="cursor-pointer rounded py-1 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">{t("aiChat.navigationOptions")}</summary>
        <p className="py-2 leading-relaxed">{t("aiChat.navigationHelp")}</p>
        <Button
          className={`${actionClass} max-w-full`}
          type="button"
          variant="outline"
          disabled={
            busy ||
            offline ||
            running ||
            !status.data?.available ||
            stale ||
            !prompt.trim() ||
            utf8ByteLength(prompt) > 16384
          }
          onClick={() => void send(true)}
        >
          {t("aiChat.sendNavigate")}
        </Button>
        <div className="flex flex-wrap gap-2 pt-2">{page?.context?.targets.map(target => <Button type="button" key={target.ref} variant="ghost" className="max-w-full whitespace-normal" disabled={busy} onClick={() => { if (page.navigate(target.ref, page.context!) !== "opened") setFailed(true); }}>{target.label}</Button>)}</div>
        </details>
        {navigationState && (
          <p role="status" className="text-xs text-muted-foreground">
            {t(`aiChat.${navigationState}`)}
          </p>
        )}
      </form>
    </div>
  );
}

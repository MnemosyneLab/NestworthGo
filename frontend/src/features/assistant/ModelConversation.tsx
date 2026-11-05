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
import { formatTimestamp } from "@/lib/time";
import { utf8ByteLength } from "@/lib/utf8";
import { useAssistantPage } from "./PageContext";
import { capturePageContent } from "./pageContent";

function samePageContent(generation: string | null, viewKey: string, hostGeneration: string) {
  if (generation === null) return true;
  try { return capturePageContent(viewKey, hostGeneration).id === generation; } catch { return false; }
}

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
const active = (state: string) => state === "accepted" || state === "running";

/** Desktop IPC only. No secrets, transcript or provider payload in browser
 * storage. Refresh/reconnect reads state; it never replays Start. */
export function ModelConversation() {
  const { t, i18n } = useTranslation();
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
  const [offset, setOffset] = useState<number | null>(null);
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
    refetchInterval: 1000,
  });
  const history = useQuery({
    queryKey: ["aiConversation", epoch, id, offset],
    queryFn: () => callService(() => API.Messages(epoch, id, offset!)),
    enabled: !!epoch && !!id && offset !== null,
    networkMode: "always",
    retry: false,
  });
  const conversation: Conversation | undefined =
    offset === null ? latest.data : history.data;
  const running = latest.data?.runs?.some((run) => active(run.state)) ?? false;
  const stale =
    status.isError ||
    list.isError ||
    latest.isError ||
    (offset !== null && history.isError);
  // An epoch identifies a process/ledger incarnation. Never carry an old ID or
  // pending draft into a replacement runtime.
  const previousEpoch = useRef(epoch);
  useEffect(() => {
    if (previousEpoch.current && previousEpoch.current !== epoch) {
      setSelected("");
      setOffset(null);
      setPrompt("");
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
      const contentGeneration = allowNavigation && page?.viewKey === "return-analysis:" ? capturePageContent(page.viewKey, page.modelStamp).id : null;
      const current = id || (await callService(() => API.Create(epoch))).id;
      setSelected(current);
      setOffset(null);
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
      <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
        <p className="text-sm text-muted-foreground">{t("aiChat.scope")}</p>
        <Notice role="status">
          {t(
            status.isPending
              ? "assistant.state.loading"
              : offline
                ? "aiChat.offlineLocal"
                : status.data?.available
                  ? "aiChat.ready"
                  : "aiChat.disabled",
          )}
        </Notice>
        <p className="text-xs text-muted-foreground">{t("aiChat.privacy")}</p>
        <p className="text-xs text-muted-foreground">{t("aiChat.pageHint")}</p>
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
        <div className="flex flex-wrap gap-2">
          <Label htmlFor="ai-history" className="sr-only">
            {t("aiChat.history")}
          </Label>
          <NativeSelect
            id="ai-history"
            className="min-h-11 w-full basis-full xl:min-h-9"
            value={id}
            disabled={busy || !list.data?.length}
            onChange={(event) => {
              setSelected(event.target.value);
              setOffset(null);
              setFailed(false);
            }}
          >
            <option value="" disabled>
              {t("aiChat.history")}
            </option>
            {list.data?.map((c, index) => (
              <option key={c.id} value={c.id}>
                {t("aiChat.session", { number: index + 1 })}
              </option>
            ))}
          </NativeSelect>
          <Button
            className={actionClass}
            variant="outline"
            disabled={busy || stale || !epoch}
            onClick={() =>
              void action(async () => {
                const c = await callService(() => API.Create(epoch));
                setSelected(c.id);
                setOffset(null);
              })
            }
          >
            {t("aiChat.new")}
          </Button>
          <Button
            className={actionClass}
            variant="outline"
            disabled={busy || running || !id || stale}
            onClick={() =>
              void action(async () => {
                await callService(() => API.Delete(epoch, id));
                setSelected("");
                setOffset(null);
              })
            }
          >
            {t("aiChat.delete")}
          </Button>
        </div>
        {conversation && conversation.runCount > 1 && (
          <div className="flex flex-wrap gap-2">
            <Button
              className={actionClass}
              variant="outline"
              disabled={busy || running || conversation.offset <= 0}
              onClick={() => setOffset(conversation.offset - 1)}
            >
              {t("aiChat.previous")}
            </Button>
            <Button
              className={actionClass}
              variant="outline"
              disabled={
                busy ||
                running ||
                conversation.offset >= conversation.runCount - 1
              }
              onClick={() =>
                setOffset(
                  conversation.offset + 1 === conversation.runCount - 1
                    ? null
                    : conversation.offset + 1,
                )
              }
            >
              {t("aiChat.next")}
            </Button>
            <Button
              className={actionClass}
              variant="ghost"
              onClick={() => setOffset(null)}
            >
              {t("aiChat.latest")}
            </Button>
          </div>
        )}
        {offset !== null &&
          latest.data?.runs
            ?.filter((run) => active(run.state))
            .map((run) => (
              <Button
                className={actionClass}
                key={run.id}
                variant="outline"
                disabled={busy}
                onClick={() => void action(() => cancelRun(run.id))}
              >
                {t("aiChat.stop")}
              </Button>
            ))}
        {conversation?.runs?.map((run) => (
          <article
            key={run.id}
            className="space-y-3 rounded-xl border border-border p-3"
          >
            <p className="whitespace-pre-wrap break-words text-sm">
              {run.prompt}
            </p>
            <p role="status" className="text-xs text-muted-foreground">
              {t(`aiChat.run.${run.state}`)}
            </p>
            <p className="text-xs text-muted-foreground">
              {run.model} ·{" "}
              {formatTimestamp(
                String(run.created),
                page?.context?.range.timezone ?? "UTC",
                i18n.language,
              )}
            </p>
            {run.text && (
              <p className="whitespace-pre-wrap break-words text-sm">
                {run.text}
              </p>
            )}
            {!!run.evidence?.length && (
              <details className="text-xs text-muted-foreground">
                <summary className={actionClass}>
                  {t("assistant.evidence")}
                </summary>
                <p>{t("aiChat.traceHelp")}</p>
                {run.evidence.map((entry, index) => (
                  <p key={index} className="mt-2">
                    {t(`aiChat.source.${entry.source}`)} ·{" "}
                    {formatTimestamp(
                      String(entry.capturedAt),
                      page?.context?.range.timezone ?? "UTC",
                      i18n.language,
                    )}
                    <br />
                    {t(
                      entry.hasMore ? "aiChat.morePages" : "aiChat.responseEnd",
                    )}
                    <br />
                    {t(
                      entry.missing === "reported"
                        ? "aiChat.missingReported"
                        : "aiChat.missingUnknown",
                    )}
                  </p>
                ))}
              </details>
            )}
            {run.code && (
              <p role="alert" className="text-sm">
                {t(responseDiagnostic(run.code) ? "aiChat.providerResponseRejected" : (diagnosticKeys[run.code] ?? "aiChat.runError"))}
                {responseDiagnostic(run.code) && <> {t("aiChat.responseDiagnostic", { code: run.code })}</>}
              </p>
            )}
            {active(run.state) && (
              <Button
                className={actionClass}
                variant="outline"
                disabled={busy}
                onClick={() => void action(() => cancelRun(run.id))}
              >
                {t("aiChat.stop")}
              </Button>
            )}
          </article>
        ))}
        {!conversation?.runs?.length && (
          <p className="text-sm text-muted-foreground">{t("aiChat.empty")}</p>
        )}
        <p className="text-xs text-muted-foreground">{t("aiChat.retention")}</p>
        {page?.refresh && (
          <Button
            className={actionClass}
            variant="outline"
            onClick={() => page.refresh?.()}
          >
            {t("assistant.refresh")}
          </Button>
        )}
        {page?.context?.targets.map((target) => (
          <Button
            className={actionClass}
            key={target.ref}
            variant="outline"
            disabled={busy}
            onClick={() => {
              if (page.navigate(target.ref, page.context!) !== "opened")
                setFailed(true);
            }}
          >
            {target.label}
          </Button>
        ))}
      </div>
      <form
        className="shrink-0 space-y-2 border-t border-border p-4"
        onSubmit={(event) => {
          event.preventDefault();
          void send();
        }}
      >
        <Label htmlFor="ai-model-prompt">{t("aiChat.message")}</Label>
        <Input
          className="min-h-11 text-base xl:min-h-9 xl:text-sm"
          id="ai-model-prompt"
          value={prompt}
          maxBytes={16384}
          autoComplete="off"
          onChange={(event) => setPrompt(event.target.value)}
          disabled={busy || !status.data?.available || stale}
        />
        <Button
          className={actionClass}
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
          {t("aiChat.send")}
        </Button>
        <Button
          className={actionClass}
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
        <p className="text-xs text-muted-foreground">
          {t("aiChat.navigationHelp")}
        </p>
        {navigationState && (
          <p role="status" className="text-xs text-muted-foreground">
            {t(`aiChat.${navigationState}`)}
          </p>
        )}
      </form>
    </div>
  );
}

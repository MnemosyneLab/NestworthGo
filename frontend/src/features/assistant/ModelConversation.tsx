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

type Conversation = Awaited<ReturnType<typeof API.Inspect>>;
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
  const status = useQuery({
    queryKey: ["aiCredentialStatus"],
    queryFn: () => callService(() => API.Status()),
    refetchInterval: 2000,
    enabled: !offline,
    retry: false,
  });
  const epoch = status.data?.epoch ?? "";
  const list = useQuery({
    queryKey: ["aiConversations", epoch],
    queryFn: () => callService(() => API.List(epoch)),
    enabled: !!epoch && !offline,
    retry: false,
  });
  const recent = [...(list.data ?? [])].sort((a, b) =>
    String(b.updated).localeCompare(String(a.updated)),
  )[0];
  const id = selected || recent?.id || "";
  const latest = useQuery({
    queryKey: ["aiConversation", epoch, id, "latest"],
    queryFn: () => callService(() => API.Inspect(epoch, id)),
    enabled: !!epoch && !!id && !offline,
    retry: false,
    refetchInterval: 1000,
  });
  const history = useQuery({
    queryKey: ["aiConversation", epoch, id, offset],
    queryFn: () => callService(() => API.Messages(epoch, id, offset!)),
    enabled: !!epoch && !!id && offset !== null && !offline,
    retry: false,
  });
  const conversation: Conversation | undefined =
    offset === null ? latest.data : history.data;
  const running = latest.data?.runs?.some((run) => active(run.state)) ?? false;
  const stale =
    offline ||
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
  async function action(work: () => Promise<unknown>) {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setFailed(false);
    try {
      await work();
    } catch {
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
  async function send() {
    if (
      !status.data?.available ||
      stale ||
      running ||
      !prompt.trim() ||
      utf8ByteLength(prompt) > 16384
    )
      return;
    const text = prompt;
    await action(async () => {
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
      await callService(() =>
        API.Start({
          epoch,
          revision: status.data!.revision,
          conversationId: current,
          requestId: crypto.randomUUID(),
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
            offline
              ? "assistant.state.disconnected"
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
                disabled={busy || offline}
                onClick={() =>
                  void action(() =>
                    callService(() => API.Cancel(epoch, id, run.id)),
                  )
                }
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
                {t(
                  run.code === "timeout" ? "aiChat.timeout" : "aiChat.runError",
                )}
              </p>
            )}
            {active(run.state) && (
              <Button
                className={actionClass}
                variant="outline"
                disabled={busy || offline}
                onClick={() =>
                  void action(() =>
                    callService(() => API.Cancel(epoch, id, run.id)),
                  )
                }
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
            running ||
            !status.data?.available ||
            stale ||
            !prompt.trim() ||
            utf8ByteLength(prompt) > 16384
          }
        >
          {t("aiChat.send")}
        </Button>
      </form>
    </div>
  );
}

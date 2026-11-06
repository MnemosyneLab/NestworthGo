import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as API from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service";
import { callService } from "@/lib/wails";
import { NativeSelect } from "@/components/ui/select";
import { DatePickerControl } from "@/components/ui/date-picker-control";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";

type Total = Awaited<ReturnType<typeof API.Usage>>["total"];
function count(value: number | string, missing: number, calls: number) {
  if (!calls || missing === calls) return "—";
  return `${value.toLocaleString()}${missing ? " + ?" : ""}`;
}
export function UsageNumbers({ total }: { total: Total }) {
  const { t } = useTranslation();
  return <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
    {([
      ["input", total.input, total.missingInput], ["output", total.output, total.missingOutput],
      ["cached", total.cachedInput, total.missingCached], ["cacheWrite", total.cacheWrite, total.missingWrite],
      ["reasoning", total.reasoning, total.missingReasoning],
    ] as const).map(([key, value, missing]) => <div className="flex justify-between gap-2" key={key}><dt>{t(`aiUsage.${key}`)}</dt><dd>{count(value, missing, total.calls)}</dd></div>)}
    <div className="flex justify-between gap-2"><dt>{t("aiUsage.calls")}</dt><dd>{total.calls}</dd></div>
    {total.audioCalls > 0 && <div className="flex justify-between gap-2"><dt>{t("aiUsage.audioSeconds")}</dt><dd>{count(total.audioSeconds, total.missingAudio, total.audioCalls)}</dd></div>}
    {(total.failed > 0 || total.pending > 0) && <p className="col-span-2 text-muted-foreground">{t("aiUsage.outcomes", { failed: total.failed, pending: total.pending })}</p>}
  </dl>;
}
const timezone = () => Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
export function ConversationUsage({ epoch, id, runID, stamp }: { epoch: string; id: string; runID: string; stamp: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const total = useQuery({ queryKey: ["aiUsage", epoch, id, stamp], enabled: !!epoch && !!id, networkMode: "always", retry: false,
    queryFn: () => callService(() => API.Usage(epoch, { timezone: timezone(), from: "", to: "", conversationId: id, runId: "", model: "", groupBy: "model" })), refetchInterval: open ? 10000 : false });
  const current = useQuery({ queryKey: ["aiUsageRun", epoch, id, runID, stamp], enabled: !!epoch && !!id && !!runID, networkMode: "always", retry: false,
    queryFn: () => callService(() => API.Usage(epoch, { timezone: timezone(), from: "", to: "", conversationId: id, runId: runID, model: "", groupBy: "model" })), refetchInterval: open ? 10000 : false });
  return <details className="shrink-0 border-t border-border px-3 py-2 text-xs" open={open} onToggle={e => setOpen(e.currentTarget.open)}>
    <summary className="cursor-pointer">{t("aiUsage.title")} · {t("aiUsage.current")} {current.data?.available ? count(current.data.total.tokens, current.data.total.missingTokens, current.data.total.calls) : "—"} · {t("aiUsage.total")} {total.data?.available ? count(total.data.total.tokens, total.data.total.missingTokens, total.data.total.calls) : "—"}</summary>
    <div className="mt-2 max-h-48 space-y-3 overflow-auto">
      {total.isPending ? <p>{t("aiUsage.loading")}</p> : total.isError || !total.data?.available ? <p role="status">{t("aiUsage.unavailable")}</p> : <>
        {current.data?.available && <div><p className="mb-1 font-medium">{t("aiUsage.current")}</p><UsageNumbers total={current.data.total} /></div>}
        <div><p className="mb-1 font-medium">{t("aiUsage.total")}</p><UsageNumbers total={total.data.total} /></div>
        {total.data.groups.map(g => <div key={g.key}><p className="mb-1 font-medium">{g.label}</p><UsageNumbers total={g.total} /></div>)}
        {total.data.incomplete && <p role="status">{t("aiUsage.incomplete")}</p>}
      </>}
      <p className="text-muted-foreground">{t("aiUsage.coverage")}</p>
      <p className="text-muted-foreground">{t("aiUsage.costUnknown")}</p>
    </div>
  </details>;
}
export function AIUsageSettings({ epoch }: { epoch: string }) {
  const { t } = useTranslation();
  const [from, setFrom] = useState(""); const [to, setTo] = useState("");
  const [groupBy, setGroupBy] = useState("day"); const [model, setModel] = useState("");
  const [conversation, setConversation] = useState("");
  const [zone, setZone] = useState(timezone());
  const localZone = timezone();
  const report = useQuery({ queryKey: ["aiUsageSettings", epoch, from, to, groupBy, model, zone, conversation], enabled: !!epoch, retry: false, networkMode: "always",
    queryFn: () => callService(() => API.Usage(epoch, { from, to, groupBy, model, timezone: zone, conversationId: conversation, runId: "" })) });
  const conversations = report.data?.conversations ?? [];
  return <section className="space-y-3 border-t border-border pt-4" aria-label={t("aiUsage.title")}>
    <div className="flex items-center justify-between"><h3 className="font-medium">{t("aiUsage.title")}</h3><Button type="button" variant="ghost" disabled={report.isFetching} onClick={() => void report.refetch()}>{t("aiUsage.refresh")}</Button></div>
    <div className="flex flex-wrap items-end gap-2">
      <div><Label htmlFor="usage-from">{t("aiUsage.from")}</Label><DatePickerControl id="usage-from" value={from} onChange={setFrom} /></div>
      <div><Label htmlFor="usage-to">{t("aiUsage.to")}</Label><DatePickerControl id="usage-to" value={to} onChange={setTo} /></div>
      <div><Label htmlFor="usage-zone">{t("aiUsage.timezone")}</Label><NativeSelect id="usage-zone" value={zone} onChange={e => setZone(e.target.value)}>{[...new Set([localZone, "UTC"])].map(z => <option key={z}>{z}</option>)}</NativeSelect></div>
      <div><Label htmlFor="usage-group">{t("aiUsage.group")}</Label><NativeSelect id="usage-group" value={groupBy} onChange={e => setGroupBy(e.target.value)}>{["day", "conversation", "model", "kind"].map(v => <option key={v} value={v}>{t(`aiUsage.${v}`)}</option>)}</NativeSelect></div>
      <div><Label htmlFor="usage-conversation">{t("aiUsage.conversation")}</Label><NativeSelect id="usage-conversation" value={conversation} onChange={e => setConversation(e.target.value)}><option value="">{t("aiUsage.allConversations")}</option>{report.data?.conversations.map((c, i) => <option key={c.key} value={c.key}>{c.deleted ? `${t("aiUsage.deleted")} ${i + 1}` : c.label}</option>)}</NativeSelect></div>
      <div><Label htmlFor="usage-model">{t("aiUsage.model")}</Label><NativeSelect id="usage-model" value={model} onChange={e => setModel(e.target.value)}><option value="">{t("aiUsage.allModels")}</option>{report.data?.models.map(m => <option key={m}>{m}</option>)}</NativeSelect></div>
      <Button type="button" variant="ghost" onClick={() => {setFrom("");setTo("");setModel("");setConversation("");}}>{t("aiUsage.clear")}</Button>
    </div>
    {report.isPending ? <p role="status">{t("aiUsage.loading")}</p> : report.isError || !report.data?.available ? <p role="alert">{t("aiUsage.unavailable")}</p> : <>
      <UsageNumbers total={report.data.total} />
      {report.data.incomplete && <p role="status">{t("aiUsage.incomplete")}</p>}
      {!report.data.total.calls && <p>{t("aiUsage.empty")}</p>}
      <div className="max-h-96 space-y-2 overflow-auto">{report.data.groups.map(g => <details key={g.key} className="rounded-lg border border-border p-3"><summary className="cursor-pointer text-sm">{groupBy === "kind" ? t(`aiUsage.${g.label === "title" ? "titleKind" : g.label}`) : g.deleted ? `${t("aiUsage.deleted")} ${conversations.findIndex(c => c.key === g.key) + 1}` : g.label || t("aiUsage.unassigned")} · {t("aiUsage.requestCount", { count: g.total.calls })}</summary><div className="pt-2"><UsageNumbers total={g.total} /></div></details>)}</div>
    </>}
    <p className="text-xs text-muted-foreground">{t("aiUsage.settingsCoverage")}</p>
    <p className="text-xs text-muted-foreground">{t("aiUsage.costUnknown")}</p>
  </section>;
}
